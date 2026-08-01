package archive

import (
	"context"
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
	mu        sync.Mutex
	parent    vfs.VFS
	arcPath   string
	innerPath string

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
	if f, ok := v.closer.(*os.File); ok {
		return f.Name()
	}
	if osvfs, ok := v.parent.(*vfs.OSVFS); ok {
		absPath, _ := osvfs.Abs(v.arcPath)
		return absPath
	}
	return v.arcPath
}

func NewArchiveVFS(parent vfs.VFS, path string) (*ArchiveVFS, error) {
	var err error
	var finalPath string
	var closer io.Closer

	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		finalPath, _ = osvfs.Abs(path)
	} else {
		rc, openErr := parent.Open(context.Background(), path)
		if openErr != nil {
			return nil, openErr
		}

		tmp, errTemp := os.CreateTemp("", "f4nested-*")
		if errTemp != nil {
			rc.Close()
			return nil, errTemp
		}
		if _, errCopy := io.Copy(tmp, ctxReader{rc, context.Background()}); errCopy != nil {
			rc.Close()
			tmp.Close()
			os.Remove(tmp.Name())
			return nil, errCopy
		}
		rc.Close()
		finalPath = tmp.Name()
		closer = tmp
	}

	fsys, err := archive.OpenFS(finalPath, archive.Options{})
	if err != nil {
		if closer != nil {
			closer.Close()
			os.Remove(finalPath)
		}
		return nil, err
	}

	return &ArchiveVFS{
		parent:    parent,
		arcPath:   path,
		innerPath: ".",
		fsys:      fsys,
		closer:    closer,
	}, nil
}

func (v *ArchiveVFS) GetPath() string {
	if v.innerPath == "." || v.innerPath == "" {
		return filepath.Clean(v.arcPath)
	}
	// Мы возвращаем нативный путь ОС, объединяя путь к архиву и внутренний путь
	return filepath.Join(v.arcPath, filepath.FromSlash(v.innerPath))
}
func (v *ArchiveVFS) IsAbs(p string) bool { return path.IsAbs(p) || strings.HasPrefix(p, v.arcPath) }

func (v *ArchiveVFS) SetPath(p string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	cleanP := filepath.ToSlash(filepath.Clean(p))
	prefix := filepath.ToSlash(filepath.Clean(v.arcPath))

	var newInner string

	if strings.HasPrefix(cleanP, prefix) {
		newInner = strings.TrimPrefix(cleanP, prefix)
	} else if filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return fmt.Errorf("path escapes archive: %s", p)
	} else {
		if v.innerPath == "" || v.innerPath == "." {
			newInner = cleanP
		} else {
			newInner = path.Join(v.innerPath, cleanP)
		}
	}

	newInner = strings.TrimPrefix(newInner, "/")
	newInner = path.Clean(newInner)

	if newInner == "" || newInner == "." {
		newInner = "."
	} else if strings.HasPrefix(newInner, "..") {
		return fmt.Errorf("path escapes archive root")
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
	v.mu.Lock()
	if v.fsys == nil {
		v.mu.Unlock()
		return fmt.Errorf("archive VFS is closed")
	}
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}
	fsPath := v.innerPath
	if path != "" && path != v.GetPath() {
		if path == v.arcPath || path == v.arcPath+"/" || path == v.arcPath+"\\" {
			fsPath = "."
		} else {
			fsPath = strings.TrimPrefix(path, v.arcPath)
			fsPath = strings.TrimPrefix(fsPath, "/")
			fsPath = strings.TrimPrefix(fsPath, "\\")
		}
	}

	normPath := filepath.ToSlash(path)
	normArcPath := filepath.ToSlash(v.arcPath)
	fsPath = "."
	if normPath != normArcPath {
		fsPath = strings.TrimPrefix(normPath, normArcPath)
		fsPath = strings.TrimPrefix(fsPath, "/")
	}

	entries, err := fs.ReadDir(v.fsys, fsPath)
	if err != nil {
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
	v.mu.Unlock()
	onChunk(items)
	return nil
}

func (v *ArchiveVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.fsys == nil {
		return vfs.VFSItem{}, fmt.Errorf("archive VFS is closed")
	}
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}

	normPath := filepath.ToSlash(path)
	normArcPath := filepath.ToSlash(v.arcPath)
	fsPath := "."
	if normPath != normArcPath {
		fsPath = strings.TrimPrefix(normPath, normArcPath)
		fsPath = strings.TrimPrefix(fsPath, "/")
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

func (w *archiveReadWrapper) extractToTemp(ctx context.Context) {
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
		w.mu.Lock()
		w.err = fmt.Errorf("no source file available for extraction")
		w.mu.Unlock()
		return
	}

	tmp, err := os.CreateTemp("", "f4arc-*")
	if err != nil {
		if srcCloser != nil {
			srcCloser.Close()
		}
		w.mu.Lock()
		w.err = err
		w.mu.Unlock()
		return
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

	if w.f != nil {
		w.f.Close()
		w.f = nil
	}

	if loopErr != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		w.err = loopErr
	} else {
		w.tmpPath = tmp.Name()
		w.tmpFile = tmp
		w.extracted = true
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

		w.extractToTemp(ctx)

		w.mu.Lock()
		w.extracting = false
		close(w.doneChan)
		w.doneChan = nil
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

	for {
		if ctx.Err() != nil {
			return ctx.Err()
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
				if update != nil {
					update(fmt.Sprintf("Extracting %s...", name), pct)
				}
				if reporter != nil {
					elapsed := time.Since(startTime)
					elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
					reporter.UpdateTransfer("Extracting", name, pct, fmt.Sprintf("Extracting: %s / %s", formatSize(currentCopied), formatSize(size)), pct, elapsedStr)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}

func (v *ArchiveVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	v.mu.Lock()
	if v.fsys == nil {
		v.mu.Unlock()
		return nil, fmt.Errorf("archive VFS is closed")
	}
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}

	// Capture fsys and increment active count EARLY to protect it while unlocked.
	v.activeCount++
	fsys := v.fsys
	v.mu.Unlock()

	normPath := filepath.ToSlash(path)
	normArcPath := filepath.ToSlash(v.arcPath)
	fsPath := "."
	if normPath != normArcPath {
		fsPath = strings.TrimPrefix(normPath, normArcPath)
		fsPath = strings.TrimPrefix(fsPath, "/")
	}

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
					reporter.UpdateTransfer("Opening", filepath.Base(path), -1, msg, -1, elapsedStr)
				}
			}
		}
	}()

	srcFile, err := fsys.Open(fsPath)
	close(openDone)

	if err != nil {
		v.decrementActive()
		return nil, err
	}

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

func (v *ArchiveVFS) ParentVFS() vfs.VFS      { return v.parent }
func (v *ArchiveVFS) Join(e ...string) string { return filepath.ToSlash(filepath.Join(e...)) }
func (v *ArchiveVFS) Abs(p string) (string, error) {
	if v.IsAbs(p) {
		return filepath.ToSlash(filepath.Clean(p)), nil
	}
	return v.Join(v.GetPath(), p), nil
}
func (v *ArchiveVFS) Base(p string) string { return filepath.Base(p) }
func (v *ArchiveVFS) Dir(p string) string {
	if p == v.arcPath {
		return v.parent.Dir(v.arcPath)
	}
	return filepath.ToSlash(filepath.Dir(p))
}

func (v *ArchiveVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	v.mu.Lock()
	if v.fsys == nil {
		v.mu.Unlock()
		return nil, fmt.Errorf("archive VFS is closed")
	}
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}

	normPath := filepath.ToSlash(path)
	normArcPath := filepath.ToSlash(v.arcPath)
	fsPath := strings.TrimPrefix(normPath, normArcPath)
	fsPath = strings.TrimPrefix(fsPath, "/")

	tmp, _ := os.CreateTemp("", "f4arc-write-*")
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
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.fsys == nil {
		return fmt.Errorf("archive VFS is closed")
	}
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}

	normPath := filepath.ToSlash(path)
	normArcPath := filepath.ToSlash(v.arcPath)
	fsPath := strings.TrimPrefix(normPath, normArcPath)
	fsPath = strings.TrimPrefix(fsPath, "/")

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
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.fsys == nil {
		return fmt.Errorf("archive VFS is closed")
	}
	if v.cleanupTimer != nil {
		v.cleanupTimer.Stop()
		v.cleanupTimer = nil
	}

	normPath := filepath.ToSlash(path)
	normArcPath := filepath.ToSlash(v.arcPath)
	fsPath := strings.TrimPrefix(normPath, normArcPath)
	fsPath = strings.TrimPrefix(fsPath, "/")

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
	v.mu.Lock()
	if v.fsys == nil {
		v.mu.Unlock()
		return fmt.Errorf("archive VFS is closed")
	}
	v.mu.Unlock()

	// Create a map of selected paths for fast O(1) lookup
	selectedMap := make(map[string]bool)
	for _, p := range srcPaths {
		fullInner := p
		if v.innerPath != "." && v.innerPath != "" {
			fullInner = path.Join(v.innerPath, p)
		}
		fullInner = strings.TrimPrefix(fullInner, "/")
		selectedMap[fullInner] = true
	}

	absPath := v.activePath()
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

	format := archive.DetectFormat(absPath)
	if format == "zip" {
		return v.copyBulkZip(ctx, archiveFile, selectedMap, dstVfs, dstDir, reporter)
	} else if format == "tar" {
		return v.copyBulkTar(ctx, archiveFile, selectedMap, dstVfs, dstDir, reporter)
	}
	return v.copyBulkFallback(ctx, archiveFile, selectedMap, dstVfs, dstDir, reporter)
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

func (v *ArchiveVFS) copyBulkZip(ctx context.Context, f vfs.ReadAtCloser, selected map[string]bool, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
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

		matched := false
		for selPath := range selected {
			if file.Name == selPath || strings.HasPrefix(file.Name, selPath+"/") {
				matched = true
				break
			}
		}

		if !matched {
			mu.Lock()
			lastAction = "Locating"
			lastFile = file.Name
			lastPct = -1
			mu.Unlock()
			if TestSkipDelay > 0 {
				time.Sleep(TestSkipDelay)
			}
			continue
		}

		relPath := file.Name
		if v.innerPath != "." && v.innerPath != "" {
			relPath = strings.TrimPrefix(relPath, v.innerPath)
			relPath = strings.TrimPrefix(relPath, "/")
		}
		targetPath := dstVfs.Join(dstDir, relPath)

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

func (v *ArchiveVFS) copyBulkTar(ctx context.Context, f vfs.ReadAtCloser, selected map[string]bool, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
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

		cleanName := strings.TrimPrefix(hdr.Name, "/")
		matched := false
		for selPath := range selected {
			if cleanName == selPath || strings.HasPrefix(cleanName, selPath+"/") {
				matched = true
				break
			}
		}

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

		relPath := cleanName
		if v.innerPath != "." && v.innerPath != "" {
			relPath = strings.TrimPrefix(relPath, v.innerPath)
			relPath = strings.TrimPrefix(relPath, "/")
		}
		targetPath := dstVfs.Join(dstDir, relPath)

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

func (v *ArchiveVFS) copyBulkFallback(ctx context.Context, f vfs.ReadAtCloser, selected map[string]bool, dstVfs vfs.VFS, dstDir string, reporter vfs.TaskReporter) error {
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

		cleanName := filepath.ToSlash(filepath.Clean(info.NameInArchive))
		cleanName = strings.TrimPrefix(cleanName, "/")
		cleanName = strings.TrimPrefix(cleanName, "./")
		if cleanName == "." || cleanName == "" {
			return nil
		}

		matched := false
		for selPath := range selected {
			if cleanName == selPath || strings.HasPrefix(cleanName, selPath+"/") {
				matched = true
				break
			}
		}

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

		relPath := cleanName
		if v.innerPath != "." && v.innerPath != "" {
			relPath = strings.TrimPrefix(relPath, v.innerPath)
			relPath = strings.TrimPrefix(relPath, "/")
		}
		targetPath := dstVfs.Join(dstDir, relPath)

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
