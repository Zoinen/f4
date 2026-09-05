//go:build windows && !amd64 && !arm64

package vtui

import "syscall"

// IDropTarget's DragEnter, DragOver and Drop take POINTL by value, and how
// that eight-byte struct is laid out in a call frame is an ABI question
// with a different answer under 32-bit stdcall than under either 64-bit
// Windows ABI. Nothing ships for 32-bit Windows today, so rather than guess
// at a layout that cannot be tested, these architectures keep the
// WS_EX_ACCEPTFILES / WM_DROPFILES path and register no OLE target.

func win32RegisterDropTarget(h *Win32GuiHost, hwnd syscall.Handle) {}

func win32RevokeDropTarget(h *Win32GuiHost, hwnd syscall.Handle) {}
