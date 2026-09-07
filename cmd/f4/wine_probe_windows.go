//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var procGetConsoleScreenBufferInfoProbe = kernel32SimpleExec.NewProc("GetConsoleScreenBufferInfo")

type probeBufferInfo struct {
	Size              overlayCoord
	CursorPosition    overlayCoord
	Attributes        uint16
	Window            simpleSmallRect
	MaximumWindowSize overlayCoord
}

// probeConsole reads the visible console's geometry through STD_OUTPUT_HANDLE,
// the same handle the Ctrl+O overlay paints into.
func probeConsole() consoleProbe {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return consoleProbe{}
	}
	var info probeBufferInfo
	r1, _, _ := procGetConsoleScreenBufferInfoProbe.Call(uintptr(h), uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return consoleProbe{}
	}
	return consoleProbe{
		OK:        true,
		BufW:      int(info.Size.X),
		BufH:      int(info.Size.Y),
		WinLeft:   int(info.Window.Left),
		WinTop:    int(info.Window.Top),
		WinRight:  int(info.Window.Right),
		WinBottom: int(info.Window.Bottom),
		CursorX:   int(info.CursorPosition.X),
		CursorY:   int(info.CursorPosition.Y),
	}
}
