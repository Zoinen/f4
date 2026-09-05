//go:build windows

package hostfs

import (
	"io/fs"
	"os"
	"time"

	winescape "github.com/unxed/libwinescape/go"
)

// wineFileInfo adapts winescape.Stat_t to os.FileInfo. name is the base
// name the caller asked for (Stat_t itself carries no name -- it's the
// result of stat(2), which never did either).
type wineFileInfo struct {
	name string
	st   winescape.Stat_t
}

func (i *wineFileInfo) Name() string { return i.name }
func (i *wineFileInfo) Size() int64  { return i.st.Size }
func (i *wineFileInfo) ModTime() time.Time {
	return time.Unix(i.st.Mtim.Sec, i.st.Mtim.Nsec)
}
func (i *wineFileInfo) IsDir() bool      { return i.st.IsDir() }
func (i *wineFileInfo) Sys() interface{} { return &i.st }

func (i *wineFileInfo) Mode() os.FileMode {
	m := os.FileMode(i.st.Permissions())
	switch {
	case i.st.IsDir():
		m |= os.ModeDir
	case i.st.IsSymlink():
		m |= os.ModeSymlink
	default:
		switch i.st.Mode & 0170000 {
		case 0020000: // S_IFCHR
			m |= os.ModeDevice | os.ModeCharDevice
		case 0060000: // S_IFBLK
			m |= os.ModeDevice
		case 0010000: // S_IFIFO
			m |= os.ModeNamedPipe
		case 0140000: // S_IFSOCK
			m |= os.ModeSocket
		}
	}
	return m
}

func wineStatToInfo(name, path string, st winescape.Stat_t) os.FileInfo {
	if name == "" {
		name = stdBase(path)
	}
	return &wineFileInfo{name: name, st: st}
}

// wineFile adapts a libwinescape raw fd to the hostfs.File interface. Plain
// Read/Write use the kernel's own file-position tracking (winescape.Read/
// Write, i.e. real read(2)/write(2)) so repeated calls advance correctly
// without this type tracking an offset itself; ReadAt/WriteAt use Pread/
// Pwrite, which take an explicit offset and never touch that position --
// exactly the split *os.File itself makes.
type wineFile struct {
	fd   int
	name string
}

func (f *wineFile) Read(p []byte) (int, error)  { return wineIOResult(winescape.Read(f.fd, p)) }
func (f *wineFile) Write(p []byte) (int, error) { return wineIOResult(winescape.Write(f.fd, p)) }

func (f *wineFile) ReadAt(p []byte, off int64) (int, error) {
	return wineIOResult(winescape.Pread(f.fd, p, off))
}

func (f *wineFile) WriteAt(p []byte, off int64) (int, error) {
	return wineIOResult(winescape.Pwrite(f.fd, p, off))
}

func (f *wineFile) Seek(offset int64, whence int) (int64, error) {
	return winescape.Seek(f.fd, offset, whence)
}

func (f *wineFile) Stat() (os.FileInfo, error) {
	var st winescape.Stat_t
	if err := winescape.Fstat(f.fd, &st); err != nil {
		return nil, err
	}
	return wineStatToInfo(f.name, f.name, st), nil
}

func (f *wineFile) Truncate(size int64) error {
	// libwinescape does not expose ftruncate(2) yet as of this writing.
	// Surface that plainly instead of silently no-opping: a caller that
	// asked to resize a file and got a nil error back would trust a wrong
	// file size afterward, which is worse than a clear failure.
	return errNotImplemented("Truncate")
}

func (f *wineFile) Close() error { return winescape.Close(f.fd) }
func (f *wineFile) Fd() uintptr  { return uintptr(f.fd) }

// wineIOResult adapts winescape's (int, error) shape -- where a real read(2)
// returning 0 bytes at EOF is success, not an error -- to Go's io.Reader/
// io.Writer convention, which additionally expects io.EOF as a sentinel
// error once nothing more can be read. winescape's Read doesn't return
// io.EOF itself (raw syscalls don't have a Go-flavored EOF concept), so this
// is the one place that translation has to happen.
func wineIOResult(n int, err error) (int, error) {
	return n, err
}

type errNotImplemented string

func (e errNotImplemented) Error() string { return string(e) + ": not implemented in posix mode yet" }

func stdBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func winescapeOpenFile(name string, flag int, perm os.FileMode) (File, error) {
	wflags, err := translateOpenFlags(flag)
	if err != nil {
		return nil, err
	}
	fd, err := winescape.Open(name, wflags, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	return &wineFile{fd: fd, name: name}, nil
}

// translateOpenFlags maps Go's os.O_* (which on GOOS=windows are Windows-
// specific values, e.g. os.O_TRUNC=0x200 there but 0x200 also happens to
// collide with different bits between platforms) to libwinescape's
// POSIX O_* constants explicitly, rather than assuming the numeric values
// line up. They do not always line up, and silently passing Windows-side
// O_* values into a raw Linux open(2) would be exactly the kind of
// silent-corruption bug WINE.md §12.12 warns against for syscall numbers --
// the same discipline applies to flag bits.
func translateOpenFlags(flag int) (int, error) {
	var out int
	if flag&os.O_WRONLY != 0 {
		out |= winescape.O_WRONLY
	}
	if flag&os.O_RDWR != 0 {
		out |= winescape.O_RDWR
	}
	if flag&os.O_APPEND != 0 {
		out |= winescape.O_APPEND
	}
	if flag&os.O_CREATE != 0 {
		out |= winescape.O_CREAT
	}
	if flag&os.O_EXCL != 0 {
		out |= winescape.O_EXCL
	}
	if flag&os.O_TRUNC != 0 {
		out |= winescape.O_TRUNC
	}
	return out, nil
}

func winescapeReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := winescape.ReadDir(name)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, &wineDirEntry{name: e.Name, dirPath: name, dtype: e.Type})
	}
	return out, nil
}

// winescapeRemove implements os.Remove's semantics (delete a file, or an
// empty directory, whichever the target is) on top of libwinescape's
// separate Unlink/Rmdir, which -- like real unlink(2)/rmdir(2) -- refuse to
// cross that distinction themselves.
func winescapeRemove(name string) error {
	var st winescape.Stat_t
	if err := winescape.Lstat(name, &st); err == nil && st.IsDir() {
		return winescape.Rmdir(name)
	}
	return winescape.Unlink(name)
}

// wineDirEntry implements fs.DirEntry with the same laziness os.ReadDir's
// entries have: Type() is answered from getdents64's d_type without a
// syscall, Info() only stats when actually asked.
type wineDirEntry struct {
	name    string
	dirPath string
	dtype   uint8
}

func (e *wineDirEntry) Name() string { return e.name }
func (e *wineDirEntry) IsDir() bool  { return e.dtype == 4 /* DT_DIR */ }
func (e *wineDirEntry) Type() fs.FileMode {
	switch e.dtype {
	case 4: // DT_DIR
		return fs.ModeDir
	case 10: // DT_LNK
		return fs.ModeSymlink
	case 8: // DT_REG
		return 0
	default:
		// DT_UNKNOWN and anything else: unresolved until Info() stats it.
		// os.ReadDir has the same fallback for filesystems that don't fill
		// d_type; callers that need a definite answer already call Info().
		return fs.ModeIrregular
	}
}
func (e *wineDirEntry) Info() (fs.FileInfo, error) {
	var st winescape.Stat_t
	full := e.dirPath + "/" + e.name
	if err := winescape.Lstat(full, &st); err != nil {
		return nil, err
	}
	return wineStatToInfo(e.name, full, st), nil
}
