//go:build windows

package vfs

import (
	"syscall"
	"time"

	"github.com/unxed/f4/vfs/hostmode"
	"golang.org/x/sys/windows"
)

func applyPlatformAttributes(path string, item VFSItem) error {
	// There are no Win32 attributes behind libwinescape, and path is a POSIX
	// path that SetFileAttributes would misread. WinAttrs is normally zero in
	// the posix personality; this keeps a stray value from reaching Win32.
	if hostmode.Posix() {
		return nil
	}
	var errAttrs error
	if item.WinAttrs != 0 {
		ptr, err := syscall.UTF16PtrFromString(path)
		if err == nil {
			errAttrs = syscall.SetFileAttributes(ptr, item.WinAttrs)
		}
	}
	if item.SetBTime && !item.BTime.IsZero() {
		if err := setCreationTime(path, item.BTime); err != nil && errAttrs == nil {
			return err
		}
	}
	return errAttrs
}

// setCreationTime writes the creation time of path (SetFileTime), which
// os.Chtimes cannot reach. FILE_WRITE_ATTRIBUTES is all it asks for, so it
// works on a read-only file and, with backup semantics, on a directory.
func setCreationTime(path string, t time.Time) error {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(ptr, windows.FILE_WRITE_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	ft := windows.NsecToFiletime(t.UnixNano())
	return windows.SetFileTime(h, &ft, nil, nil)
}

// SupportsSetBTime is true on native Windows: the creation time is settable.
// In the POSIX personality (Wine's libwinescape) there is no Win32 behind the
// paths, so it is not.
func (v *OSVFS) SupportsSetBTime() bool { return !hostmode.Posix() }
