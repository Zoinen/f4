//go:build windows

package app

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Explorer context menu of a file (f4#2041) is the shell's IContextMenu
// of the item, shown with TrackPopupMenuEx from a window of the calling
// thread: the console window belongs to another process and cannot own it.

var (
	explorerShell32 = windows.NewLazySystemDLL("shell32.dll")
	explorerUser32  = windows.NewLazySystemDLL("user32.dll")

	procSHParseDisplayName = explorerShell32.NewProc("SHParseDisplayName")
	procSHBindToParent     = explorerShell32.NewProc("SHBindToParent")

	procCreatePopupMenu     = explorerUser32.NewProc("CreatePopupMenu")
	procDestroyMenu         = explorerUser32.NewProc("DestroyMenu")
	procGetMenuItemCount    = explorerUser32.NewProc("GetMenuItemCount")
	procTrackPopupMenuEx    = explorerUser32.NewProc("TrackPopupMenuEx")
	procGetCursorPos        = explorerUser32.NewProc("GetCursorPos")
	procSetForegroundWindow = explorerUser32.NewProc("SetForegroundWindow")
	procCreateWindowExW     = explorerUser32.NewProc("CreateWindowExW")
	procDestroyWindow       = explorerUser32.NewProc("DestroyWindow")
	procPostMessageW        = explorerUser32.NewProc("PostMessageW")
	procRegisterClassExW    = explorerUser32.NewProc("RegisterClassExW")
	procDefWindowProcW      = explorerUser32.NewProc("DefWindowProcW")
	procGetModuleHandleW    = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

var (
	iidIShellFolder  = windows.GUID{Data1: 0x000214E6, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIContextMenu  = windows.GUID{Data1: 0x000214E4, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIContextMenu2 = windows.GUID{Data1: 0x000214F4, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIContextMenu3 = windows.GUID{Data1: 0xBCFCE0A0, Data2: 0xEC17, Data3: 0x11D0, Data4: [8]byte{0x8D, 0x10, 0x00, 0xA0, 0xC9, 0x0F, 0x27, 0x19}}
)

const (
	explorerCmdFirst = 1
	explorerCmdLast  = 0x7FFF

	cmfNormal      = 0x0
	cmfExplore     = 0x4
	tpmReturnCmd   = 0x0100
	tpmRightBtn    = 0x0002
	wsPopup        = 0x80000000
	swShowNormal   = 1
	wmNull         = 0
	vtblQueryIface = 0
	vtblRelease    = 2
	vtblUIObjOf    = 10 // IShellFolder::GetUIObjectOf
	vtblQueryMenu  = 3  // IContextMenu::QueryContextMenu
	vtblInvoke     = 4  // IContextMenu::InvokeCommand
	vtblMenuMsg    = 6  // IContextMenu2::HandleMenuMsg
	vtblMenuMsg2   = 7  // IContextMenu3::HandleMenuMsg2

	wmDrawItem      = 0x002B
	wmMeasureItem   = 0x002C
	wmMenuChar      = 0x0120
	wmInitMenuPopup = 0x0117
)

type cmInvokeCommandInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       uintptr
	lpParameters uintptr
	lpDirectory  uintptr
	nShow        int32
	dwHotKey     uint32
	hIcon        uintptr
}

// hrFailed reports whether an HRESULT is an error: its top bit is set.
func hrFailed(hr uintptr) bool { return hr&0x80000000 != 0 }

// comCall calls method index of the COM object obj.
func comCall(obj unsafe.Pointer, index int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	method := *(*uintptr)(unsafe.Add(vtbl, uintptr(index)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

// explorerMenu is the shell's context menu of one item, filled into a popup
// menu. close releases both.
type explorerMenu struct {
	menu uintptr
	cm   unsafe.Pointer
	// drawer is the IContextMenu3 of the menu, or else its IContextMenu2,
	// nil when it has neither: the submenus the shell fills while they open
	// ("Send to", "Open with") and owner-drawn items go through it.
	drawer unsafe.Pointer
	v3     bool
}

func (m *explorerMenu) close() {
	if m.menu != 0 {
		_, _, _ = procDestroyMenu.Call(m.menu)
	}
	if m.drawer != nil {
		comCall(m.drawer, vtblRelease)
	}
	if m.cm != nil {
		comCall(m.cm, vtblRelease)
	}
}

// explorerMenuShown is the menu on screen, for the window procedure, which
// runs on the thread that shows it; explorerMenuMu lets one show at a time.
var (
	explorerMenuMu    sync.Mutex
	explorerMenuShown *explorerMenu
)

// explorerMenuWndProc passes the messages of opening submenus and
// owner-drawn items to the shell's menu, as Explorer does.
var explorerMenuWndProc = windows.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	if m := explorerMenuShown; m != nil && m.drawer != nil {
		switch msg {
		case wmInitMenuPopup, wmDrawItem, wmMeasureItem, wmMenuChar:
			var result uintptr
			var hr uintptr
			if m.v3 {
				hr = comCall(m.drawer, vtblMenuMsg2, msg, wparam, lparam, uintptr(unsafe.Pointer(&result)))
			} else {
				hr = comCall(m.drawer, vtblMenuMsg, msg, wparam, lparam)
			}
			if !hrFailed(hr) {
				switch msg {
				case wmMenuChar:
					return result
				case wmDrawItem, wmMeasureItem:
					return 1
				}
				return 0
			}
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
})

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

var (
	explorerMenuClassOnce sync.Once
	explorerMenuClass     *uint16
	explorerMenuClassErr  error
)

// registerExplorerMenuClass registers, once, the window class of the window
// the menu is shown from.
func registerExplorerMenuClass() (*uint16, error) {
	explorerMenuClassOnce.Do(func() {
		name, err := windows.UTF16PtrFromString("f4ExplorerMenu")
		if err != nil {
			explorerMenuClassErr = err
			return
		}
		instance, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassEx{lpfnWndProc: explorerMenuWndProc, hInstance: instance, lpszClassName: name}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
			explorerMenuClassErr = fmt.Errorf("RegisterClassEx: %w", err)
			return
		}
		explorerMenuClass = name
	})
	return explorerMenuClass, explorerMenuClassErr
}

// buildExplorerMenu asks the shell for the context menu of path, owned by
// hwnd. COM must be initialised on the calling thread.
func buildExplorerMenu(path string, hwnd uintptr) (*explorerMenu, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var pidl unsafe.Pointer
	if hr, _, _ := procSHParseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0); hrFailed(hr) {
		return nil, fmt.Errorf("SHParseDisplayName: %w", windows.Errno(hr))
	}
	defer windows.CoTaskMemFree(pidl)

	var folder, child unsafe.Pointer
	if hr, _, _ := procSHBindToParent.Call(uintptr(pidl), uintptr(unsafe.Pointer(&iidIShellFolder)), uintptr(unsafe.Pointer(&folder)), uintptr(unsafe.Pointer(&child))); hrFailed(hr) {
		return nil, fmt.Errorf("SHBindToParent: %w", windows.Errno(hr))
	}
	defer comCall(folder, vtblRelease)

	m := &explorerMenu{}
	if hr := comCall(folder, vtblUIObjOf, hwnd, 1, uintptr(unsafe.Pointer(&child)), uintptr(unsafe.Pointer(&iidIContextMenu)), 0, uintptr(unsafe.Pointer(&m.cm))); hrFailed(hr) {
		return nil, fmt.Errorf("GetUIObjectOf: %w", windows.Errno(hr))
	}
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		m.close()
		return nil, errors.New("CreatePopupMenu failed")
	}
	m.menu = menu
	if hr := comCall(m.cm, vtblQueryMenu, menu, 0, explorerCmdFirst, explorerCmdLast, cmfNormal|cmfExplore); hrFailed(hr) {
		m.close()
		return nil, fmt.Errorf("QueryContextMenu: %w", windows.Errno(hr))
	}
	if hr := comCall(m.cm, vtblQueryIface, uintptr(unsafe.Pointer(&iidIContextMenu3)), uintptr(unsafe.Pointer(&m.drawer))); !hrFailed(hr) && m.drawer != nil {
		m.v3 = true
	} else {
		m.drawer = nil
		if hr := comCall(m.cm, vtblQueryIface, uintptr(unsafe.Pointer(&iidIContextMenu2)), uintptr(unsafe.Pointer(&m.drawer))); hrFailed(hr) {
			m.drawer = nil
		}
	}
	return m, nil
}

// itemCount is the number of entries the shell put into the menu, -1 when
// GetMenuItemCount fails.
func (m *explorerMenu) itemCount() int {
	n, _, _ := procGetMenuItemCount.Call(m.menu)
	if n > 0x7FFFFFFF {
		return -1
	}
	return int(n)
}

// showExplorerMenu shows the Explorer context menu of path at the mouse
// pointer and runs the command chosen. It blocks until the menu is closed,
// on a thread of its own.
func showExplorerMenu(path string) error {
	if !explorerMenuMu.TryLock() {
		return nil // one is on screen already
	}
	defer explorerMenuMu.Unlock()
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, windows.Errno(1)) { // S_FALSE: already initialised
			done <- err
			return
		}
		defer windows.CoUninitialize()

		var pt struct{ x, y int32 }
		_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		class, err := registerExplorerMenuClass()
		if err != nil {
			done <- err
			return
		}
		x, y := uintptr(pt.x), uintptr(pt.y) // #nosec G115 -- screen coordinates go to the calls as LONG, sign kept
		hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), 0, wsPopup, x, y, 0, 0, 0, 0, 0, 0)
		if hwnd == 0 {
			done <- errors.New("CreateWindowEx failed")
			return
		}
		defer func() { _, _, _ = procDestroyWindow.Call(hwnd) }()

		m, err := buildExplorerMenu(path, hwnd)
		if err != nil {
			done <- err
			return
		}
		defer m.close()

		_, _, _ = procSetForegroundWindow.Call(hwnd)
		explorerMenuShown = m
		cmd, _, _ := procTrackPopupMenuEx.Call(m.menu, tpmReturnCmd|tpmRightBtn, x, y, hwnd, 0)
		explorerMenuShown = nil
		_, _, _ = procPostMessageW.Call(hwnd, wmNull, 0, 0)
		if cmd == 0 {
			done <- nil
			return
		}
		info := cmInvokeCommandInfo{hwnd: hwnd, lpVerb: cmd - explorerCmdFirst, nShow: swShowNormal}
		info.cbSize = uint32(unsafe.Sizeof(info))
		if hr := comCall(m.cm, vtblInvoke, uintptr(unsafe.Pointer(&info))); hrFailed(hr) {
			done <- fmt.Errorf("InvokeCommand: %w", windows.Errno(hr))
			return
		}
		done <- nil
	}()
	return <-done
}
