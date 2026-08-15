//go:build windows

package vtui

import (
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procSetClipboardData = user32.NewProc("SetClipboardData")

	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

const (
	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002
)

func setOSClipboard(text string) bool {
	if err := procOpenClipboard.Find(); err != nil {
		return false
	}
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return false
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	u16, err := syscall.UTF16FromString(text)
	if err != nil {
		return false
	}

	size := uintptr(len(u16) * 2)
	hMem, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, size)
	if hMem == 0 {
		return false
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return false
	}

	// Copy memory safely without CGO
	src := unsafe.Pointer(&u16[0])
	var i uintptr
	for i = 0; i < size; i++ {
		*(*byte)(unsafe.Pointer(ptr + i)) = *(*byte)(unsafe.Pointer(uintptr(src) + i))
	}
	procGlobalUnlock.Call(hMem)

	rSet, _, _ := procSetClipboardData.Call(CF_UNICODETEXT, hMem)
	if rSet == 0 {
		procGlobalFree.Call(hMem)
		return false
	}
	return true
}

func getOSClipboard() (string, bool) {
	if err := procOpenClipboard.Find(); err != nil {
		return "", false
	}
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return "", false
	}
	defer procCloseClipboard.Call()

	hMem, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if hMem == 0 {
		return "", false
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(hMem)

	// Read UTF-16 string until null terminator
	var text []uint16
	for i := 0; ; i++ {
		val := *(*uint16)(unsafe.Pointer(ptr + uintptr(i)*2))
		if val == 0 {
			break
		}
		text = append(text, val)
	}

	return syscall.UTF16ToString(text), true
}
