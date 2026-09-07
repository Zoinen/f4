//go:build windows

package main

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	modMsvcrtDLL            = syscall.NewLazyDLL("msvcrt.dll")
	procGetch               = modMsvcrtDLL.NewProc("_getch")
	kernel32SimpleExec      = syscall.NewLazyDLL("kernel32.dll")
	procReadConsoleOutputW  = kernel32SimpleExec.NewProc("ReadConsoleOutputW")
	procWriteConsoleOutputW = kernel32SimpleExec.NewProc("WriteConsoleOutputW")
)

type simpleCharInfo struct {
	UnicodeChar uint16
	Attributes  uint16
}

type simpleSmallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

var (
	savedHostConsoleBuffer []simpleCharInfo
	savedHostConsoleW      int
	savedHostConsoleH      int
	savedHostConsoleTop    int16
	savedHostConsoleMu     sync.Mutex
)

// currentWindowTop reads srWindow.Top off the live console, defaulting to 0
// (the old, always-absolute-row-0 behavior) when the read fails. Snapshotting
// and restoring at the buffer's row 0 only happens to match the visible
// window when there is no scrollback (dwSize == srWindow); the moment
// scrollback exists -- the ordinary case under wineconsole, see WINE.md
// §2g -- row 0 is whatever was printed first in the session, not what the
// user is currently looking at.
func currentWindowTop(h syscall.Handle) int16 {
	var info overlayBufferInfo
	if r1, _, _ := procGetConsoleScreenBufferInfoOverlay.Call(uintptr(h), uintptr(unsafe.Pointer(&info))); r1 != 0 {
		return info.Window.Top
	}
	return 0
}

func modMsvcrtProcImpl() interface {
	Call(...uintptr) (uintptr, uintptr, error)
} {
	if procGetch.Find() == nil {
		return procGetch
	}
	return nil
}

func captureHostConsoleBufferImpl(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	hOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || hOut == 0 || hOut == syscall.InvalidHandle {
		return
	}
	top := currentWindowTop(hOut)

	savedHostConsoleMu.Lock()
	defer savedHostConsoleMu.Unlock()
	size := w * h
	if len(savedHostConsoleBuffer) != size {
		savedHostConsoleBuffer = make([]simpleCharInfo, size)
	}
	savedHostConsoleW = w
	savedHostConsoleH = h
	savedHostConsoleTop = top

	bufSize := uintptr(uint32(uint16(w)) | (uint32(uint16(h)) << 16))
	bufCoord := uintptr(0)
	readRegion := simpleSmallRect{
		Left:   0,
		Top:    top,
		Right:  int16(w - 1),
		Bottom: top + int16(h-1),
	}

	procReadConsoleOutputW.Call(
		uintptr(hOut),
		uintptr(unsafe.Pointer(&savedHostConsoleBuffer[0])),
		bufSize,
		bufCoord,
		uintptr(unsafe.Pointer(&readRegion)),
	)
}

func restoreHostConsoleBufferImpl() {
	savedHostConsoleMu.Lock()
	defer savedHostConsoleMu.Unlock()
	if len(savedHostConsoleBuffer) == 0 || savedHostConsoleW <= 0 || savedHostConsoleH <= 0 {
		return
	}
	hOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || hOut == 0 || hOut == syscall.InvalidHandle {
		return
	}
	w, h := savedHostConsoleW, savedHostConsoleH
	// Write back at the window's *current* Top, not the Top the snapshot was
	// taken at. The two calls are normally back-to-back (see simple_exec.go),
	// but if anything scrolled the console in between, blitting at the old
	// absolute rows would land the saved content off the currently visible
	// window instead of on it. Falls back to row 0 if the live read fails,
	// same as the old unconditional behavior.
	top := currentWindowTop(hOut)
	bufSize := uintptr(uint32(uint16(w)) | (uint32(uint16(h)) << 16))
	bufCoord := uintptr(0)
	writeRegion := simpleSmallRect{
		Left:   0,
		Top:    top,
		Right:  int16(w - 1),
		Bottom: top + int16(h-1),
	}

	procWriteConsoleOutputW.Call(
		uintptr(hOut),
		uintptr(unsafe.Pointer(&savedHostConsoleBuffer[0])),
		bufSize,
		bufCoord,
		uintptr(unsafe.Pointer(&writeRegion)),
	)
}

// hostConsoleBufferMatches reports whether the saved snapshot still describes a
// screen of the given size.
func hostConsoleBufferMatches(w, h int) bool {
	savedHostConsoleMu.Lock()
	defer savedHostConsoleMu.Unlock()
	return len(savedHostConsoleBuffer) > 0 && savedHostConsoleW == w && savedHostConsoleH == h
}
