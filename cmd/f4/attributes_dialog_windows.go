//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           syscall.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       syscall.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      syscall.Handle
	dwHotKey       uint32
	hIconOrMonitor syscall.Handle
	hProcess       syscall.Handle
}

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

const (
	seeMaskInvokeIDList = 0x0000000c
	// SEE_MASK_FLAG_NO_UI tells the Windows shell not to create its own
	// modal error dialog. Errors are returned to F4 instead.
	seeMaskFlagNoUI = 0x00000400
	swShow          = 5
)

func showNativePropertiesOS(path string) error {
	// Avoid handing an already-stale selection to the shell. The file can still
	// disappear after this check, so SEE_MASK_FLAG_NO_UI remains necessary.
	if _, err := os.Stat(path); err != nil {
		return err
	}
	verbPtr, err := syscall.UTF16PtrFromString("properties")
	if err != nil {
		return err
	}
	filePtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	info := shellExecuteInfo{
		fMask:  seeMaskInvokeIDList | seeMaskFlagNoUI,
		lpVerb: verbPtr,
		lpFile: filePtr,
		nShow:  swShow,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	result, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if result != 0 {
		return nil
	}
	if callErr != syscall.Errno(0) {
		return callErr
	}
	return errors.New("ShellExecuteExW failed")
}
