//go:build windows

package dialog

import (
	"fmt"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"golang.org/x/sys/windows"
)

// FSCTL_SET_SPARSE marks or clears a file's sparse attribute.
const fsctlSetSparse = 0x000900c4

func setWindowsFileSparse(v vfs.VFS, path string, sparse bool) (retErr error) {
	if hostmode.Posix() {
		return fmt.Errorf("NTFS sparse files are unavailable in POSIX mode")
	}
	osv, ok := v.(*vfs.OSVFS)
	if !ok {
		return fmt.Errorf("NTFS sparse files are available only on the local filesystem")
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

	// FILE_SET_SPARSE_BUFFER is one BOOLEAN followed by seven reserved bytes.
	var input [8]byte
	if sparse {
		input[0] = 1
	}
	var returned uint32
	return windows.DeviceIoControl(h, fsctlSetSparse, &input[0], uint32(len(input)), nil, 0, &returned, nil)
}
