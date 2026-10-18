package fileops

import (
	"context"
	"io"
	"os"
	"path"
	"strings"

	"github.com/unxed/f4/vfs"
)

// IsNullDeviceDest reports whether destInput names the null device, the
// target far2l's copy accepts for "read every file, keep nothing" (a quick
// check that files on a flash drive are readable). The explicit device paths
// mean it everywhere; the bare Windows names "nul" and "nul:" only on
// Windows, where they are the device — elsewhere "nul" is an ordinary file
// name, and copying onto it must stay a copy.
func IsNullDeviceDest(destInput, goos string) bool {
	name := strings.ToLower(strings.TrimRight(strings.TrimSpace(destInput), `/\`))
	switch name {
	case `\\.\nul`, `\\?\nul`, `//./nul`, `//?/nul`, "/dev/null":
		return true
	case "nul", "nul:":
		return goos == "windows"
	}
	return false
}

// discardVFS is the destination of a copy to the null device: it holds a
// root folder, accepts every folder and file below it and keeps nothing, so
// the whole copy machinery (scan, progress, read errors) runs unchanged.
type discardVFS struct{}

var errDiscardReadOnly = os.ErrPermission

func (discardVFS) IsAtRoot() bool          { return true }
func (discardVFS) GetPath() string         { return "/" }
func (discardVFS) IsAbs(p string) bool     { return path.IsAbs(p) }
func (discardVFS) SetPath(string) error    { return nil }
func (discardVFS) Join(e ...string) string { return path.Join(e...) }
func (discardVFS) Base(p string) string    { return path.Base(p) }
func (discardVFS) Dir(p string) string     { return path.Dir(p) }

func (discardVFS) Abs(p string) (string, error) {
	return path.Clean("/" + p), nil
}

func (discardVFS) ReadDir(context.Context, string, func([]vfs.VFSItem)) error { return nil }

// Stat knows only the root: every file below it is "new", so a copy never
// asks about overwriting.
func (discardVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	if path.Clean("/"+p) == "/" {
		return vfs.VFSItem{Name: "/", IsDir: true}, nil
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func (discardVFS) MkDir(context.Context, string) error                      { return nil }
func (discardVFS) Remove(context.Context, string) error                     { return nil }
func (discardVFS) Rename(context.Context, string, string) error             { return errDiscardReadOnly }
func (discardVFS) SetAttributes(context.Context, string, vfs.VFSItem) error { return nil }

func (discardVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasWrite: true}
}

func (discardVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

func (discardVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, os.ErrNotExist
}

func (discardVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return discardWriter{}, nil
}

func (discardVFS) ParentVFS() vfs.VFS { return nil }
func (v discardVFS) Clone() vfs.VFS   { return v }
func (discardVFS) Close() error       { return nil }

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) Close() error                { return nil }
