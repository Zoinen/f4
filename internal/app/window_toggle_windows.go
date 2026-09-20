//go:build windows

package app

import (
	"syscall"
	"unsafe"

	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

var (
	windowToggleUser32      = syscall.NewLazyDLL("user32.dll")
	windowToggleIsZoomed    = windowToggleUser32.NewProc("IsZoomed")
	windowToggleSendMessage = windowToggleUser32.NewProc("SendMessageW")
	windowToggleGetClass    = windowToggleUser32.NewProc("GetClassNameW")
)

const (
	windowToggleWMSysCommand = 0x0112
	windowToggleSCMaximize   = 0xF030
	windowToggleSCRestore    = 0xF120
)

// toggleDirectWindowsTerminalWindow handles the default-terminal handoff:
// Windows Terminal starts f4 without WT_SESSION and without making the
// PseudoConsoleWindow returned by GetConsoleWindow owned by the terminal.
// While Alt+F9 is pressed, the actual WT host is nevertheless the foreground
// window. Send the same system command Far uses for a console window. The
// message is sent to another process just as it is for a normal console
// window; Windows marshals this system command across the thread boundary.
func toggleDirectWindowsTerminalWindow() bool {
	activeBackend := vtui.ActiveBackend()
	if activeBackend != "" {
		return false
	}
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return false
	}
	class := windowClass(hwnd)
	if !shouldToggleDirectWindowsTerminalWindow(activeBackend, class) {
		return false
	}

	zoomed, _, _ := windowToggleIsZoomed.Call(uintptr(hwnd))
	command := uintptr(windowToggleSCMaximize)
	name := "SC_MAXIMIZE"
	if zoomed != 0 {
		command = windowToggleSCRestore
		name = "SC_RESTORE"
	}
	windowToggleSendMessage.Call(uintptr(hwnd), windowToggleWMSysCommand, command, 0)
	vtui.DebugLog("CONSOLE: direct Windows Terminal window %#x class %q, sent %s", hwnd, class, name)
	return true
}

func windowClass(hwnd windows.HWND) string {
	var buf [128]uint16
	n, _, _ := windowToggleGetClass.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
