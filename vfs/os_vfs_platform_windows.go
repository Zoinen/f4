//go:build windows

package vfs

import (
	"syscall"

	"github.com/unxed/f4/vfs/hostmode"
)

func applyPlatformAttributes(path string, item VFSItem) error {
	// There are no Win32 attributes behind libwinescape, and path is a POSIX
	// path that SetFileAttributes would misread. WinAttrs is normally zero in
	// the posix personality; this keeps a stray value from reaching Win32.
	if hostmode.Posix() {
		return nil
	}
	if item.WinAttrs != 0 {
		ptr, err := syscall.UTF16PtrFromString(path)
		if err == nil {
			return syscall.SetFileAttributes(ptr, item.WinAttrs)
		}
	}
	return nil
}
