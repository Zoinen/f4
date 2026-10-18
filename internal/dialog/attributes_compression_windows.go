//go:build windows

package dialog

import (
	"fmt"
	"unsafe"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"golang.org/x/sys/windows"
)

// FSCTL_SET_COMPRESSION is the NTFS operation for setting file compression;
// SetFileAttributes cannot change FILE_ATTRIBUTE_COMPRESSED.
const fsctlSetCompression = 0x0009c040

func setWindowsFileCompression(v vfs.VFS, path string, compressed bool) (retErr error) {
	if hostmode.Posix() {
		return fmt.Errorf("NTFS compression is unavailable in POSIX mode")
	}
	osv, ok := v.(*vfs.OSVFS)
	if !ok {
		return fmt.Errorf("NTFS compression is available only on the local filesystem")
	}
	abs, err := osv.Abs(path)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := windows.CloseHandle(h); retErr == nil {
			retErr = closeErr
		}
	}()

	format := uint16(0)
	if compressed {
		format = 1 // COMPRESSION_FORMAT_DEFAULT
	}
	var returned uint32
	return windows.DeviceIoControl(h, fsctlSetCompression, (*byte)(unsafe.Pointer(&format)), 2, nil, 0, &returned, nil)
}
