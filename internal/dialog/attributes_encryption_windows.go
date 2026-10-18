//go:build windows

package dialog

import (
	"fmt"
	"unsafe"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"golang.org/x/sys/windows"
)

var (
	advapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procEncryptFileW = advapi32.NewProc("EncryptFileW")
	procDecryptFileW = advapi32.NewProc("DecryptFileW")
)

func setWindowsFileEncryption(v vfs.VFS, path string, encrypted bool) error {
	if hostmode.Posix() {
		return fmt.Errorf("NTFS encryption is unavailable in POSIX mode")
	}
	osv, ok := v.(*vfs.OSVFS)
	if !ok {
		return fmt.Errorf("NTFS encryption is available only on the local filesystem")
	}
	abs, err := osv.Abs(path)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}
	proc := procDecryptFileW
	if encrypted {
		proc = procEncryptFileW
	}
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(name)))
	if result == 0 {
		return callErr
	}
	return nil
}
