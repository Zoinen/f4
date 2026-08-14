package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/archives"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/zipper/archive"

	"github.com/unxed/tar"
	"github.com/unxed/zip"

	"github.com/unxed/vtui"
)

var TestSkipDelay time.Duration

type dummyDirInfo struct {
	name string
}

func (d dummyDirInfo) Name() string       { return d.name }
func (d dummyDirInfo) Size() int64        { return 0 }
func (d dummyDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0755 }
func (d dummyDirInfo) ModTime() time.Time { return time.Now() }
func (d dummyDirInfo) IsDir() bool        { return true }
func (d dummyDirInfo) Sys() any           { return nil }

type ctxReader struct {
	r   vfs.ReadAtCloser
	ctx context.Context
}

func (cr ctxReader) Read(p []byte) (int, error) {
	return cr.r.Read(cr.ctx, p)
}

type readerAtAdapter struct {
	r   vfs.ReadAtCloser
	ctx context.Context
}

func (a readerAtAdapter) ReadAt(p []byte, off int64) (int, error) {
	return a.r.ReadAt(a.ctx, p, off)
}

type nopWriteCloser struct {
	io.Writer
}

func (n *nopWriteCloser) Close() error { return nil }

type ArchiveVFS struct {
	mu          sync.Mutex
	parent      vfs.VFS
	arcPath     string
	backingPath string
	format      string
	innerPath   string

	fsys   archive.FileSystem
	closer io.Closer

	activeCount  int
	isClosed     bool
	cleanupTimer *time.Timer
}

func (v *ArchiveVFS) IsAtRoot() bool {
	return v.innerPath == "." || v.innerPath == ""
}

func (v *ArchiveVFS) activePath() string {
	if v.backingPath != "" {
		return v.backingPath
	}
	if osvfs, ok := v.parent.(*vfs.OSVFS); ok {
		absPath, _ := osvfs.Abs(v.arcPath)
		return absPath
	}
	return v.arcPath
}

func (v *ArchiveVFS) ensureFSLocked() error {
	if v.fsys != nil {
		return nil
	}
	if v.cleanupTimer == nil || v.activePath() == "" {
		return fmt.Errorf("archive VFS is closed")
	}
	reopened, err := archive.OpenFS(v.activePath(), archive.Options{})
	if err != nil {
		return err
	}
	v.fsys = reopened
	return nil
}

func (v *ArchiveVFS) cancelCleanupLocked() {
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}
}

func (v *ArchiveVFS) finishNonHandleOperationLocked() {
	if v.isClosed && v.activeCount == 0 && (v.fsys != nil || v.closer != nil) {
		v.startCleanupTimer()
	}
}

func NewArchiveVFS(parent vfs.VFS, path string) (*ArchiveVFS, error) {
	return NewArchiveVFSContext(context.Background(), parent, path)
}

func NewArchiveVFSContext(ctx context.Context, parent vfs.VFS, archivePath string) (*ArchiveVFS, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, fmt.Errorf("archive parent VFS is nil")
	}

	canonicalPath := cleanArchiveRootPath(archivePath)
	displayName := parent.Base(archivePath)
	if displayName == "" {
		displayName = path.Base(strings.ReplaceAll(archivePath, "\\", "/"))
	}
	format := archive.DetectFormat(displayName)
	var finalPath string
	var closer io.Closer
	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		var err error
		finalPath, err = osvfs.Abs(archivePath)
		if err != nil {
			return nil, err
		}
	} else {
		lease, err := acquireArchiveMaterialization(ctx, parent, archivePath, displayName)
		if err != nil {
			return nil, err
		}
		finalPath = lease.Path()
		closer = lease
	}

	fsys, cleanupTransferred, err := openArchiveFSWithContext(ctx, finalPath, displayName, closer)
	if err != nil {
		if closer != nil && !cleanupTransferred {
			_ = closer.Close()
		}
		return nil, err
	}

	return &ArchiveVFS{
		parent: parent, arcPath: canonicalPath, backingPath: finalPath, format: format,
		innerPath: ".", fsys: fsys, closer: closer,
	}, nil
}

type archiveFSOpenResult struct {
	fsys archive.FileSystem
	err  error
}

func openArchiveFSWithContext(ctx context.Context, localPath, displayName string, backing io.Closer) (archive.FileSystem, bool, error) {
	result := make(chan archiveFSOpenResult, 1)
	go func() {
		fsys, err := archive.OpenFS(localPath, archive.Options{})
		result <- archiveFSOpenResult{fsys: fsys, err: err}
	}()

	update, reporter := archiveProgressTargets(ctx)
	ticker := time.NewTicker(ProgressTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case opened := <-result:
			if err := archiveOperationCancelled(ctx, reporter); err != nil {
				if opened.fsys != nil {
					_ = opened.fsys.Close()
				}
				return nil, false, err
			}
			if opened.err == nil {
				if update != nil {
					update("Opening archive...", 100)
				}
				if reporter != nil {
					reporter.UpdateTransfer("Opening", displayName, 100, "Archive ready", 100, "")
				}
			}
			return opened.fsys, false, opened.err
		case <-ctx.Done():
			// OpenFS has no context-aware entry point. Return cancellation now,
			// then close its late result so neither a decoder nor an fd leaks.
			go func() {
				opened := <-result
				if opened.fsys != nil {
					_ = opened.fsys.Close()
				}
				if backing != nil {
					_ = backing.Close()
				}
			}()
			return nil, backing != nil, ctx.Err()
		case <-ticker.C:
			if err := archiveOperationCancelled(ctx, reporter); err != nil {
				go func() {
					opened := <-result
					if opened.fsys != nil {
						_ = opened.fsys.Close()
					}
					if backing != nil {
						_ = backing.Close()
					}
				}()
				return nil, backing != nil, err
			}
			if update != nil {
				update("Opening archive...", -1)
			}
			if reporter != nil {
				reporter.UpdateTransfer("Opening", displayName, -1, "Reading archive index...", -1, "")
			}
		}
	}
}

func cleanArchiveRootPath(value string) string {
	if vfs.IsURIPath(value) {
		return strings.TrimRight(value, "\\/")
	}
	return filepath.Clean(value)
}

func archivePathHasPrefix(candidate, root string) bool {
	_, ok := archiveRelativePath(candidate, root)
	return ok
}

func archiveRelativePath(candidate, root string) (string, bool) {
	if vfs.IsURIPath(root) {
		if candidate == root {
			return ".", true
		}
		if !strings.HasPrefix(candidate, root) || len(candidate) <= len(root) {
			return "", false
		}
		next := candidate[len(root)]
		if next != '/' && next != '\\' {
			return "", false
		}
		return strings.TrimLeft(candidate[len(root):], "\\/"), true
	}
	cleanRoot := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(root, "\\", "/")))
	cleanCandidate := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(candidate, "\\", "/")))
	relative, err := filepath.Rel(cleanRoot, cleanCandidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func archivePathJoin(root, inner string) string {
	inner = strings.TrimLeft(strings.ReplaceAll(inner, "\\", "/"), "/")
	inner = path.Clean(inner)
	if inner == "." || inner == "" {
		return root
	}
	if vfs.IsURIPath(root) {
		return strings.TrimRight(root, "\\/") + "/" + inner
	}
	return filepath.Join(root, filepath.FromSlash(inner))
}

func (v *ArchiveVFS) resolveInnerPath(candidate string) (string, error) {
	root := v.arcPath
	if candidate == "" || candidate == "." {
		if v.innerPath == "" {
			return ".", nil
		}
		return v.innerPath, nil
	}
	if relative, owned := archiveRelativePath(candidate, root); owned {
		if relative == "." {
			return ".", nil
		}
		return cleanArchiveInnerPath(relative)
	}
	if vfs.IsURIPath(candidate) || filepath.IsAbs(candidate) || path.IsAbs(candidate) || filepath.VolumeName(candidate) != "" {
		return "", fmt.Errorf("path escapes archive: %s", candidate)
	}
	inner := path.Join(v.innerPath, strings.ReplaceAll(candidate, "\\", "/"))
	return cleanArchiveInnerPath(inner)
}

func cleanArchiveInnerPath(inner string) (string, error) {
	inner = path.Clean(strings.TrimLeft(strings.ReplaceAll(inner, "\\", "/"), "/"))
	if inner == "" || inner == "." {
		return ".", nil
	}
	if inner == ".." || strings.HasPrefix(inner, "../") {
		return "", fmt.Errorf("path escapes archive root")
	}
	return inner, nil
}

// cleanArchiveExtractionPath converts an archive member name to the only form
// which may be matched or joined to an extraction destination. Archive member
// names are untrusted: filepath.Join would otherwise let an entry such as
// "folder/../../outside" escape the selected destination.
func cleanArchiveExtractionPath(name string) (string, error) {
	if strings.IndexByte(name, 0) >= 0 {
		return "", fmt.Errorf("archive entry path contains NUL")
	}

	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || hasWindowsArchiveVolume(normalized) {
		return "", fmt.Errorf("archive entry path is absolute")
	}

	cleaned := path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("archive entry path escapes archive root")
	}
	return cleaned, nil
}

func hasWindowsArchiveVolume(name string) bool {
	return len(name) >= 2 && name[1] == ':' &&
		((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z'))
}

func archiveExtractionPathSelected(name string, selected map[string]bool) bool {
	for selectedPath := range selected {
		if selectedPath == "." || name == selectedPath || strings.HasPrefix(name, selectedPath+"/") {
			return true
		}
	}
	return false
}

func archiveExtractionRelativePath(name, innerPath string) (string, error) {
	cleanInner, err := cleanArchiveExtractionPath(innerPath)
	if err != nil {
		return "", err
	}
	if cleanInner == "." {
		return name, nil
	}
	if name == cleanInner {
		return ".", nil
	}
	prefix := cleanInner + "/"
	if !strings.HasPrefix(name, prefix) {
		return "", fmt.Errorf("archive entry path is outside the current archive folder")
	}
	return strings.TrimPrefix(name, prefix), nil
}

func archiveExtractionTarget(dstVfs vfs.VFS, dstDir, name, innerPath string) (string, error) {
	relative, err := archiveExtractionRelativePath(name, innerPath)
	if err != nil {
		return "", err
	}
	relative, err = cleanArchiveExtractionPath(relative)
	if err != nil {
		return "", err
	}

	target := dstVfs.Join(dstDir, filepath.FromSlash(relative))
	baseAbs, err := dstVfs.Abs(dstDir)
	if err != nil {
		return "", fmt.Errorf("resolve archive extraction destination: %w", err)
	}
	targetAbs, err := dstVfs.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve archive extraction target: %w", err)
	}
	resolvedRelative, inside := archiveRelativePath(targetAbs, baseAbs)
	if !inside || (resolvedRelative == "." && relative != ".") {
		return "", fmt.Errorf("archive entry path escapes extraction destination")
	}
	return target, nil
}

func (v *ArchiveVFS) GetPath() string {
	if v.innerPath == "." || v.innerPath == "" {
		return archivePathJoin(v.arcPath, ".")
	}
	// Мы возвращаем нативный путь ОС, объединяя путь к архиву и внутренний путь
	return archivePathJoin(v.arcPath, v.innerPath)
}
func (v *ArchiveVFS) IsAbs(candidate string) bool {
	return archivePathHasPrefix(candidate, v.arcPath) || (!vfs.IsURIPath(candidate) && (filepath.IsAbs(candidate) || path.IsAbs(candidate)))
}

func (v *ArchiveVFS) SetPath(p string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	defer v.finishNonHandleOperationLocked()
	if err := v.ensureFSLocked(); err != nil {
		return err
	}
	v.cancelCleanupLocked()

	newInner, err := v.resolveInnerPath(p)
	if err != nil {
		return err
	}

	if v.fsys != nil && newInner != "." {
		info, err := fs.Stat(v.fsys, newInner)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", newInner)
		}
	}

	v.innerPath = newInner
	return nil
}

func (v *ArchiveVFS) ReadDir(ctx context.Context, path string, onChunk func([]vfs.VFSItem)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	v.mu.Lock()
	if err := v.ensureFSLocked(); err != nil {
		v.mu.Unlock()
		return err
	}
	v.cancelCleanupLocked()
	closedView := v.isClosed
	fsPath, pathErr := v.resolveInnerPath(path)
	if pathErr != nil {
		v.finishNonHandleOperationLocked()
		v.mu.Unlock()
		return pathErr
	}

	entries, err := fs.ReadDir(v.fsys, fsPath)
	if err != nil {
		v.finishNonHandleOperationLocked()
		v.mu.Unlock()
		return err
	}

	items := make([]vfs.VFSItem, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		name := e.Name()

		items = append(items, vfs.VFSItem{
			Name:     name,
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			MTime:    info.ModTime(),
			IsHidden: strings.HasPrefix(name, "."),
		})
	}
	if closedView {
		v.finishNonHandleOperationLocked()
	}
	v.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (v *ArchiveVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	if err := ctx.Err(); err != nil {
		return vfs.VFSItem{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	defer v.finishNonHandleOperationLocked()
	if err := v.ensureFSLocked(); err != nil {
		return vfs.VFSItem{}, err
	}
	v.cancelCleanupLocked()

	fsPath, pathErr := v.resolveInnerPath(path)
	if pathErr != nil {
		return vfs.VFSItem{}, pathErr
	}

	info, err := fs.Stat(v.fsys, fsPath)
	if err != nil {
		return vfs.VFSItem{}, err
	}

	return vfs.VFSItem{
		Name:     info.Name(),
		IsDir:    info.IsDir(),
		Size:     info.Size(),
		MTime:    info.ModTime(),
		IsHidden: strings.HasPrefix(info.Name(), "."),
	}, nil
}

type archiveReadWrapper struct {
	v          *ArchiveVFS
	once       sync.Once
	mu         sync.Mutex
	f          fs.File
	fsPath     string
	size       int64
	tmpFile    *os.File
	tmpPath    string
	extracted  bool
	extracting bool
	doneChan   chan struct{}
	err        error
	readPos    int64
}

func (w *archiveReadWrapper) Size() int64 {
	return w.size
}

func (w *archiveReadWrapper) Close() error {
	var err error
	w.once.Do(func() {
		w.mu.Lock()
		if w.f != nil {
			w.f.Close()
			w.f = nil
		}
		if w.tmpFile != nil {
			w.tmpFile.Close()
			os.Remove(w.tmpPath)
			w.tmpFile = nil
		}
		w.mu.Unlock()
		w.v.decrementActive()
	})
	return err
}

func (w *archiveReadWrapper) TempPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tmpPath
}

func (w *archiveReadWrapper) LocalPath() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tmpPath, w.extracted && w.tmpPath != ""
}

func (w *archiveReadWrapper) extractToTemp(ctx context.Context) error {
	w.mu.Lock()
	v := w.v
	fsPath := w.fsPath
	w.mu.Unlock()

	var src io.Reader
	var srcCloser io.Closer

	w.mu.Lock()
	if seeker, ok := w.f.(io.Seeker); ok {
		_, err := seeker.Seek(0, io.SeekStart)
		if err == nil {
			src = w.f
		}
	}
	w.mu.Unlock()

	if src == nil && v != nil {
		v.mu.Lock()
		fsys := v.fsys
		v.mu.Unlock()
		if fsys != nil {
			fNew, err := fsys.Open(fsPath)
			if err == nil {
				src = fNew
				srcCloser = fNew
			}
		}
	}

	if src == nil {
		w.mu.Lock()
		src = w.f
		w.mu.Unlock()
	}

	if src == nil {
		err := fmt.Errorf("no source file available for extraction")
		w.mu.Lock()
		w.err = err
		w.mu.Unlock()
		return err
	}

	tmp, err := os.CreateTemp("", "f4arc-*")
	if err != nil {
		if srcCloser != nil {
			srcCloser.Close()
		}
		w.mu.Lock()
		w.err = err
		w.mu.Unlock()
		return err
	}

	buf := make([]byte, 128*1024)
	var loopErr error

	for {
		if ctx.Err() != nil {
			loopErr = ctx.Err()
			break
		}
		n, errRead := src.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				loopErr = werr
				break
			}
		}
		if errRead != nil {
			if errRead != io.EOF {
				loopErr = errRead
			}
			break
		}
	}

	if srcCloser != nil {
		srcCloser.Close()
	}

	w.mu.Lock()
	readPos := w.readPos
	w.mu.Unlock()

	if loopErr == nil {
		_, loopErr = tmp.Seek(readPos, io.SeekStart)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if loopErr != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		if !errors.Is(loopErr, context.Canceled) && !errors.Is(loopErr, context.DeadlineExceeded) {
			if w.f != nil {
				w.f.Close()
				w.f = nil
			}
			w.err = loopErr
		}
		return loopErr
	} else {
		if w.f != nil {
			w.f.Close()
			w.f = nil
		}
		w.tmpPath = tmp.Name()
		w.tmpFile = tmp
		w.extracted = true
		return nil
	}
}

func (w *archiveReadWrapper) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	w.mu.Lock()
	for !w.extracted && w.err == nil {
		if w.extracting {
			ch := w.doneChan
			w.mu.Unlock()
			select {
			case <-ch:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
			w.mu.Lock()
			continue
		}

		w.extracting = true
		w.doneChan = make(chan struct{})
		w.mu.Unlock()

		attemptErr := w.extractToTemp(ctx)

		w.mu.Lock()
		w.extracting = false
		close(w.doneChan)
		w.doneChan = nil
		if attemptErr != nil {
			w.mu.Unlock()
			return 0, attemptErr
		}
	}

	if w.err != nil {
		w.mu.Unlock()
		return 0, w.err
	}
	tmp := w.tmpFile
	w.mu.Unlock()

	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return tmp.ReadAt(p, off)
}

func (w *archiveReadWrapper) Read(ctx context.Context, p []byte) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	w.mu.Lock()
	if w.err != nil {
		w.mu.Unlock()
		return 0, w.err
	}
	if w.extracted {
		tmp := w.tmpFile
		w.mu.Unlock()
		return tmp.Read(p)
	}

	f := w.f
	w.mu.Unlock()

	n, err := f.Read(p)
	if n > 0 {
		w.mu.Lock()
		w.readPos += int64(n)
		w.mu.Unlock()
	}
	return n, err
}

func formatSize(b int64) string {
	if b < 0 {
		return "?"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func extractWithProgress(ctx context.Context, src io.Reader, dst io.Writer, size int64, name string, update vfs.ProgressCallback, reporter vfs.TaskReporter) error {
	buf := make([]byte, 128*1024)
	var copied int64
	startTime := time.Now()
	lastUpdate := startTime

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		dots := ""
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if atomic.LoadInt64(&copied) == 0 {
					dots += "."
					if len(dots) > 3 {
						dots = ""
					}
					msg := fmt.Sprintf("Seeking/Decompressing%s", dots)
					if update != nil {
						update(msg, -1)
					}
					if reporter != nil {
						elapsed := time.Since(startTime)
						elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
						reporter.UpdateTransfer("Extracting", name, -1, msg, -1, elapsedStr)
					}
				}
			}
		}
	}()

	report := func(current int64, percent int) {
		if update != nil {
			update(fmt.Sprintf("Extracting %s...", name), percent)
		}
		if reporter != nil {
			elapsed := time.Since(startTime)
			elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
			reporter.UpdateTransfer("Extracting", name, percent, fmt.Sprintf("Extracting: %s / %s", formatSize(current), formatSize(size)), percent, elapsedStr)
		}
	}
	report(0, 0)

	for {
		if err := archiveOperationCancelled(ctx, reporter); err != nil {
			return err
		}
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			atomic.AddInt64(&copied, int64(n))

			now := time.Now()
			if now.Sub(lastUpdate) > 50*time.Millisecond || err != nil {
				lastUpdate = now
				pct := 0
				currentCopied := atomic.LoadInt64(&copied)
				if size > 0 {
					pct = int((currentCopied * 100) / size)
					if pct > 100 {
						pct = 100
					}
				}
				report(currentCopied, pct)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	report(atomic.LoadInt64(&copied), 100)
	return nil
}

func (v *ArchiveVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v.mu.Lock()
	if err := v.ensureFSLocked(); err != nil {
		v.mu.Unlock()
		return nil, err
	}
	v.cancelCleanupLocked()
	fsPath, pathErr := v.resolveInnerPath(path)
	if pathErr != nil {
		v.mu.Unlock()
		return nil, pathErr
	}

	// Capture fsys and increment active count EARLY to protect it while unlocked.
	v.activeCount++
	fsys := v.fsys
	v.mu.Unlock()

	var update vfs.ProgressCallback
	if val := ctx.Value(vfs.ProgressKey); val != nil {
		if cb, ok := val.(vfs.ProgressCallback); ok {
			update = cb
		}
	}

	var reporter vfs.TaskReporter
	if val := ctx.Value(vfs.ReporterKey); val != nil {
		if r, ok := val.(vfs.TaskReporter); ok {
			reporter = r
		}
	}

	startTime := time.Now()
	openDone := make(chan struct{})
	go func() {
		if update == nil && reporter == nil {
			return
		}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		dots := ""
		for {
			select {
			case <-openDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				dots += "."
				if len(dots) > 3 {
					dots = ""
				}
				msg := fmt.Sprintf("Locating file%s", dots)
				if update != nil {
					update(msg, -1)
				}
				if reporter != nil {
					elapsed := time.Since(startTime)
					elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
					reporter.UpdateTransfer("Opening", v.Base(path), -1, msg, -1, elapsedStr)
				}
			}
		}
	}()
	cancelPoll := time.NewTicker(100 * time.Millisecond)
	defer cancelPoll.Stop()

	type openResult struct {
		file fs.File
		err  error
	}
	result := make(chan openResult, 1)
	go func() {
		file, err := fsys.Open(fsPath)
		result <- openResult{file: file, err: err}
	}()
	var srcFile fs.File
	var err error
	for srcFile == nil {
		select {
		case opened := <-result:
			srcFile, err = opened.file, opened.err
			if err == nil && srcFile == nil {
				err = fmt.Errorf("archive returned an empty file handle")
			}
			if err != nil {
				close(openDone)
				v.decrementActive()
				return nil, err
			}
		case <-ctx.Done():
			close(openDone)
			go func() {
				opened := <-result
				if opened.file != nil {
					_ = opened.file.Close()
				}
				v.decrementActive()
			}()
			return nil, ctx.Err()
		case <-cancelPoll.C:
			if reporter != nil && reporter.IsCancelled() {
				close(openDone)
				go func() {
					opened := <-result
					if opened.file != nil {
						_ = opened.file.Close()
					}
					v.decrementActive()
				}()
				return nil, context.Canceled
			}
		}
	}
	close(openDone)

	info, err := srcFile.Stat()
	var size int64
	if err == nil && info != nil {
		size = info.Size()
	}

	if update != nil || reporter != nil {
		tmp, errTemp := os.CreateTemp("", "f4arc-open-*")
		if errTemp != nil {
			srcFile.Close()
			v.decrementActive()
			return nil, errTemp
		}

		fileName := "unknown"
		if info != nil {
			fileName = info.Name()
		}
		errExtract := extractWithProgress(ctx, srcFile, tmp, size, fileName, update, reporter)
		srcFile.Close()

		if errExtract != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			v.decrementActive()
			return nil, errExtract
		}

		tmp.Seek(0, 0)

		return &archiveReadWrapper{
			v:         v,
			size:      size,
			tmpFile:   tmp,
			tmpPath:   tmp.Name(),
			extracted: true,
		}, nil
	}

	return &archiveReadWrapper{
		v:      v,
		f:      srcFile,
		fsPath: fsPath,
		size:   size,
	}, nil
}

func (v *ArchiveVFS) ParentVFS() vfs.VFS { return v.parent }
func (v *ArchiveVFS) Join(elements ...string) string {
	if len(elements) == 0 {
		return ""
	}
	if relative, owned := archiveRelativePath(elements[0], v.arcPath); owned {
		inner := relative
		for _, element := range elements[1:] {
			inner = path.Join(inner, strings.ReplaceAll(element, "\\", "/"))
		}
		clean, err := cleanArchiveInnerPath(inner)
		if err != nil {
			return v.arcPath
		}
		return archivePathJoin(v.arcPath, clean)
	}
	if vfs.IsURIPath(elements[0]) {
		joined := elements[0]
		for _, element := range elements[1:] {
			if element == "" || element == "." {
				continue
			}
			joined = archivePathJoin(joined, element)
		}
		return joined
	}
	return filepath.Join(elements...)
}
func (v *ArchiveVFS) Abs(candidate string) (string, error) {
	if _, owned := archiveRelativePath(candidate, v.arcPath); owned {
		return candidate, nil
	}
	if vfs.IsURIPath(candidate) {
		return "", fmt.Errorf("path escapes archive: %s", candidate)
	}
	if filepath.IsAbs(candidate) || path.IsAbs(candidate) {
		return filepath.Clean(candidate), nil
	}
	return filepath.Clean(v.Join(v.GetPath(), candidate)), nil
}
func (v *ArchiveVFS) Base(candidate string) string {
	if candidate == v.arcPath {
		return v.parent.Base(v.arcPath)
	}
	if relative, owned := archiveRelativePath(candidate, v.arcPath); owned {
		return path.Base(relative)
	}
	return filepath.Base(candidate)
}
func (v *ArchiveVFS) Dir(candidate string) string {
	if candidate == v.arcPath {
		return v.parent.Dir(v.arcPath)
	}
	if relative, owned := archiveRelativePath(candidate, v.arcPath); owned {
		parent := path.Dir(relative)
		return archivePathJoin(v.arcPath, parent)
	}
	return filepath.Dir(candidate)
}

func (v *ArchiveVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, local := v.parent.(*vfs.OSVFS); !local {
		return nil, fmt.Errorf("remote and nested archives are read-only")
	}
	v.mu.Lock()
	if err := v.ensureFSLocked(); err != nil {
		v.mu.Unlock()
		return nil, err
	}
	v.cancelCleanupLocked()

	fsPath, pathErr := v.resolveInnerPath(path)
	if pathErr != nil || fsPath == "." {
		v.mu.Unlock()
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, fmt.Errorf("cannot replace archive root")
	}

	tmp, err := os.CreateTemp("", "f4arc-write-*")
	if err != nil {
		v.mu.Unlock()
		return nil, err
	}
	v.activeCount++
	v.mu.Unlock()

	return &archiveWriteWrapper{v: v, tmpFile: tmp, destPath: fsPath}, nil
}

type archiveWriteWrapper struct {
	v        *ArchiveVFS
	tmpFile  *os.File
	destPath string
	once     sync.Once
}

func (w *archiveWriteWrapper) Write(p []byte) (n int, err error) { return w.tmpFile.Write(p) }
func (w *archiveWriteWrapper) Close() error {
	var err error
	w.once.Do(func() {
		w.tmpFile.Close()
		tmpName := w.tmpFile.Name()
		defer os.Remove(tmpName)

		w.v.mu.Lock()
		isClosed := w.v.isClosed
		w.v.mu.Unlock()

		if !isClosed {
			upd, errUpd := archive.NewUpdater(w.v.activePath(), archive.Options{})
			if errUpd == nil {
				defer upd.Close()
				w.tmpFile, err = os.Open(tmpName)
				if err == nil {
					defer w.tmpFile.Close()
					stat, _ := w.tmpFile.Stat()
					err = upd.Append(w.destPath, stat.Size(), w.tmpFile)
					if err == nil {
						w.v.reloadFS()
					}
				}
			} else {
				err = errUpd
			}
		} else {
			err = fmt.Errorf("archive VFS was closed")
		}
		w.v.decrementActive()
	})
	return err
}

func (v *ArchiveVFS) MkDir(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, local := v.parent.(*vfs.OSVFS); !local {
		return fmt.Errorf("remote and nested archives are read-only")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	defer v.finishNonHandleOperationLocked()
	if err := v.ensureFSLocked(); err != nil {
		return err
	}
	v.cancelCleanupLocked()

	fsPath, pathErr := v.resolveInnerPath(path)
	if pathErr != nil {
		return pathErr
	}
	if fsPath == "." {
		return fmt.Errorf("cannot create archive root")
	}

	if !strings.HasSuffix(fsPath, "/") {
		fsPath += "/"
	}

	upd, err := archive.NewUpdater(v.activePath(), archive.Options{})
	if err != nil {
		return err
	}
	defer upd.Close()

	err = upd.Append(fsPath, 0, nil)
	if err == nil {
		v.reloadFS()
	}
	return err
}

func (v *ArchiveVFS) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, local := v.parent.(*vfs.OSVFS); !local {
		return fmt.Errorf("remote and nested archives are read-only")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	defer v.finishNonHandleOperationLocked()
	if err := v.ensureFSLocked(); err != nil {
		return err
	}
	v.cancelCleanupLocked()

	fsPath, pathErr := v.resolveInnerPath(path)
	if pathErr != nil {
		return pathErr
	}
	if fsPath == "." {
		return fmt.Errorf("cannot remove archive root")
	}

	upd, err := archive.NewUpdater(v.activePath(), archive.Options{})
	if err != nil {
		return err
	}
	defer upd.Close()

	err = upd.Remove(fsPath)
	if err == nil {
		v.reloadFS()
	}
	return err
}

func (v *ArchiveVFS) reloadFS() {
	if v.fsys != nil {
		v.fsys.Close()
	}
	newFS, err := archive.OpenFS(v.activePath(), archive.Options{})
	if err == nil {
		v.fsys = newFS
	}
}

func (v *ArchiveVFS) Rename(ctx context.Context, o, n string) error { return fmt.Errorf("read-only") }

func (v *ArchiveVFS) SetAttributes(ctx context.Context, path string, item vfs.VFSItem) error {
	return fmt.Errorf("SetAttributes not supported for Archives yet")
}

func (v *ArchiveVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: runtime.GOOS != "windows"}
}
func (v *ArchiveVFS) Search(ctx context.Context, p, pat string) (chan int64, error) { return nil, nil }
func (v *ArchiveVFS) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.isClosed = true
	if v.activeCount > 0 {
		return nil
	}

	v.startCleanupTimer()
	return nil
}

func (v *ArchiveVFS) decrementActive() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.activeCount--
	if v.activeCount == 0 && v.isClosed {
		v.startCleanupTimer()
	}
}

func (v *ArchiveVFS) startCleanupTimer() {
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
	}
	// Release decoder file handles immediately (important on Windows), while
	// retaining the backing lease for a short grace period. A late operation
	// can reopen the decoder from the same backing without another download.
	if v.fsys != nil {
		_ = v.fsys.Close()
		v.fsys = nil
	}
	// 2-second grace period of complete inactivity
	v.cleanupTimer = time.AfterFunc(2*time.Second, func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		if v.activeCount == 0 && v.isClosed {
			v.performCleanup()
		}
	})
}

func (v *ArchiveVFS) performCleanup() error {
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}
	if v.fsys != nil {
		v.fsys.Close()
		v.fsys = nil
	}
	if v.closer != nil {
		err := v.closer.Close()
		if f, ok := v.closer.(*os.File); ok {
			os.Remove(f.Name())
		}
		v.closer = nil
		return err
	}
	return nil
}

func (v *ArchiveVFS) Clone() vfs.VFS {
	// Archive VFS is currently stateful and linked to temp files.
	// For now, return self as cloning requires extracting everything again.
	return v
}

var ProgressTickerInterval = 250 * time.Millisecond

func runProgressTicker(ctx context.Context, done chan struct{}, reporter vfs.TaskReporter, getStatus func() (action, file string, pct int)) {
	if reporter == nil {
		return
	}
	ticker := time.NewTicker(ProgressTickerInterval)
	defer ticker.Stop()
	dots := ""
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			dots += "."
			if len(dots) > 3 {
				dots = ""
			}
			act, file, pct := getStatus()
			if act == "Locating" {
				reporter.UpdateTransfer(act, file+dots, pct, "", pct, "")
			} else if act != "" {
				reporter.UpdateTransfer(act, file, pct, "", pct, "")
			}
		}
	}
}
func (v *ArchiveVFS) CopyBulk(ctx context.Context, srcPaths []string, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
	if err := archiveOperationCancelled(ctx, reporter); err != nil {
		return err
	}
	v.mu.Lock()
	if err := v.ensureFSLocked(); err != nil {
		v.mu.Unlock()
		return err
	}
	v.cancelCleanupLocked()
	v.activeCount++
	innerPath := v.innerPath
	absPath := v.activePath()
	v.mu.Unlock()
	defer v.decrementActive()

	// Create a map of selected paths for fast O(1) lookup
	selectedMap := make(map[string]bool)
	for _, p := range srcPaths {
		fullInner := strings.ReplaceAll(p, "\\", "/")
		if innerPath != "." && innerPath != "" {
			fullInner = path.Join(innerPath, fullInner)
		}
		cleanSelected, err := cleanArchiveExtractionPath(fullInner)
		if err != nil {
			return fmt.Errorf("unsafe selected archive path %q: %w", p, err)
		}
		selectedMap[cleanSelected] = true
	}

	waitLock := true
	if !vfs.GlobalArchiveLockManager.TryLock(absPath) {
		// If "AutoQueue" is requested via Context (used by headless unit tests), bypass the UI prompt
		if ctx.Value("AutoQueue") != nil {
			waitLock = true
		} else if vtui.FrameManager == nil {
			// Fallback headless mode
			waitLock = true
		} else {
			resChan := make(chan int, 1)
			vtui.FrameManager.PostTask(func() {
				dlg := vtui.ShowMessage(" Archive Busy ", "This archive is currently being processed.\nRunning multiple operations simultaneously may severely degrade performance.", []string{"&Queue", "&Parallel", "&Cancel"})
				dlg.OnResult = func(c int) { resChan <- c }
			})
			res := <-resChan
			if res == 2 || res < 0 {
				return context.Canceled
			}
			waitLock = (res == 0)
		}
	} else {
		vfs.GlobalArchiveLockManager.Unlock(absPath)
	}

	if waitLock {
		if reporter != nil {
			reporter.UpdateTransfer("Waiting", v.Base(absPath), -1, "Waiting in queue...", -1, "")
		}
		vfs.GlobalArchiveLockManager.Lock(absPath)
		defer vfs.GlobalArchiveLockManager.Unlock(absPath)
	}

	archiveFile, err := v.openArchiveFile(ctx)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	format := v.format
	if format == "" {
		format = archive.DetectFormat(v.Base(v.arcPath))
	}
	if format == "zip" {
		return v.copyBulkZip(ctx, archiveFile, selectedMap, innerPath, dstVfs, dstDir, reporter)
	} else if format == "tar" {
		return v.copyBulkTar(ctx, archiveFile, selectedMap, innerPath, dstVfs, dstDir, reporter)
	}
	return v.copyBulkFallback(ctx, archiveFile, selectedMap, innerPath, dstVfs, dstDir, reporter)
}

func (v *ArchiveVFS) openArchiveFile(ctx context.Context) (vfs.ReadAtCloser, error) {
	if osvfs, ok := v.parent.(*vfs.OSVFS); ok {
		absPath, _ := osvfs.Abs(v.arcPath)
		f, err := os.Open(absPath)
		if err != nil {
			return nil, err
		}
		stat, _ := f.Stat()
		return &vfs.TempFileWrapper{File: f, SizeVal: stat.Size(), TempPath: ""}, nil
	}
	return v.parent.Open(ctx, v.arcPath)
}

func (v *ArchiveVFS) copyBulkZip(ctx context.Context, f vfs.ReadAtCloser, selected map[string]bool, innerPath string, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
	zr, err := zip.NewReader(readerAtAdapter{r: f, ctx: ctx}, f.Size())
	if err != nil {
		return err
	}

	var mu sync.Mutex
	lastAction := "Locating"
	lastFile := "Archive data"
	lastPct := -1

	done := make(chan struct{})
	defer close(done)

	go runProgressTicker(ctx, done, reporter, func() (string, string, int) {
		mu.Lock()
		defer mu.Unlock()
		return lastAction, lastFile, lastPct
	})

	buf := make([]byte, 128*1024)
	for _, file := range zr.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		cleanName, err := cleanArchiveExtractionPath(file.Name)
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q: %w", file.Name, err)
		}
		if cleanName == "." {
			continue
		}
		matched := archiveExtractionPathSelected(cleanName, selected)

		if !matched {
			mu.Lock()
			lastAction = "Locating"
			lastFile = cleanName
			lastPct = -1
			mu.Unlock()
			if TestSkipDelay > 0 {
				time.Sleep(TestSkipDelay)
			}
			continue
		}

		targetPath, err := archiveExtractionTarget(dstVfs, dstDir, cleanName, innerPath)
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q: %w", file.Name, err)
		}

		if file.FileInfo().IsDir() {
			dstVfs.MkDir(ctx, targetPath)
			if fp, ok := reporter.(vfs.FileProgress); ok {
				fp.DirDone()
			}
			continue
		}

		dstVfs.MkDir(ctx, dstVfs.Dir(targetPath))

		if fp, ok := reporter.(vfs.FileProgress); ok {
			fp.StartFile(file.Name, int64(file.UncompressedSize64))
		}
		if reporter != nil {
			mu.Lock()
			lastAction = "Extracting"
			lastFile = file.Name
			lastPct = 0
			mu.Unlock()
		}

		rc, err := file.Open()
		if err != nil {
			return err
		}

		wc, err := dstVfs.Create(ctx, targetPath)
		if err != nil {
			rc.Close()
			return err
		}

		var copied int64
		for {
			if ctx.Err() != nil {
				rc.Close()
				wc.Close()
				return ctx.Err()
			}
			n, rerr := rc.Read(buf)
			if n > 0 {
				if _, werr := wc.Write(buf[:n]); werr != nil {
					rc.Close()
					wc.Close()
					return werr
				}
				if fp, ok := reporter.(vfs.FileProgress); ok {
					fp.UpdateBytes(n)
				}
				copied += int64(n)
				if reporter != nil && file.UncompressedSize64 > 0 {
					pct := int((copied * 100) / int64(file.UncompressedSize64))
					mu.Lock()
					lastAction = "Extracting"
					lastFile = file.Name
					lastPct = pct
					mu.Unlock()
				}
			}
			if rerr != nil {
				if rerr == io.EOF {
					break
				}
				rc.Close()
				wc.Close()
				return rerr
			}
		}
		rc.Close()
		wc.Close()

		if fp, ok := reporter.(vfs.FileProgress); ok {
			fp.FileDone()
		}

		mode := uint32(file.Mode().Perm())
		if mode == 0 {
			mode = 0644
		}
		item := vfs.VFSItem{
			Name:     file.Name,
			Size:     int64(file.UncompressedSize64),
			IsDir:    false,
			MTime:    file.Modified,
			ATime:    file.Modified,
			UnixMode: mode,
			Uid:      -1,
			Gid:      -1,
		}
		dstVfs.SetAttributes(ctx, targetPath, item)
	}
	return nil
}

func (v *ArchiveVFS) copyBulkTar(ctx context.Context, f vfs.ReadAtCloser, selected map[string]bool, innerPath string, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
	tr := tar.NewReader(ctxReader{r: f, ctx: ctx})
	var mu sync.Mutex
	lastAction := "Locating"
	lastFile := "Archive data"
	lastPct := -1

	done := make(chan struct{})
	defer close(done)

	go runProgressTicker(ctx, done, reporter, func() (string, string, int) {
		mu.Lock()
		defer mu.Unlock()
		return lastAction, lastFile, lastPct
	})

	buf := make([]byte, 128*1024)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanName, err := cleanArchiveExtractionPath(hdr.Name)
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q: %w", hdr.Name, err)
		}
		if cleanName == "." {
			continue
		}
		matched := archiveExtractionPathSelected(cleanName, selected)

		if !matched {
			mu.Lock()
			lastAction = "Locating"
			lastFile = cleanName
			lastPct = -1
			mu.Unlock()
			if TestSkipDelay > 0 {
				time.Sleep(TestSkipDelay)
			}
			continue
		}

		targetPath, err := archiveExtractionTarget(dstVfs, dstDir, cleanName, innerPath)
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q: %w", hdr.Name, err)
		}

		if hdr.Typeflag == tar.TypeDir {
			dstVfs.MkDir(ctx, targetPath)
			if fp, ok := reporter.(vfs.FileProgress); ok {
				fp.DirDone()
			}
			continue
		}

		dstVfs.MkDir(ctx, dstVfs.Dir(targetPath))

		if fp, ok := reporter.(vfs.FileProgress); ok {
			fp.StartFile(cleanName, hdr.Size)
		}
		if reporter != nil {
			mu.Lock()
			lastAction = "Extracting"
			lastFile = cleanName
			lastPct = 0
			mu.Unlock()
		}

		wc, err := dstVfs.Create(ctx, targetPath)
		if err != nil {
			return err
		}

		var copied int64
		for {
			if ctx.Err() != nil {
				wc.Close()
				return ctx.Err()
			}
			n, rerr := tr.Read(buf)
			if n > 0 {
				if _, werr := wc.Write(buf[:n]); werr != nil {
					wc.Close()
					return werr
				}
				if fp, ok := reporter.(vfs.FileProgress); ok {
					fp.UpdateBytes(n)
				}
				copied += int64(n)
				if reporter != nil && hdr.Size > 0 {
					pct := int((copied * 100) / hdr.Size)
					mu.Lock()
					lastAction = "Extracting"
					lastFile = cleanName
					lastPct = pct
					mu.Unlock()
				}
			}
			if rerr != nil {
				if rerr == io.EOF {
					break
				}
				wc.Close()
				return rerr
			}
		}
		wc.Close()

		if fp, ok := reporter.(vfs.FileProgress); ok {
			fp.FileDone()
		}

		mode := uint32(hdr.Mode)
		if mode == 0 {
			mode = 0644
		}
		item := vfs.VFSItem{
			Name:     hdr.Name,
			Size:     hdr.Size,
			IsDir:    false,
			MTime:    hdr.ModTime,
			ATime:    hdr.ModTime,
			UnixMode: mode,
			Uid:      -1,
			Gid:      -1,
		}
		dstVfs.SetAttributes(ctx, targetPath, item)
	}
	return nil
}

func (v *ArchiveVFS) copyBulkFallback(ctx context.Context, f vfs.ReadAtCloser, selected map[string]bool, innerPath string, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
	var localPath string
	if temp, ok := f.(*vfs.TempFileWrapper); ok && temp.TempPath != "" {
		localPath = temp.TempPath
	} else {
		localPath = v.activePath()
	}

	localF, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localF.Close()

	format, _, err := archives.Identify(ctx, localPath, localF)
	if err != nil {
		return err
	}

	ex, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format %T does not support extraction", format)
	}

	if _, err := localF.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var mu sync.Mutex
	lastAction := "Locating"
	lastFile := "Archive data"
	lastPct := -1

	done := make(chan struct{})
	defer close(done)

	go runProgressTicker(ctx, done, reporter, func() (string, string, int) {
		mu.Lock()
		defer mu.Unlock()
		return lastAction, lastFile, lastPct
	})

	return ex.Extract(ctx, localF, func(ctx context.Context, info archives.FileInfo) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		cleanName, err := cleanArchiveExtractionPath(info.NameInArchive)
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q: %w", info.NameInArchive, err)
		}
		if cleanName == "." || cleanName == "" {
			return nil
		}

		matched := archiveExtractionPathSelected(cleanName, selected)

		size := info.Size()

		if !matched {
			mu.Lock()
			lastAction = "Locating"
			lastFile = cleanName
			lastPct = -1
			mu.Unlock()
			if fp, ok := reporter.(vfs.FileProgress); ok {
				fp.StartFile(cleanName, size)
				fp.FileSkipped()
			}
			if TestSkipDelay > 0 {
				time.Sleep(TestSkipDelay)
			}
			return nil
		}

		targetPath, err := archiveExtractionTarget(dstVfs, dstDir, cleanName, innerPath)
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q: %w", info.NameInArchive, err)
		}

		if info.IsDir() {
			dstVfs.MkDir(ctx, targetPath)
			if fp, ok := reporter.(vfs.FileProgress); ok {
				fp.DirDone()
			}
			return nil
		}

		dstVfs.MkDir(ctx, dstVfs.Dir(targetPath))

		if fp, ok := reporter.(vfs.FileProgress); ok {
			fp.StartFile(cleanName, size)
		}
		if reporter != nil {
			mu.Lock()
			lastAction = "Extracting"
			lastFile = cleanName
			lastPct = 0
			mu.Unlock()
		}

		rc, err := info.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		wc, err := dstVfs.Create(ctx, targetPath)
		if err != nil {
			return err
		}
		defer wc.Close()

		buf := make([]byte, 128*1024)
		var copied int64
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n, rerr := rc.Read(buf)
			if n > 0 {
				if _, werr := wc.Write(buf[:n]); werr != nil {
					return werr
				}
				if fp, ok := reporter.(vfs.FileProgress); ok {
					fp.UpdateBytes(n)
				}
				copied += int64(n)
				if reporter != nil && size > 0 {
					pct := int((copied * 100) / size)
					mu.Lock()
					lastAction = "Extracting"
					lastFile = cleanName
					lastPct = pct
					mu.Unlock()
				}
			}
			if rerr != nil {
				if rerr == io.EOF {
					break
				}
				return rerr
			}
		}

		if fp, ok := reporter.(vfs.FileProgress); ok {
			fp.FileDone()
		}

		mode := uint32(info.Mode().Perm())
		if mode == 0 {
			mode = 0644
		}
		item := vfs.VFSItem{
			Name:     info.Name(),
			Size:     info.Size(),
			IsDir:    false,
			MTime:    info.ModTime(),
			ATime:    info.ModTime(),
			UnixMode: mode,
			Uid:      -1,
			Gid:      -1,
		}
		dstVfs.SetAttributes(ctx, targetPath, item)
		return nil
	})
}
