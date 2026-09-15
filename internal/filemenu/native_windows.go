package filemenu

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/zzl/go-win32api/v2/win32"
)

const Platform = "windows"

var selectShellCommand = func(menu *win32.IContextMenu, popup win32.HMENU, point win32.POINT, hwnd win32.HWND) uint32 {
	choice, _ := win32.TrackPopupMenuEx(popup, uint32(win32.TPM_RETURNCMD|win32.TPM_RIGHTBUTTON|win32.TPM_NOANIMATION), point.X, point.Y, hwnd, nil)
	return uint32(choice)
}

type shellMenu struct {
	paths   []string
	files   []os.FileInfo
	menu    *win32.IContextMenu
	menu2   *win32.IContextMenu2
	menu3   *win32.IContextMenu3
	popup   win32.HMENU
	cleanup []func()
}

func (m *shellMenu) close() {
	for i := len(m.cleanup) - 1; i >= 0; i-- {
		m.cleanup[i]()
	}
	m.cleanup = nil
}

func (m *shellMenu) matches(paths []string) bool {
	if m == nil || !slices.Equal(m.paths, paths) {
		return false
	}
	for i, path := range paths {
		f, err := os.Lstat(path)
		if err != nil || !os.SameFile(f, m.files[i]) || f.ModTime() != m.files[i].ModTime() || f.Size() != m.files[i].Size() || f.Mode() != m.files[i].Mode() {
			return false
		}
	}
	return true
}

type shellSession struct {
	hwnd       win32.HWND
	className  *uint16
	instance   win32.HINSTANCE
	references int32
	pinned     runtime.Pinner
	threadRef  *win32.IUnknown
	prepared   *shellMenu
	theme      *menuTheme
}

func newShellSession() (*shellSession, error) {
	theme, err := newMenuTheme()
	if err != nil {
		return nil, err
	}
	if hr := win32.OleInitialize(nil); hr < 0 {
		theme.close()
		return nil, shellError("Initialize OLE", hr)
	}
	s := &shellSession{theme: theme}
	// Windows retains this address across syscalls and Go stack relocations.
	s.pinned.Pin(&s.references)
	if hr := win32.SHCreateThreadRef(&s.references, &s.threadRef); hr < 0 {
		s.pinned.Unpin()
		win32.OleUninitialize()
		theme.close()
		return nil, shellError("Create thread reference", hr)
	}
	if hr := win32.SHSetThreadRef(s.threadRef); hr < 0 {
		s.threadRef.Release()
		s.pinned.Unpin()
		win32.OleUninitialize()
		theme.close()
		return nil, shellError("Set thread reference", hr)
	}
	s.className, _ = syscall.UTF16PtrFromString("F4FileMenuOwner")
	s.instance, _ = win32.GetModuleHandleW(nil)
	proc := syscall.NewCallback(func(hwnd win32.HWND, msg uint32, wp win32.WPARAM, lp win32.LPARAM) win32.LRESULT {
		switch msg {
		case win32.WM_INITMENUPOPUP, win32.WM_DRAWITEM, win32.WM_MEASUREITEM, win32.WM_MENUCHAR:
			if m := s.prepared; m != nil {
				if m.menu3 != nil {
					var result win32.LRESULT
					if m.menu3.HandleMenuMsg2(msg, wp, lp, &result) == 0 {
						return result
					}
				} else if m.menu2 != nil && m.menu2.HandleMenuMsg(msg, wp, lp) == 0 {
					return 0
				}
			}
		}
		return win32.DefWindowProcW(hwnd, msg, wp, lp)
	})
	wc := win32.WNDCLASSW{LpfnWndProc: proc, HInstance: s.instance, LpszClassName: s.className}
	if atom, err := win32.RegisterClassW(&wc); atom == 0 {
		s.close()
		return nil, fmt.Errorf("Register menu owner: %v", err)
	}
	hwnd, err := win32.CreateWindowExW(win32.WS_EX_TOOLWINDOW, s.className, s.className, win32.WS_POPUP, 0, 0, 1, 1, 0, 0, s.instance, nil)
	if hwnd == 0 {
		s.close()
		return nil, fmt.Errorf("Create menu owner: %v", err)
	}
	s.hwnd = hwnd
	s.theme.update(hwnd)
	return s, nil
}

func pumpShellMessages() {
	var msg win32.MSG
	for win32.PeekMessageW(&msg, 0, 0, 0, win32.PM_REMOVE) != 0 {
		win32.TranslateMessage(&msg)
		win32.DispatchMessageW(&msg)
	}
}

func (s *shellSession) discard() {
	if m := s.prepared; m != nil {
		s.prepared = nil
		m.close()
	}
}

func (s *shellSession) close() {
	s.discard()
	win32.SHSetThreadRef(nil)
	s.threadRef.Release()
	for atomic.LoadInt32(&s.references) > 0 {
		pumpShellMessages()
		win32.MsgWaitForMultipleObjects(0, nil, 0, 50, win32.QS_ALLINPUT)
	}
	if s.hwnd != 0 {
		win32.DestroyWindow(s.hwnd)
	}
	if s.className != nil {
		win32.UnregisterClassW(s.className, s.instance)
	}
	s.pinned.Unpin()
	win32.OleUninitialize()
	s.theme.close()
}

func shellError(stage string, hr win32.HRESULT) error {
	return fmt.Errorf("%s: HRESULT 0x%08x", stage, uint32(hr))
}

func (s *shellSession) prepare(paths []string) (err error) {
	if s.prepared.matches(paths) {
		return nil
	}
	s.discard()
	m := &shellMenu{paths: slices.Clone(paths)}
	defer func() {
		if err != nil {
			m.close()
		}
	}()
	var pidls []*win32.ITEMIDLIST
	for _, path := range paths {
		f, e := os.Lstat(path)
		if e != nil {
			return e
		}
		m.files = append(m.files, f)
		name, e := syscall.UTF16PtrFromString(path)
		if e != nil {
			return e
		}
		var pidl *win32.ITEMIDLIST
		if hr := win32.SHParseDisplayName(name, nil, &pidl, 0, nil); hr < 0 {
			return shellError("Resolve shell item", hr)
		}
		m.cleanup = append(m.cleanup, func() { win32.CoTaskMemFree(unsafe.Pointer(pidl)) })
		pidls = append(pidls, pidl)
	}
	var items *win32.IShellItemArray
	if hr := win32.SHCreateShellItemArrayFromIDLists(uint32(len(pidls)), &pidls[0], &items); hr < 0 {
		return shellError("Create selection", hr)
	}
	m.cleanup = append(m.cleanup, func() { items.Release() })
	if hr := items.BindToHandler(nil, &win32.BHID_SFUIObject, &win32.IID_IContextMenu, unsafe.Pointer(&m.menu)); hr < 0 {
		return shellError("Get shell menu", hr)
	}
	m.cleanup = append(m.cleanup, func() { m.menu.Release() })
	if m.menu.QueryInterface(&win32.IID_IContextMenu3, unsafe.Pointer(&m.menu3)) >= 0 {
		m.cleanup = append(m.cleanup, func() { m.menu3.Release() })
	}
	if m.menu.QueryInterface(&win32.IID_IContextMenu2, unsafe.Pointer(&m.menu2)) >= 0 {
		m.cleanup = append(m.cleanup, func() { m.menu2.Release() })
	}
	m.popup, _ = win32.CreatePopupMenu()
	if m.popup == 0 {
		return fmt.Errorf("Create shell popup failed")
	}
	m.cleanup = append(m.cleanup, func() { win32.DestroyMenu(m.popup) })
	if hr := m.menu.QueryContextMenu(m.popup, 0, 1, 0x7fff, win32.CMF_NORMAL); hr < 0 {
		return shellError("Populate shell menu", hr)
	}
	s.prepared = m
	return nil
}

func (s *shellSession) run(r Request) Result {
	if s.theme.update(s.hwnd) {
		s.discard()
	}
	if r.Operation == "open" {
		for _, path := range r.Paths {
			if err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path).Run(); err != nil {
				return Result{Outcome: Failed, Error: err.Error()}
			}
		}
		return Result{Outcome: Invoked}
	}
	if r.Operation != "" && r.Operation != "prepare" {
		return Result{Outcome: Unavailable}
	}
	if err := s.prepare(r.Paths); err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	if r.Operation == "prepare" {
		return Result{Outcome: Cancelled}
	}
	m := s.prepared
	var point win32.POINT
	win32.GetCursorPos(&point)
	if r.Position.Valid {
		point.X, point.Y = int32(r.Position.X), int32(r.Position.Y)
	}
	previous := win32.GetForegroundWindow()
	win32.SetForegroundWindow(s.hwnd)
	choice := selectShellCommand(m.menu, m.popup, point, s.hwnd)
	win32.PostMessageW(s.hwnd, win32.WM_NULL, 0, 0)
	if choice == 0 {
		if previous != 0 {
			win32.SetForegroundWindow(previous)
		}
		return Result{Outcome: Cancelled}
	}
	defer s.discard()
	if !m.matches(r.Paths) {
		return Result{Outcome: Failed, Error: "The selected files changed while the menu was open"}
	}
	// Integer resources are verb offsets, not string pointers.
	info := win32.CMINVOKECOMMANDINFOEX{FMask: 0x4000 | 0x100 | 0x20000000, Hwnd: s.hwnd, NShow: int32(win32.SW_SHOWNORMAL), PtInvoke: point}
	info.CbSize = uint32(unsafe.Sizeof(info))
	info.LpVerb = (*byte)(unsafe.Pointer(uintptr(choice - 1)))
	info.LpVerbW = (*uint16)(unsafe.Pointer(uintptr(choice - 1)))
	if hr := m.menu.InvokeCommand((*win32.CMINVOKECOMMANDINFO)(unsafe.Pointer(&info))); hr < 0 {
		return Result{Outcome: Failed, Error: shellError("Invoke shell command", hr).Error()}
	}
	win32.OleFlushClipboard()
	return Result{Outcome: Invoked}
}

// One-shot entry point retained for native smoke tests. The production helper
// keeps the session (and its STA thread) alive across requests.
func showNative(r Request) Result {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s, err := newShellSession()
	if err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	defer s.close()
	return s.run(r)
}
