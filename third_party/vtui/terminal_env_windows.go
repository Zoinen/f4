//go:build windows

package vtui

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modMsvcrt   = syscall.NewLazyDLL("msvcrt.dll")
	procSetMode = modMsvcrt.NewProc("_setmode")
)

const _O_BINARY = 0x8000

func watchResizeSignal(c chan os.Signal) {
	// Windows doesn't use signals for resizing.
	// FrameManager already polls terminal size on Windows.
}

func initTerminalOS() {
	// Ensure that Windows Console handles UTF-8 output properly.
	// 65001 is the ID for CP_UTF8
	windows.SetConsoleOutputCP(65001)
	windows.SetConsoleCP(65001)

	// Set binary mode for Stdin and Stdout to prevent CRLF translation and improve speed.
	// This is the "secret trick" for high-performance console output in Windows.
	procSetMode.Call(uintptr(0), uintptr(_O_BINARY))
	procSetMode.Call(uintptr(1), uintptr(_O_BINARY))
	procSetMode.Call(uintptr(2), uintptr(_O_BINARY))

	// Enable VT processing for Windows Console (conhost)
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil {
		var mode uint32
		if err := windows.GetConsoleMode(hOut, &mode); err == nil {
			mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_WRAP_AT_EOL_OUTPUT
			windows.SetConsoleMode(hOut, mode)
		}
	}

	// Отключаем режим QuickEdit на Windows, чтобы консоль отдавала нам правый и средний клики
	hIn, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err == nil {
		var mode uint32
		if err := windows.GetConsoleMode(hIn, &mode); err == nil {
			const ENABLE_QUICK_EDIT_MODE = 0x0040
			const ENABLE_EXTENDED_FLAGS = 0x0080
			mode &^= ENABLE_QUICK_EDIT_MODE
			mode |= ENABLE_EXTENDED_FLAGS
			windows.SetConsoleMode(hIn, mode)
		}
	}
}

type consoleCursorInfo struct {
	size    uint32
	visible int32
}

var (
	kernel32DLL              = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleCursorInfo = kernel32DLL.NewProc("GetConsoleCursorInfo")
	procSetConsoleCursorInfo = kernel32DLL.NewProc("SetConsoleCursorInfo")
)

func SetCursorStyleOS(visible bool, shape CursorShape) {
	hOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}

	var info consoleCursorInfo
	r1, _, _ := procGetConsoleCursorInfo.Call(uintptr(hOut), uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return
	}

	if visible {
		info.visible = 1
	} else {
		info.visible = 0
	}

	if shape == CursorShapeBlock {
		info.size = 100
	} else {
		info.size = 30
	}

	procSetConsoleCursorInfo.Call(uintptr(hOut), uintptr(unsafe.Pointer(&info)))
}
