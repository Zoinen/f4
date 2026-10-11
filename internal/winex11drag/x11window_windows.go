//go:build windows

package winex11drag

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winex11User32       = syscall.NewLazyDLL("user32.dll")
	winex11ProcGetPropA = winex11User32.NewProc("GetPropA")
)

// WineX11Window returns the Xlib Window ID of hwnd's top-level window under
// Wine, read from the "__wine_x11_whole_window" property winex11.drv sets on
// it (issue #566, step 1). ok is false when the property is absent or zero,
// which is the normal case on real Windows -- winex11.drv is not loaded
// there at all -- and can also happen for a window winex11.drv has not
// finished creating yet.
//
// This ID names the window the drag started from, for finding which window
// to ask; it must not become the XDND source window for the gesture itself
// -- see the issue's step 4 for why (a SelectionRequest would be delivered
// to Wine's own X connection, which knows nothing about this drag).
func WineX11Window(hwnd windows.HWND) (xid uintptr, ok bool) {
	if hwnd == 0 {
		return 0, false
	}
	namePtr, err := syscall.BytePtrFromString(WineX11PropName)
	if err != nil {
		return 0, false
	}
	raw, _, _ := winex11ProcGetPropA.Call(uintptr(hwnd), uintptr(unsafe.Pointer(namePtr)))
	return wineX11WindowFromProp(raw)
}
