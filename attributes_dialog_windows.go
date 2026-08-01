//go:build windows

package main

import (
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
	swShow              = 5
)

func showNativePropertiesOS(path string) {
	verbPtr, err := syscall.UTF16PtrFromString("properties")
	if err != nil {
		return
	}
	filePtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}

	info := shellExecuteInfo{
		fMask:  seeMaskInvokeIDList,
		lpVerb: verbPtr,
		lpFile: filePtr,
		nShow:  swShow,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
}
