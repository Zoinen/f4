//go:build windows

package vfs

import (
	"context"
	"os"

	"github.com/unxed/f4/vfs/hostmode"
	"golang.org/x/sys/windows"
)

// OpenPreview keeps cached media readers from blocking rename/delete. Normal
// editor and transfer handles retain their existing sharing contract.
func (v *OSVFS) OpenPreview(ctx context.Context, path string) (ReadAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hostmode.Posix() {
		return v.Open(ctx, path)
	}
	prepared := prepareOSPath(path)
	info, err := os.Stat(prepared)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	name, err := windows.UTF16PtrFromString(prepared)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open preview", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(handle), prepared)
	info, err = file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &osFileWrapper{File: file, size: info.Size(), localPath: prepared}, nil
}
