package observer

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"time"
)

// ReaderAt is the read surface this package needs from a probed file,
// restated here in vfs.ReadAtCloser's own shape (ReadAt, Read, Close, Size,
// each context-aware) so that this early scaffolding does not have to import
// package vfs before a real provider needs it -- a *vfs.ReadAtCloser value
// already satisfies this interface with no adapter. Wherever the bytes
// actually live (local disk, a remote VFS, or another archive) is the parent
// VFS's business, not this package's: it only ever reads through this.
type ReaderAt interface {
	ReadAt(ctx context.Context, p []byte, off int64) (int, error)
	Read(ctx context.Context, p []byte) (int, error)
	io.Closer
	Size() int64
}

// SingleFileFS exposes exactly one file to a WASI-mounted guest, so a
// module reads the probed file straight out of the parent VFS's ReaderAt
// instead of a real path on the host disk (that is the whole point: it is
// what lets a module opened on a nested archive member, or a file on a
// remote VFS, read only the bytes it actually needs). Name must be the
// guest-relative name the module's FilePath was built from (see
// runtime.go); wazero calls Open with that same name once the guest asks to
// open it, without a leading slash, per io/fs.FS's own convention.
//
// Close on the returned fs.File is a no-op: SingleFileFS does not own ra's
// lifetime, since callers keep it open for as long as the storage handle
// backed by it stays open, which can outlive any single guest open/close of
// the file.
type SingleFileFS struct {
	ctx     context.Context
	name    string
	ra      ReaderAt
	modTime time.Time
}

// NewSingleFileFS builds a SingleFileFS. ctx governs every read wazero
// performs against ra for as long as the returned FS stays mounted; it
// should be tied to the storage handle's lifetime, not to a single call.
func NewSingleFileFS(ctx context.Context, name string, ra ReaderAt) SingleFileFS {
	return SingleFileFS{ctx: ctx, name: name, ra: ra}
}

// Open implements fs.FS.
//
// It also answers Open(".") with a virtual, empty root directory. wazero
// v1.12.0 lazily opens "." on a mount's own preopen to confirm it really is
// a directory, both when the guest's libc first enumerates its preopens
// (fd_prestat_get, internal/sys/lazy.go's lazyDir.file, called from
// preopenPath in imports/wasi_snapshot_preview1/fs.go) and again on every
// later path_open against it (atPath, same file). A mount with no answer for
// "." fails that check, wasi-libc treats the resulting EBADF as "there is no
// preopen at all", and every absolute path -- including the one file this
// type actually serves -- then fails with ENOENT before path_open is ever
// reached, no matter how the requested name compares to s.name.
func (s SingleFileFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &dirFile{}, nil
	}
	if name != s.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &mountFile{ctx: s.ctx, name: s.name, ra: s.ra, modTime: s.modTime}, nil
}

// dirFile is the virtual root directory Open(".") returns. It carries no
// real entries -- SingleFileFS never has to answer a directory listing, only
// prove to wazero's preopen check that "." is a directory at all.
type dirFile struct{}

func (d *dirFile) Stat() (fs.FileInfo, error) { return dirFileInfo{}, nil }
func (d *dirFile) Read([]byte) (int, error)   { return 0, io.EOF }
func (d *dirFile) Close() error               { return nil }

type dirFileInfo struct{}

func (dirFileInfo) Name() string       { return "." }
func (dirFileInfo) Size() int64        { return 0 }
func (dirFileInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (dirFileInfo) ModTime() time.Time { return time.Time{} }
func (dirFileInfo) IsDir() bool        { return true }
func (dirFileInfo) Sys() any           { return nil }

// mountFile adapts a ReaderAt into an fs.File that also implements
// io.ReaderAt and io.Seeker, which is what lets wazero's WASI implementation
// (internal/sysfs/file.go in v1.12.0) serve fd_pread and fd_seek straight
// from it instead of buffering the whole file first.
type mountFile struct {
	ctx     context.Context
	name    string
	ra      ReaderAt
	off     int64
	modTime time.Time
}

func (f *mountFile) Stat() (fs.FileInfo, error) {
	return mountFileInfo{name: f.name, size: f.ra.Size(), modTime: f.modTime}, nil
}

func (f *mountFile) Read(p []byte) (int, error) {
	n, err := f.ra.ReadAt(f.ctx, p, f.off)
	f.off += int64(n)
	return n, err
}

func (f *mountFile) ReadAt(p []byte, off int64) (int, error) {
	return f.ra.ReadAt(f.ctx, p, off)
}

func (f *mountFile) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = f.off
	case io.SeekEnd:
		base = f.ra.Size()
	default:
		return 0, fmt.Errorf("observer: mountFile.Seek: invalid whence %d", whence)
	}
	next := base + offset
	if next < 0 {
		return 0, fmt.Errorf("observer: mountFile.Seek: negative position")
	}
	f.off = next
	return next, nil
}

// Close is deliberately a no-op; see SingleFileFS's doc comment.
func (f *mountFile) Close() error { return nil }

type mountFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i mountFileInfo) Name() string       { return i.name }
func (i mountFileInfo) Size() int64        { return i.size }
func (i mountFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i mountFileInfo) ModTime() time.Time { return i.modTime }
func (i mountFileInfo) IsDir() bool        { return false }
func (i mountFileInfo) Sys() any           { return nil }
