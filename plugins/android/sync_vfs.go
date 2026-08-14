package androidfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
)

const (
	syncReadDirChunk = 500
	remoteModeType   = 0170000
	remoteModeDir    = 0040000
	remoteModeLink   = 0120000
)

// syncFS is the narrow surface the VFS needs. SyncClient has deliberately
// concrete streaming return types, so syncClientFS adapts them to io interfaces
// and lets the VFS be tested without constructing wire packets.
type syncFS interface {
	List(context.Context, string) ([]SyncEntry, error)
	Lstat(context.Context, string) (SyncEntry, error)
	Stat(context.Context, string) (SyncEntry, error)
	Receive(context.Context, string) (io.ReadCloser, error)
	Send(context.Context, string, uint32, time.Time) (io.WriteCloser, error)
}

type syncClientFS struct{ client *SyncClient }

func (s syncClientFS) List(ctx context.Context, p string) ([]SyncEntry, error) {
	return s.client.List(ctx, p)
}
func (s syncClientFS) Lstat(ctx context.Context, p string) (SyncEntry, error) {
	return s.client.Lstat(ctx, p)
}
func (s syncClientFS) Stat(ctx context.Context, p string) (SyncEntry, error) {
	return s.client.Stat(ctx, p)
}
func (s syncClientFS) Receive(ctx context.Context, p string) (io.ReadCloser, error) {
	return s.client.Receive(ctx, p)
}
func (s syncClientFS) Send(ctx context.Context, p string, mode uint32, mtime time.Time) (io.WriteCloser, error) {
	return s.client.Send(ctx, p, mode, mtime)
}

type shellResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type shellCommandFunc func(context.Context, string, string) (shellResult, error)

// shellCommandStreamFunc is the production command path. shellCommandFunc is
// retained alongside it because mutations need collected stderr for error
// messages and because it is the established constructor/test seam.
type shellCommandStreamFunc func(context.Context, string, string, func([]byte)) (int, error)

// SyncVFS is the compatibility backend used when the device shell cannot run
// the FISH+ helper. Sync connections are operation-scoped, while mutations that
// ADB Sync does not express are executed by the ordinary unprivileged shell.
type SyncVFS struct {
	parent vfs.VFS
	serial string
	title  string
	path   string
	client syncFS
	run    shellCommandFunc
	stream shellCommandStreamFunc

	panelInfoMu sync.RWMutex
	panelInfo   vfs.PanelInfoProvider
}

func newSyncVFS(parent vfs.VFS, serial, title string, client syncFS, run shellCommandFunc) *SyncVFS {
	return &SyncVFS{parent: parent, serial: serial, title: title, path: "/", client: client, run: run}
}

func (s *SyncVFS) GetTitle() string { return s.title }
func (s *SyncVFS) SessionKey() any  { return "android:" + s.serial }
func (s *SyncVFS) PanelTitle(p string) string {
	return androidPanelTitle(s.title, p)
}

func (s *SyncVFS) SetPanelInfoProvider(provider vfs.PanelInfoProvider) {
	s.panelInfoMu.Lock()
	s.panelInfo = provider
	s.panelInfoMu.Unlock()
}

func (s *SyncVFS) panelInfoProvider() vfs.PanelInfoProvider {
	s.panelInfoMu.RLock()
	defer s.panelInfoMu.RUnlock()
	return s.panelInfo
}

func (s *SyncVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if provider := s.panelInfoProvider(); provider != nil {
		return provider.PanelInfoKey(req)
	}
	return ""
}

func (s *SyncVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	if provider := s.panelInfoProvider(); provider != nil {
		return provider.CachedPanelInfo(req)
	}
	return vfs.PanelInfoSnapshot{}, true
}

func (s *SyncVFS) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if provider := s.panelInfoProvider(); provider != nil {
		return provider.RefreshPanelInfo(ctx, req)
	}
	return vfs.PanelInfoSnapshot{}, nil
}
func (s *SyncVFS) IsAtRoot() bool  { return s.path == "/" || s.path == "" }
func (s *SyncVFS) GetPath() string { return s.path }
func (s *SyncVFS) IsAbs(p string) bool {
	return path.IsAbs(p)
}
func (s *SyncVFS) Join(elem ...string) string { return path.Join(elem...) }
func (s *SyncVFS) Base(p string) string       { return path.Base(p) }
func (s *SyncVFS) Dir(p string) string        { return path.Dir(p) }

func (s *SyncVFS) abs(p string) string {
	if p == "" {
		return s.path
	}
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Join(s.path, p)
}

func (s *SyncVFS) mutationTarget(p string) (string, error) {
	if strings.IndexByte(p, 0) >= 0 {
		return "", fmt.Errorf("android: path contains NUL")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fmt.Errorf("android: mutation path %q contains a '..' component", p)
		}
	}
	return mutationPath(s.abs(p))
}

func (s *SyncVFS) Abs(p string) (string, error) { return s.abs(p), nil }

func (s *SyncVFS) SetPath(p string) error {
	target := s.abs(p)
	entry, err := s.client.Stat(context.Background(), target)
	if err != nil {
		return err
	}
	isDir := entry.Mode&remoteModeType == remoteModeDir
	if !isDir && entry.Mode&remoteModeType == remoteModeLink {
		isDir, err = s.shellTestDir(context.Background(), target)
		if err != nil {
			return err
		}
	}
	if !isDir {
		return os.ErrInvalid
	}
	s.path = target
	return nil
}

func syncEntrySize(size uint64) int64 {
	if size > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(size)
}

func syncEntryItem(entry SyncEntry) vfs.VFSItem {
	name := entry.Name
	uid, gid := -1, -1
	if entry.metadataV2 {
		uid, gid = int(entry.UID), int(entry.GID)
	}
	return vfs.VFSItem{
		Name:         name,
		Size:         syncEntrySize(entry.Size),
		IsDir:        entry.Mode&remoteModeType == remoteModeDir,
		MTime:        entry.ModTime,
		ATime:        entry.AccessTime,
		CTime:        entry.ChangeTime,
		IsExecutable: entry.Mode&0111 != 0,
		IsHidden:     strings.HasPrefix(name, "."),
		IsSymlink:    entry.Mode&remoteModeType == remoteModeLink,
		UnixMode:     entry.Mode,
		Uid:          uid,
		Gid:          gid,
		Device:       entry.Device,
		Inode:        entry.Inode,
	}
}

func (s *SyncVFS) resolveDirectory(ctx context.Context, dir string, entry SyncEntry) bool {
	if entry.Mode&remoteModeType != remoteModeLink {
		return entry.Mode&remoteModeType == remoteModeDir
	}
	target := path.Join(dir, entry.Name)
	if followed, err := s.client.Stat(ctx, target); err == nil && followed.Mode&remoteModeType == remoteModeDir {
		return true
	}
	isDir, err := s.shellTestDir(ctx, target)
	return err == nil && isDir
}

func (s *SyncVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	dir := s.abs(p)
	entries, err := s.client.List(ctx, dir)
	if err != nil {
		return err
	}
	items := make([]vfs.VFSItem, 0, syncReadDirChunk)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name == "." || entry.Name == ".." {
			continue
		}
		if err := entry.Err(); err != nil {
			// The name raced with lstat on the device. It is already gone (or
			// inaccessible), so presenting a row that cannot be opened is worse
			// than omitting this one refresh.
			continue
		}
		item := syncEntryItem(entry)
		if item.IsSymlink {
			item.IsDir = s.resolveDirectory(ctx, dir, entry)
		}
		items = append(items, item)
		if len(items) == syncReadDirChunk {
			if onChunk != nil {
				onChunk(items)
			}
			items = make([]vfs.VFSItem, 0, syncReadDirChunk)
		}
	}
	if len(items) != 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (s *SyncVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	target := s.abs(p)
	entry, err := s.client.Lstat(ctx, target)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	entry.Name = path.Base(target)
	item := syncEntryItem(entry)
	if item.IsSymlink {
		item.IsDir = s.resolveDirectory(ctx, path.Dir(target), entry)
	}
	return item, nil
}

func (s *SyncVFS) runChecked(ctx context.Context, command string) error {
	if s.run == nil {
		return errors.New("android: shell command runner is unavailable")
	}
	result, err := s.run(ctx, s.serial, command)
	if err != nil {
		return err
	}
	if result.ExitCode == 0 {
		return nil
	}
	message := strings.TrimSpace(string(result.Stderr))
	if message == "" {
		message = strings.TrimSpace(string(result.Stdout))
	}
	if message == "" {
		message = "remote command failed"
	}
	return fmt.Errorf("android: shell exited with status %d: %s", result.ExitCode, message)
}

// RunCommand gives the compatibility Sync backend the same command-line
// behavior as Android FISH+. Production mounts use the streaming shell-v2
// path, whose callback receives stdout and stderr in packet order. The
// collected runner remains a source-compatible fallback for tests and older
// construction paths.
func (s *SyncVFS) RunCommand(ctx context.Context, dir, command string, cb func(line string)) (int, error) {
	if s.stream == nil && s.run == nil {
		return 0, errors.New("android: shell command runner is unavailable")
	}
	if strings.TrimSpace(command) == "" {
		return 0, errors.New("android: empty shell command")
	}
	target := s.abs(dir)
	// Keep the closing syntax on its own line so a valid trailing shell comment
	// cannot comment it out. The whole group remains non-interactive.
	wrapped := "cd " + quoteShellArg(target) + " && (\n" + command + "\n) </dev/null"
	if s.stream != nil {
		lines := newAndroidCommandLineWriter(cb)
		code, err := s.stream(ctx, s.serial, wrapped, func(output []byte) {
			_, _ = lines.Write(output)
		})
		lines.Flush()
		return code, err
	}
	result, err := s.run(ctx, s.serial, wrapped)
	if err != nil {
		return result.ExitCode, err
	}
	emitShellLines(result.Stdout, cb)
	emitShellLines(result.Stderr, cb)
	return result.ExitCode, nil
}

// CommandRunnerInfo describes the device-side shell. Each ADB shell_v2 call
// owns its transport, so a small bounded group can safely execute in parallel.
func (*SyncVFS) CommandRunnerInfo() vfs.CommandRunnerInfo {
	return vfs.CommandRunnerInfo{Dialect: vfs.CommandDialectPOSIX, MaxParallel: 4}
}

func emitShellLines(output []byte, cb func(line string)) {
	if cb == nil || len(output) == 0 {
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	if len(lines) != 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		cb(strings.TrimSuffix(line, "\r"))
	}
}

type androidCommandLineWriter struct {
	mu      sync.Mutex
	pending []byte
	cb      func(string)
}

const androidCommandOutputChunkBytes = 64 << 10

func newAndroidCommandLineWriter(cb func(string)) *androidCommandLineWriter {
	return &androidCommandLineWriter{cb: cb}
}

func (w *androidCommandLineWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 || w.cb == nil {
		return n, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 && len(w.pending) <= androidCommandOutputChunkBytes {
			break
		}
		end, consumed := i, i+1
		if i < 0 || i > androidCommandOutputChunkBytes {
			end = androidCommandOutputChunkEnd(w.pending, androidCommandOutputChunkBytes)
			consumed = end
		}
		line := w.pending[:end]
		if len(line) != 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		w.cb(strings.ToValidUTF8(string(line), "\uFFFD"))
		w.pending = w.pending[consumed:]
	}
	return n, nil
}

func androidCommandOutputChunkEnd(data []byte, limit int) int {
	if len(data) <= limit {
		return len(data)
	}
	end := limit
	for end > 0 && end < len(data) && !utf8.RuneStart(data[end]) {
		end--
	}
	if end == 0 {
		return limit
	}
	return end
}

func (w *androidCommandLineWriter) Flush() {
	if w.cb == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	line := w.pending
	if line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	w.cb(strings.ToValidUTF8(string(line), "\uFFFD"))
	w.pending = nil
}

func (s *SyncVFS) shellTestDir(ctx context.Context, p string) (bool, error) {
	if s.run == nil {
		return false, nil
	}
	result, err := s.run(ctx, s.serial, "test -d "+quoteShellArg(p))
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (s *SyncVFS) MkDir(ctx context.Context, p string) error {
	target, err := s.mutationTarget(p)
	if err != nil {
		return err
	}
	return s.runChecked(ctx, "mkdir -p -- "+quoteShellArg(target))
}

func (s *SyncVFS) Remove(ctx context.Context, p string) error {
	target, err := s.mutationTarget(p)
	if err != nil {
		return err
	}
	return s.runChecked(ctx, "rm -rf -- "+quoteShellArg(target))
}

func (s *SyncVFS) Rename(ctx context.Context, oldPath, newPath string) error {
	oldTarget, err := s.mutationTarget(oldPath)
	if err != nil {
		return err
	}
	newTarget, err := s.mutationTarget(newPath)
	if err != nil {
		return err
	}
	return s.runChecked(ctx, "mv -f -- "+quoteShellArg(oldTarget)+" "+quoteShellArg(newTarget))
}

func (s *SyncVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: true}
}

func (s *SyncVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, nil
}

func (s *SyncVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	target := s.abs(p)
	entry, err := s.client.Stat(ctx, target)
	if err != nil {
		return nil, err
	}
	if entry.Mode&remoteModeType == remoteModeDir {
		return nil, fmt.Errorf("android: %q is a directory", target)
	}
	return &syncReadFile{client: s.client, path: target, size: syncEntrySize(entry.Size)}, nil
}

func (s *SyncVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	target, err := s.mutationTarget(p)
	if err != nil {
		return nil, err
	}
	return s.client.Send(ctx, target, 0644, time.Now())
}

// CreatePrivateCommandFile sends Apply list files with their private mode in
// the ADB Sync SEND request. Sync does not materialize the remote file until
// DATA/DONE, so chmod-before-write cannot secure it.
func (s *SyncVFS) CreatePrivateCommandFile(ctx context.Context, p string) (io.WriteCloser, error) {
	target, err := s.mutationTarget(p)
	if err != nil {
		return nil, err
	}
	return s.client.Send(ctx, target, 0600, time.Now())
}

func (s *SyncVFS) SetAttributes(ctx context.Context, p string, item vfs.VFSItem) error {
	target, err := s.mutationTarget(p)
	if err != nil {
		return err
	}
	quoted := quoteShellArg(target)
	if item.UnixMode != 0 {
		mode := strconv.FormatUint(uint64(item.UnixMode&07777), 8)
		if err := s.runChecked(ctx, "chmod "+mode+" -- "+quoted); err != nil {
			return err
		}
	}
	if item.Uid >= 0 && item.Gid >= 0 {
		spec := strconv.Itoa(item.Uid) + ":" + strconv.Itoa(item.Gid)
		if err := s.runChecked(ctx, "chown "+spec+" -- "+quoted); err != nil {
			return err
		}
	} else if item.Uid >= 0 {
		if err := s.runChecked(ctx, "chown "+strconv.Itoa(item.Uid)+" -- "+quoted); err != nil {
			return err
		}
	} else if item.Gid >= 0 {
		if err := s.runChecked(ctx, "chgrp "+strconv.Itoa(item.Gid)+" -- "+quoted); err != nil {
			return err
		}
	}

	mtime, atime := item.MTime, item.ATime
	if mtime.IsZero() {
		mtime = atime
	}
	if atime.IsZero() {
		atime = mtime
	}
	if !mtime.IsZero() {
		if err := s.runChecked(ctx, "touch -m -d @"+strconv.FormatInt(mtime.Unix(), 10)+" -- "+quoted); err != nil {
			return err
		}
	}
	if !atime.IsZero() {
		if err := s.runChecked(ctx, "touch -a -d @"+strconv.FormatInt(atime.Unix(), 10)+" -- "+quoted); err != nil {
			return err
		}
	}
	return nil
}

func (s *SyncVFS) ParentVFS() vfs.VFS { return s.parent }
func (s *SyncVFS) Clone() vfs.VFS {
	return &SyncVFS{
		parent: s.parent, serial: s.serial, title: s.title, path: s.path,
		client: s.client, run: s.run, stream: s.stream, panelInfo: s.panelInfoProvider(),
	}
}
func (s *SyncVFS) Close() error { return nil }

var (
	_ vfs.PanelInfoProvider         = (*SyncVFS)(nil)
	_ vfs.CommandRunner             = (*SyncVFS)(nil)
	_ vfs.CommandRunnerInfoProvider = (*SyncVFS)(nil)
)

// syncReadFile streams ordinary sequential reads directly from RECV. A caller
// that asks for ReadAt triggers one local materialization, because the Sync
// protocol has no offset operation unless sendrecv_v2 happens to be present and
// older API-24 devices still need identical VFS semantics.
type syncReadFile struct {
	mu sync.Mutex

	client  syncFS
	path    string
	size    int64
	offset  int64
	stream  io.ReadCloser
	cancel  context.CancelFunc
	readErr error
	temp    *os.File
	tmp     string
	closed  bool
}

func (f *syncReadFile) Size() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.size
}

func (f *syncReadFile) Read(ctx context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if f.temp != nil {
		n, err := f.temp.Read(p)
		f.offset += int64(n)
		return n, err
	}
	if f.readErr != nil {
		return 0, f.readErr
	}
	if f.stream == nil {
		streamCtx, cancel := context.WithCancel(context.Background())
		stream, err := f.client.Receive(streamCtx, f.path)
		if err != nil {
			cancel()
			return 0, err
		}
		f.stream = stream
		f.cancel = cancel
	}
	stop := context.AfterFunc(ctx, f.cancel)
	n, err := f.stream.Read(p)
	_ = stop()
	f.offset += int64(n)
	if err != nil {
		closeErr := f.stream.Close()
		f.cancel()
		f.stream = nil
		f.cancel = nil
		if err == io.EOF && closeErr != nil {
			err = closeErr
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		f.readErr = err
	}
	return n, err
}

func (f *syncReadFile) materialize(ctx context.Context) error {
	if f.temp != nil {
		return nil
	}
	if f.stream != nil {
		_ = f.stream.Close()
		f.stream = nil
	}
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	stream, err := f.client.Receive(ctx, f.path)
	if err != nil {
		return err
	}
	defer stream.Close()

	tmp, err := os.CreateTemp("", "f4-android-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			tmp.Close()
			os.Remove(name)
		}
	}()
	n, err := io.Copy(tmp, stream)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := tmp.Seek(f.offset, io.SeekStart); err != nil {
		return err
	}
	f.size = n
	f.temp = tmp
	f.tmp = name
	keep = true
	return nil
}

func (f *syncReadFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	if off < 0 {
		return 0, fmt.Errorf("android: negative read offset %d", off)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := f.materialize(ctx); err != nil {
		return 0, err
	}
	return f.temp.ReadAt(p, off)
}

func (f *syncReadFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	var err error
	if f.stream != nil {
		err = f.stream.Close()
		f.stream = nil
	}
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	if f.temp != nil {
		if closeErr := f.temp.Close(); err == nil {
			err = closeErr
		}
		f.temp = nil
	}
	if f.tmp != "" {
		if removeErr := os.Remove(f.tmp); err == nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = removeErr
		}
		f.tmp = ""
	}
	return err
}
