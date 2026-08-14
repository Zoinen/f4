package fusefs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	defaultDirCacheTTL = 5 * time.Second
	spoolChunk         = 256 * 1024
)

// errClosed is returned once the mount has released its VFS.
var errNoSymlinks = errors.New("this file system has no symbolic links")

var errClosed = errors.New("mount is closed")

// bridge owns one vfs.VFS and turns path-based FUSE requests into calls on
// it. It contains no FUSE types, so the whole VFS side of a mount compiles
// and can be tested on platforms that cannot mount anything.
//
// Every VFS call happens under mu. f4's VFS implementations are stateful
// and were written for a single UI thread, while FUSE serves requests from
// many kernel threads at once; serializing is the only assumption that
// holds for every backend. It also means one slow read stalls the mount,
// which is the main thing later iterations should improve.
type bridge struct {
	mu     sync.Mutex
	v      vfs.VFS
	closed bool

	root     string
	randomOK bool
	// readOnly is the mount's mode, not the backend's. Until iteration 4
	// every mount sets it, but the write side has to ask a fact rather than
	// a constant, or turning writes on means hunting for hardcoded EROFS.
	readOnly bool
	// writeOK is what the backend says about itself, kept next to readOnly
	// so the two reasons a write can be refused stay distinguishable.
	writeOK  bool
	cacheTTL time.Duration
	cacheMu  sync.Mutex
	dirCache map[string]dirCacheEntry

	// writeMu guards writers only. It is deliberately not b.mu: looking up
	// who is writing a file must not queue behind a VFS call that is busy
	// reading something else.
	writeMu sync.Mutex
	writers map[string]*writeHandle

	// stats counts VFS calls and the time spent in them, when
	// F4_FUSE_STATS is set. Off by default: a mount should not pay for
	// bookkeeping nobody asked for.
	statsOn bool
	statsMu sync.Mutex
	stats   map[string]*opStat
}

// opStat is one operation's tally.
type opStat struct {
	calls int64
	total time.Duration
}

// track times one VFS call. The returned function must be called when the
// call finishes; it costs a map lookup and is skipped entirely when stats are
// off.
//
// This exists because a benchmark can only say that something is slow. Which
// call is being made, and how many times, is the part that says why: a
// backend answering 500 opens is a different problem from one answering
// 50 000 lookups, and the two are indistinguishable from the outside.
func (b *bridge) track(op string) func() {
	if !b.statsOn {
		return func() {}
	}
	start := time.Now()
	return func() {
		elapsed := time.Since(start)
		b.statsMu.Lock()
		st, ok := b.stats[op]
		if !ok {
			st = &opStat{}
			b.stats[op] = st
		}
		st.calls++
		st.total += elapsed
		b.statsMu.Unlock()
	}
}

// StatsReport renders the tally, busiest first, or "" when stats are off.
func (b *bridge) statsReport() string {
	if !b.statsOn {
		return ""
	}
	b.statsMu.Lock()
	defer b.statsMu.Unlock()
	if len(b.stats) == 0 {
		return ""
	}
	names := make([]string, 0, len(b.stats))
	for name := range b.stats {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return b.stats[names[i]].total > b.stats[names[j]].total
	})
	var sb strings.Builder
	sb.WriteString("f4 fuse: VFS calls made by this mount\n")
	for _, name := range names {
		st := b.stats[name]
		per := st.total / time.Duration(st.calls)
		fmt.Fprintf(&sb, "  %-12s %6d calls  %10s total  %10s each\n",
			name, st.calls, st.total.Truncate(time.Millisecond), per.Truncate(time.Microsecond))
	}
	return sb.String()
}

// writeHandle is one file being written through the mount, shared by every
// kernel handle open on that path.
//
// The table exists because FUSE hands out one handle per open() and a file
// manager, a shell and an editor may all be writing the same file. Without a
// table each of them would stage a private copy and the last close would
// silently win, which is exactly the kind of loss a mount must not invent.
type writeHandle struct {
	path string
	refs int

	// staged is where the writes land when the backend can only be handed a
	// whole file. Exactly one of staged and direct is set.
	staged *stagedFile
	// direct is the backend's own file, written at the offset the kernel
	// asked for. No staging, no download on open, no commit on close.
	direct vfs.WriterAtCloser
}

// needsCommit says whether closing this handle still has to send anything.
// A directly written file is already in the backend; a staged one is not.
func (h *writeHandle) needsCommit() bool { return h.direct == nil }

func (h *writeHandle) writeAt(p []byte, off int64) (int, error) {
	if h.direct != nil {
		return h.direct.WriteAt(p, off)
	}
	return h.staged.WriteAt(p, off)
}

func (h *writeHandle) truncate(size int64) error {
	if h.direct != nil {
		return h.direct.Truncate(size)
	}
	return h.staged.Truncate(size)
}

func (h *writeHandle) close() error {
	if h.direct != nil {
		return h.direct.Close()
	}
	return h.staged.Close()
}

// stagedFile is a file being assembled on local disk before it is handed to
// the backend in one piece.
//
// It exists because FUSE writes arrive as many small offsets while vfs.Create
// returns a plain io.WriteCloser: there is no way to hand the backend a write
// at offset 40000 and then one at offset 0. Staging locally also means a
// failed transfer leaves the backend's copy untouched rather than half
// rewritten.
//
// The file is unlinked immediately after creation, exactly like the read-side
// spool, so it disappears with the process even on a crash and never shows up
// in anybody's temp directory listing.
type stagedFile struct {
	f *os.File
}

func newStagedFile() (*stagedFile, error) {
	f, err := os.CreateTemp("", "f4-fuse-w-*")
	if err != nil {
		return nil, err
	}
	os.Remove(f.Name())
	return &stagedFile{f: f}, nil
}

func (s *stagedFile) WriteAt(p []byte, off int64) (int, error) {
	return s.f.WriteAt(p, off)
}

func (s *stagedFile) ReadAt(p []byte, off int64) (int, error) {
	return s.f.ReadAt(p, off)
}

func (s *stagedFile) Truncate(size int64) error {
	return s.f.Truncate(size)
}

func (s *stagedFile) Size() (int64, error) {
	fi, err := s.f.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// Reader rewinds the staged file so it can be copied into the backend. The
// commit is one sequential pass, which is all vfs.Create can accept.
func (s *stagedFile) Reader() (io.Reader, error) {
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return s.f, nil
}

// LoadFrom replaces the staged contents with r, leaving the file exactly as
// long as what was read: a shorter source must not leave the tail of a longer
// previous version behind.
func (s *stagedFile) LoadFrom(ctx context.Context, b *bridge, r vfs.ReadAtCloser) error {
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, spoolChunk)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return errClosed
		}
		n, readErr := r.Read(ctx, buf)
		b.mu.Unlock()
		if n > 0 {
			if _, err := s.f.Write(buf[:n]); err != nil {
				return err
			}
			total += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return s.f.Truncate(total)
}

func (s *stagedFile) Close() error {
	return s.f.Close()
}

type dirCacheEntry struct {
	items []vfs.VFSItem
	at    time.Time
}

func newBridge(v vfs.VFS, root string, opts Options) *bridge {
	ttl := opts.DirCacheTTL
	if ttl <= 0 {
		ttl = defaultDirCacheTTL
	}
	return &bridge{
		v:        v,
		root:     root,
		randomOK: v.GetCapabilities().HasRandomAccess,
		readOnly: opts.ReadOnly,
		writeOK:  v.GetCapabilities().HasWrite,
		cacheTTL: ttl,
		dirCache: make(map[string]dirCacheEntry),
		writers:  make(map[string]*writeHandle),
		statsOn:  os.Getenv("F4_FUSE_STATS") != "",
		stats:    make(map[string]*opStat),
	}
}

// close releases the VFS. Open file handles keep their own spooled copies,
// so they stay readable until the kernel releases them.
func (b *bridge) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	v := b.v
	b.mu.Unlock()

	if v != nil {
		v.Close()
	}
	b.cacheMu.Lock()
	b.dirCache = make(map[string]dirCacheEntry)
	b.cacheMu.Unlock()
}

// join maps a name inside dirPath to a VFS-native path. Paths are whatever
// the backend uses; only the VFS knows how to build them.
func (b *bridge) join(dirPath, name string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return path.Join(dirPath, name)
	}
	return b.v.Join(dirPath, name)
}

func (b *bridge) readDir(ctx context.Context, dirPath string) ([]vfs.VFSItem, error) {
	if items, ok := b.cachedDir(dirPath); ok {
		return items, nil
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errClosed
	}
	var items []vfs.VFSItem
	doneList := b.track("ReadDir")
	err := b.v.ReadDir(ctx, dirPath, func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	})
	doneList()
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}

	filtered := items[:0]
	for _, item := range items {
		if item.Name == "" || item.Name == "." || item.Name == ".." {
			continue
		}
		filtered = append(filtered, item)
	}
	items = filtered

	b.cacheMu.Lock()
	b.dirCache[dirPath] = dirCacheEntry{items: items, at: time.Now()}
	b.cacheMu.Unlock()
	return items, nil
}

func (b *bridge) cachedDir(dirPath string) ([]vfs.VFSItem, bool) {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()
	entry, ok := b.dirCache[dirPath]
	if !ok || time.Since(entry.at) > b.cacheTTL {
		return nil, false
	}
	return entry.items, true
}

func (b *bridge) invalidate(dirPath string) {
	b.cacheMu.Lock()
	delete(b.dirCache, dirPath)
	b.cacheMu.Unlock()
}

// stat answers about one path. The root of a mount is reported as a
// directory even when the backend cannot stat it: a VFS that only lists is
// still perfectly mountable.
func (b *bridge) stat(ctx context.Context, itemPath string) (vfs.VFSItem, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return vfs.VFSItem{}, errClosed
	}
	done := b.track("Stat")
	item, err := b.v.Stat(ctx, itemPath)
	done()
	b.mu.Unlock()
	if err == nil {
		return item, nil
	}
	if itemPath == b.root {
		return vfs.VFSItem{Name: displayName(itemPath), IsDir: true, MTime: time.Now()}, nil
	}
	return vfs.VFSItem{}, err
}

// lookup resolves one name inside a directory, preferring the listing the
// kernel just walked over a fresh per-name round trip.
// mkdir is the first write the bridge does. It drops the parent's cached
// listing on the way out: without that, `mkdir x && ls` would not show x for
// as long as the cache lives, which reads as the mount having ignored the
// command.
func (b *bridge) mkdir(ctx context.Context, dirPath, name string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	err := b.v.MkDir(ctx, b.join(dirPath, name))
	b.mu.Unlock()
	b.invalidate(dirPath)
	return err
}

// remove deletes one entry. Like mkdir it drops the parent's cached listing,
// so a file the kernel just unlinked stops being offered by the next ls.
func (b *bridge) remove(ctx context.Context, dirPath, name string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	err := b.v.Remove(ctx, b.join(dirPath, name))
	b.mu.Unlock()
	b.invalidate(dirPath)
	return err
}

// rename moves one entry, possibly into another directory. Both listings are
// dropped: the entry has to stop appearing in the old one and start appearing
// in the new one, and a cache that lags either way looks like a lost file.
func (b *bridge) rename(ctx context.Context, oldDir, oldName, newDir, newName string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	err := b.v.Rename(ctx, b.join(oldDir, oldName), b.join(newDir, newName))
	b.mu.Unlock()
	b.invalidate(oldDir)
	if newDir != oldDir {
		b.invalidate(newDir)
	}
	return err
}

func (b *bridge) lookup(ctx context.Context, dirPath, name string) (vfs.VFSItem, error) {
	if items, ok := b.cachedDir(dirPath); ok {
		for _, item := range items {
			if item.Name == name {
				return item, nil
			}
		}
		return vfs.VFSItem{}, os.ErrNotExist
	}

	child := b.join(dirPath, name)
	if item, err := b.stat(ctx, child); err == nil {
		if item.Name == "" {
			item.Name = name
		}
		return item, nil
	}

	items, err := b.readDir(ctx, dirPath)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

// handle is one open file. Backends without random access are spooled to a
// temporary file once, so that a mount never turns a seek into a re-read of
// the whole object.
type handle struct {
	b    *bridge
	mu   sync.Mutex
	r    vfs.ReadAtCloser
	tmp  *os.File
	path string
	size int64
	done bool
}

func (b *bridge) open(ctx context.Context, itemPath string, size int64) (*handle, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errClosed
	}
	doneOpen := b.track("Open")
	reader, err := b.v.Open(ctx, itemPath)
	doneOpen()
	random := b.randomOK
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}

	h := &handle{b: b, r: reader, path: itemPath, size: size}
	if reader.Size() > 0 {
		h.size = reader.Size()
	}
	if random {
		return h, nil
	}

	if err := h.spool(ctx); err != nil {
		h.release()
		return nil, err
	}
	return h, nil
}

// spool copies a sequential-only source into a temporary file. It holds the
// bridge lock for the whole transfer because the VFS cannot serve anything
// else meanwhile anyway.
func (h *handle) spool(ctx context.Context) error {
	tmp, err := os.CreateTemp("", "f4-fuse-*")
	if err != nil {
		return err
	}
	os.Remove(tmp.Name())

	buf := make([]byte, spoolChunk)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			tmp.Close()
			return err
		}
		// The lock is taken per chunk rather than held for the whole
		// transfer. Backends are still one conversation at a time, but a
		// 4 GiB download no longer means the mount answers nothing for
		// the length of it: every other request gets its turn between
		// chunks. Writing to the local spool needs no lock at all.
		h.b.mu.Lock()
		if h.b.closed {
			h.b.mu.Unlock()
			tmp.Close()
			return errClosed
		}
		doneChunk := h.b.track("Read(spool)")
		n, readErr := h.r.Read(ctx, buf)
		doneChunk()
		h.b.mu.Unlock()
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				tmp.Close()
				return err
			}
			total += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmp.Close()
			return readErr
		}
		if n == 0 {
			break
		}
	}

	h.tmp = tmp
	h.size = total

	// The source has been consumed to the end; releasing it now lets the
	// backend drop its own temporary state while the mount keeps serving.
	if h.r != nil {
		h.r.Close()
		h.r = nil
	}
	return nil
}

// readAt fills dest and returns the number of bytes read. A short read at
// the end of the file is success, not an error: FUSE has no EOF.
func (h *handle) readAt(ctx context.Context, dest []byte, off int64) (int, error) {
	h.mu.Lock()
	tmp := h.tmp
	reader := h.r
	closed := h.done
	h.mu.Unlock()
	if closed {
		return 0, errClosed
	}

	if tmp != nil {
		n, err := tmp.ReadAt(dest, off)
		if err == io.EOF {
			err = nil
		}
		return n, err
	}

	h.b.mu.Lock()
	defer h.b.mu.Unlock()
	if h.b.closed {
		return 0, errClosed
	}
	doneRead := h.b.track("ReadAt")
	n, err := reader.ReadAt(ctx, dest, off)
	doneRead()
	if err == io.EOF {
		err = nil
	}
	return n, err
}

func (h *handle) release() error {
	h.mu.Lock()
	if h.done {
		h.mu.Unlock()
		return nil
	}
	h.done = true
	reader, tmp := h.r, h.tmp
	h.r, h.tmp = nil, nil
	h.mu.Unlock()

	if tmp != nil {
		tmp.Close()
	}
	if reader != nil {
		h.b.mu.Lock()
		if !h.b.closed {
			reader.Close()
		}
		h.b.mu.Unlock()
	}
	return nil
}

// inodeOf derives a stable inode number from a path. Real inode numbers are
// not available from most backends, and the kernel only needs uniqueness
// among live objects.
func inodeOf(itemPath string) uint64 {
	sum := fnv.New64a()
	sum.Write([]byte(itemPath))
	ino := sum.Sum64()
	switch ino {
	case 0, 1, ^uint64(0):
		return 2
	}
	return ino
}

// unixMode turns VFS metadata into a POSIX mode. Backends that carry real
// permissions win; the rest get sane read-only defaults.
func unixMode(item vfs.VFSItem) uint32 {
	if item.UnixMode != 0 {
		return item.UnixMode
	}
	if item.IsDir {
		return 0o555
	}
	if item.IsExecutable {
		return 0o555
	}
	return 0o444
}

// displayName strips a directory prefix a backend may have included.
func displayName(name string) string {
	name = strings.TrimSuffix(name, "/")
	if idx := strings.LastIndexAny(name, "/\\"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

// acquireWriter returns the handle for path, creating it if this is the first
// writer. created says which happened, so the caller knows whether it still
// has to prepare the staging file that chunk 2 adds.
func (b *bridge) acquireWriter(path string) (h *writeHandle, created bool) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	if b.writers == nil {
		b.writers = make(map[string]*writeHandle)
	}
	if existing, ok := b.writers[path]; ok {
		existing.refs++
		return existing, false
	}
	h = &writeHandle{path: path, refs: 1}
	b.writers[path] = h
	return h, true
}

// releaseWriter drops one reference and reports whether that was the last
// one — the moment the file has to be committed to the backend.
func (b *bridge) releaseWriter(h *writeHandle) (last bool) {
	if h == nil {
		return false
	}
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	h.refs--
	if h.refs > 0 {
		return false
	}
	delete(b.writers, h.path)
	return true
}

// acquireStagedWriter is acquireWriter plus the staging file, created under
// the same lock. Doing it in one step matters: two processes creating the
// same file at once must not race over who has a staging file yet.
func (b *bridge) acquireWriteHandle(ctx context.Context, itemPath string) (h *writeHandle, created bool, err error) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	if b.writers == nil {
		b.writers = make(map[string]*writeHandle)
	}
	if existing, ok := b.writers[itemPath]; ok {
		existing.refs++
		return existing, false, nil
	}

	// A backend that can write at an offset gets written at an offset. The
	// staging file is the fallback, not the design.
	if rw, ok := b.v.(vfs.RandomWriteVFS); ok {
		b.mu.Lock()
		closed := b.closed
		var f vfs.WriterAtCloser
		if !closed {
			f, err = rw.OpenWriteAt(ctx, itemPath)
		}
		b.mu.Unlock()
		if closed {
			return nil, false, errClosed
		}
		if err != nil {
			// Staging would not rescue this: committing goes through
			// Create on the same backend and would fail the same way,
			// only later and after a pointless download.
			return nil, false, err
		}
		h = &writeHandle{path: itemPath, refs: 1, direct: f}
		b.writers[itemPath] = h
		return h, true, nil
	}

	staged, err := newStagedFile()
	if err != nil {
		return nil, false, err
	}
	h = &writeHandle{path: itemPath, refs: 1, staged: staged}
	b.writers[itemPath] = h
	return h, true, nil
}

// loadStaged fills a staging file with what the backend already holds. This
// is the read-modify-write an editor's save needs: FUSE may write two bytes
// in the middle of a file, and the commit sends the whole thing back, so what
// was not overwritten has to be there to send.
//
// It is also the one place where writing costs a full download. Backends that
// can write at an offset natively are iteration 5.
func (b *bridge) loadStaged(ctx context.Context, itemPath string, s *stagedFile) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	rc, err := b.v.Open(ctx, itemPath)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	defer rc.Close()
	// Like the read spool: the lock is taken per chunk, so opening a large
	// file for writing does not stall every other request for the length of
	// the download.
	return s.LoadFrom(ctx, b, rc)
}

// isLastWriter reports whether h is the only handle left on its file, which
// is how a close knows it is the one that has to commit.
func (b *bridge) isLastWriter(h *writeHandle) bool {
	if h == nil {
		return false
	}
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	return h.refs <= 1
}

// commit hands the staged file to the backend in one sequential pass, which
// is all vfs.Create can accept. The parent listing is dropped afterwards, or
// cp a b && ls would not show b.
func (b *bridge) commit(ctx context.Context, itemPath string, r io.Reader) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	w, err := b.v.Create(ctx, itemPath)
	if err == nil {
		_, err = io.Copy(w, r)
		if cerr := w.Close(); err == nil {
			err = cerr
		}
	}
	b.mu.Unlock()
	b.invalidate(path.Dir(itemPath))
	return err
}

// setAttributes changes metadata. vfs.VFS takes a whole VFSItem, so only the
// fields the kernel actually asked about are filled in: a zero UnixMode or a
// zero time means "leave it alone" on the backend side, which is why chmod
// and touch can be answered separately without one clobbering the other.
func (b *bridge) setAttributes(ctx context.Context, itemPath string, item vfs.VFSItem) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	err := b.v.SetAttributes(ctx, itemPath, item)
	b.mu.Unlock()
	b.invalidate(path.Dir(itemPath))
	return err
}

// readlink asks the backend what a link points at. A backend with no links
// answers "not supported", which is how a link ends up presented as an
// ordinary file — the behaviour every mount had before this existed.
func (b *bridge) readlink(ctx context.Context, itemPath string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", errClosed
	}
	sl, ok := b.v.(vfs.SymlinkVFS)
	if !ok {
		return "", errNoSymlinks
	}
	return sl.Readlink(ctx, itemPath)
}

// symlink creates a link. tar -x makes symlinks, so an extraction into a
// mount that cannot make them simply fails — which is the honest outcome, and
// far better than an extraction that quietly produces regular files.
func (b *bridge) symlink(ctx context.Context, target, dirPath, name string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errClosed
	}
	sl, ok := b.v.(vfs.SymlinkVFS)
	if !ok {
		b.mu.Unlock()
		return errNoSymlinks
	}
	err := sl.Symlink(ctx, target, b.join(dirPath, name))
	b.mu.Unlock()
	b.invalidate(dirPath)
	return err
}

// parentOf is the directory an item lives in, in VFS path terms.
func (b *bridge) parentOf(itemPath string) string { return path.Dir(itemPath) }

// writerFor reports the open write handle for path, if there is one. A read
// of a file being written has to see the staged copy rather than the version
// the backend still holds.
func (b *bridge) writerFor(path string) *writeHandle {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	return b.writers[path]
}

// openWriters is how many files are being written right now.
func (b *bridge) openWriters() int {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	return len(b.writers)
}
