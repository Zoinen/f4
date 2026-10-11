//go:build windows

package wincondrag

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/unxed/vtui"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")

	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procSendMessageW               = user32.NewProc("SendMessageW")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procSetCapture                 = user32.NewProc("SetCapture")
	procReleaseCapture             = user32.NewProc("ReleaseCapture")
	procSetTimer                   = user32.NewProc("SetTimer")
	procKillTimer                  = user32.NewProc("KillTimer")
	procGetCursorPos               = user32.NewProc("GetCursorPos")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procGetWindowRect              = user32.NewProc("GetWindowRect")
	procGetWindowLongW             = user32.NewProc("GetWindowLongW")
	procGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	procMouseEvent                 = user32.NewProc("mouse_event")

	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")

	procOleInitialize = ole32.NewProc("OleInitialize")
	procCoTaskMemFree = ole32.NewProc("CoTaskMemFree")

	procSHParseDisplayName                = shell32.NewProc("SHParseDisplayName")
	procSHCreateShellItemArrayFromIDLists = shell32.NewProc("SHCreateShellItemArrayFromIDLists")
	procSHDoDragDrop                      = shell32.NewProc("SHDoDragDrop")

	procWineGetVersion = ntdll.NewProc("wine_get_version")
)

const (
	wmTimer       = 0x0113
	wmLButtonDown = 0x0201
	wmRButtonDown = 0x0204
	wmUser        = 0x0400
	wmPrepare     = wmUser + 0x101
	wmStart       = wmUser + 0x102
	wmAbort       = wmUser + 0x103

	wsPopup        = 0x80000000
	wsExLayered    = 0x00080000
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000
	wsExTopmost    = 0x00000008
	gwlExStyle     = ^uintptr(19) // -20
	lwaAlpha       = 0x00000002

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoActivate   = 0x0010
	swpShowWindow   = 0x0040
	swHide          = 0
	hwndTop         = 0
	hwndTopmost     = ^uintptr(0) // -1
	hwndNoTopmost   = ^uintptr(1) // -2
	smSwapButton    = 23
	armTimerID      = 1
	armTimeoutMs    = 1000
	dragDropSDrop   = 0x00040100
	dragDropSCancel = 0x00040101

	// toolReadyTimeout bounds the wait for the tool window. Creating a
	// window cannot hang on anyone else's thread here -- it has no parent
	// and no owner -- but a drag is not worth a wedged UI either way.
	toolReadyTimeout = 5 * time.Second
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	bhidDataObject = guid{0xB8C0BD9F, 0xED24, 0x455C, [8]byte{0x83, 0xE6, 0xD5, 0x39, 0x0C, 0x4F, 0xE8, 0xC4}}
	iidIDataObject = guid{0x0000010E, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type point struct{ X, Y int32 }

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

var errArmTimeout = errors.New("the synthetic press never reached the tool window")

// request is one drag, from the backend's StartDrag to the tool thread and
// back. Only the tool thread touches data; done is written once.
type request struct {
	paths   []string
	effects uint32
	host    Host
	buttons Buttons
	data    uintptr // IDataObject*, owned by the tool thread
	active  bool
	done    chan result
}

type result struct {
	action vtui.DropAction
	err    error
}

type tool struct {
	once  sync.Once
	hwnd  uintptr
	err   error
	mu    sync.Mutex
	req   *request
	busy  atomic.Bool
	wproc uintptr
}

var theTool tool

// Install registers the console drag source with vtui. It is called once, in
// the Windows console session; a GUI backend registers its own and never
// reaches here. Wine is left alone: its console is not a Windows desktop,
// and Wine has no bridge from OLE out to X11 drop targets anyway.
func Install() {
	if procWineGetVersion.Find() == nil {
		vtui.DebugLog("CONSOLE_DND: running under Wine, console drag out stays off")
		return
	}
	vtui.SetDragBackend(consoleBackend{})
}

type consoleBackend struct{}

func (consoleBackend) AcceptsDrops() bool { return false }

func (consoleBackend) CanStartDrag() bool { return true }

func (consoleBackend) StartDrag(payload vtui.DragPayload, allowed vtui.DropAction) (vtui.DropAction, error) {
	return theTool.drag(payload.Paths, allowed)
}

func (t *tool) drag(paths []string, allowed vtui.DropAction) (vtui.DropAction, error) {
	vtui.DebugLog("CONSOLE_DND: drag of %d path(s) asked for, allowed=%s", len(paths), allowed)
	if len(paths) == 0 {
		return vtui.DropNone, vtui.ErrDragNoData
	}
	if !t.busy.CompareAndSwap(false, true) {
		vtui.DebugLog("CONSOLE_DND: refused, a drag is already in flight")
		return vtui.DropNone, vtui.ErrDragBusy
	}
	defer t.busy.Store(false)

	hwnd, err := t.start()
	if err != nil {
		vtui.DebugLog("CONSOLE_DND: no tool window: %v", err)
		return vtui.DropNone, err
	}

	swapped, _, _ := procGetSystemMetrics.Call(smSwapButton)
	req := &request{
		paths:   paths,
		effects: ActionToEffects(allowed),
		buttons: PrimaryButtons(swapped != 0),
		done:    make(chan result, 1),
	}
	if !buttonDown(req.buttons) {
		vtui.DebugLog("CONSOLE_DND: the button is already up, nothing to drag (swapped=%v)", swapped != 0)
		return vtui.DropNone, nil
	}

	t.mu.Lock()
	t.req = req
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		if t.req == req {
			t.req = nil
		}
		t.mu.Unlock()
	}()

	if r, _, _ := procSendMessageW.Call(hwnd, wmPrepare, 0, 0); r == 0 {
		return vtui.DropNone, fmt.Errorf("no shell data object for %d path(s)", len(paths))
	}

	host, ok := findHost()
	if !ok {
		procSendMessageW.Call(hwnd, wmAbort, 0, 0)
		return vtui.DropNone, errors.New("no visible window under the pointer to lay the tool window over")
	}
	if !buttonDown(req.buttons) {
		vtui.DebugLog("CONSOLE_DND: the button went up while the data object was built")
		procSendMessageW.Call(hwnd, wmAbort, 0, 0)
		return vtui.DropNone, nil
	}
	t.mu.Lock()
	req.host = host
	t.mu.Unlock()

	// The press over the tool window must follow this release in the one
	// serial input stream, or it is a second press of a held button.
	procMouseEvent.Call(req.buttons.Up, 0, 0, 0, 0)
	vtui.DebugLog("CONSOLE_DND: physical button released synthetically")

	if r, _, _ := procSendMessageW.Call(hwnd, wmStart, 0, 0); r == 0 {
		procSendMessageW.Call(hwnd, wmAbort, 0, 0)
		return vtui.DropNone, errors.New("the tool window could not be shown over the host")
	}
	res := <-req.done
	vtui.DebugLog("CONSOLE_DND: drag finished as %s, err=%v", res.action, res.err)
	return res.action, res.err
}

// start brings up the tool thread and its window once per process.
func (t *tool) start() (uintptr, error) {
	t.once.Do(func() {
		ready := make(chan struct{})
		go t.threadMain(ready)
		select {
		case <-ready:
		case <-time.After(toolReadyTimeout):
			t.mu.Lock()
			t.err = fmt.Errorf("tool thread silent for %s", toolReadyTimeout)
			t.mu.Unlock()
		}
	})
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return 0, t.err
	}
	return t.hwnd, nil
}

func (t *tool) threadMain(ready chan struct{}) {
	runtime.LockOSThread()
	// The thread stays locked for the life of the process: it owns an STA
	// and a window, and handing it back to the scheduler would hand both to
	// whichever goroutine ran next.

	hr, _, _ := procOleInitialize.Call(0)
	vtui.DebugLog("CONSOLE_DND: OleInitialize on the tool thread hr=0x%08X", uint32(hr))

	inst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("F4ConsoleDragTool")
	t.wproc = syscall.NewCallback(t.wndProc)
	wc := wndClassExW{WndProc: t.wproc, Instance: inst, ClassName: className}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		vtui.DebugLog("CONSOLE_DND: RegisterClassExW failed: %v", err)
	}

	// No parent and no owner: see the package comment.
	hwnd, _, err := procCreateWindowExW.Call(
		wsExLayered|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), 0, wsPopup,
		0, 0, 1, 1, 0, 0, inst, 0)
	if hwnd == 0 {
		failure := fmt.Errorf("CreateWindowExW failed: %v", err)
		vtui.DebugLog("CONSOLE_DND: %v", failure)
		t.mu.Lock()
		t.err = failure
		t.mu.Unlock()
		close(ready)
		return
	}
	t.mu.Lock()
	t.hwnd = hwnd
	t.mu.Unlock()
	// Alpha 1, not 0: a fully transparent layered window is not hit-tested,
	// and the whole point of this window is to be hit by the press.
	procSetLayeredWindowAttributes.Call(hwnd, 0, 1, lwaAlpha)
	vtui.DebugLog("CONSOLE_DND: tool window 0x%X ready", hwnd)
	close(ready)

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *tool) current() *request {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.req
}

func (t *tool) wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch message {
	case wmPrepare:
		req := t.current()
		if req == nil {
			return 0
		}
		releaseData(req)
		data, err := shellDataObject(req.paths)
		if err != nil {
			vtui.DebugLog("CONSOLE_DND: %v", err)
			return 0
		}
		req.data = data
		vtui.DebugLog("CONSOLE_DND: shell data object ready for %d path(s)", len(req.paths))
		return 1
	case wmStart:
		return t.showAndArm(hwnd)
	case wmAbort:
		if req := t.current(); req != nil {
			releaseData(req)
		}
		return 0
	case wmLButtonDown, wmRButtonDown:
		procKillTimer.Call(hwnd, armTimerID)
		vtui.DebugLog("CONSOLE_DND: the synthetic press reached the tool window (msg=0x%X)", message)
		t.runDrag(hwnd)
		return 0
	case wmTimer:
		if wparam == armTimerID {
			t.disarm(hwnd)
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
	return r
}

func (t *tool) showAndArm(hwnd uintptr) uintptr {
	req := t.current()
	if req == nil || req.data == 0 {
		return 0
	}
	h := req.host
	// Demoting first is harmless when an earlier topmost request was
	// refused, and keeps the cover from depending on foreground rights.
	if !h.Topmost {
		procSetWindowPos.Call(hwnd, hwndNoTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
	after := uintptr(hwndTop)
	if h.Topmost {
		after = hwndTopmost
	}
	procSetWindowPos.Call(hwnd, after, uintptr(h.Rect.Left), uintptr(h.Rect.Top),
		uintptr(h.Rect.Right-h.Rect.Left), uintptr(h.Rect.Bottom-h.Rect.Top), swpNoActivate|swpShowWindow)
	if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
		vtui.DebugLog("CONSOLE_DND: the tool window did not become visible")
		releaseData(req)
		return 0
	}
	procSetCapture.Call(hwnd)
	procSetTimer.Call(hwnd, armTimerID, armTimeoutMs, 0)
	procMouseEvent.Call(mouseEventMove|req.buttons.Down, 0, 0, 0, 0)
	vtui.DebugLog("CONSOLE_DND: tool window over host 0x%X at %d,%d-%d,%d (topmost=%v), synthetic press sent",
		h.Handle, h.Rect.Left, h.Rect.Top, h.Rect.Right, h.Rect.Bottom, h.Topmost)
	return 1
}

func (t *tool) runDrag(hwnd uintptr) {
	req := t.current()
	if req == nil || req.data == 0 || req.active {
		vtui.DebugLog("CONSOLE_DND: a press with no drag armed, ignored")
		return
	}
	req.active = true
	var effect uint32
	vtui.DebugLog("CONSOLE_DND: calling SHDoDragDrop, effects=0x%X", req.effects)
	// A nil drop source: the shell supplies its own, which drops on the
	// release of the button that started the drag and cancels on Esc.
	hr, _, _ := procSHDoDragDrop.Call(hwnd, req.data, 0, uintptr(req.effects), uintptr(unsafe.Pointer(&effect)))
	vtui.DebugLog("CONSOLE_DND: SHDoDragDrop returned hr=0x%08X effect=0x%X", uint32(hr), effect)
	procReleaseCapture.Call()
	procShowWindow.Call(hwnd, swHide)
	releaseData(req)
	switch uint32(hr) {
	case dragDropSDrop:
		req.done <- result{action: EffectToAction(effect)}
	case dragDropSCancel:
		req.done <- result{action: vtui.DropNone}
	default:
		req.done <- result{err: fmt.Errorf("SHDoDragDrop failed with HRESULT 0x%08X", uint32(hr))}
	}
}

func (t *tool) disarm(hwnd uintptr) {
	procKillTimer.Call(hwnd, armTimerID)
	req := t.current()
	if req != nil && req.active {
		return
	}
	procReleaseCapture.Call()
	procShowWindow.Call(hwnd, swHide)
	if req == nil {
		return
	}
	vtui.DebugLog("CONSOLE_DND: disarmed after %d ms: %v", armTimeoutMs, errArmTimeout)
	releaseData(req)
	req.done <- result{err: errArmTimeout}
}

// findHost is the window under the pointer the tool window will cover: the
// console window, or under a pseudoconsole the terminal in the foreground.
// The console window is only read, never sent anything.
func findHost() (Host, bool) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	console, _, _ := procGetConsoleWindow.Call()
	foreground, _, _ := procGetForegroundWindow.Call()
	c, f := describe(console), describe(foreground)
	host, ok := PickHost(pt.X, pt.Y, c, f)
	vtui.DebugLog("CONSOLE_DND: pointer %d,%d; console 0x%X %v visible=%v; foreground 0x%X %v visible=%v; picked 0x%X ok=%v",
		pt.X, pt.Y, c.Handle, c.Rect, c.Visible, f.Handle, f.Rect, f.Visible, host.Handle, ok)
	return host, ok
}

func describe(hwnd uintptr) Host {
	if hwnd == 0 {
		return Host{}
	}
	h := Host{Handle: hwnd}
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&h.Rect)))
	v, _, _ := procIsWindowVisible.Call(hwnd)
	h.Visible = v != 0
	ex, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	h.Topmost = ex&wsExTopmost != 0
	return h
}

func buttonDown(b Buttons) bool {
	s, _, _ := procGetAsyncKeyState.Call(b.VirtualKey)
	return s&0x8000 != 0
}

// shellDataObject is the data object Explorer itself would hand out for
// these files: every format a target may ask for, from CF_HDROP to the
// shell's own, with no COM object written in Go. It must be built on the
// thread that will run the drag, since the object belongs to its apartment.
func shellDataObject(paths []string) (uintptr, error) {
	pidls := make([]uintptr, 0, len(paths))
	defer func() {
		for _, p := range pidls {
			procCoTaskMemFree.Call(p)
		}
	}()
	for _, p := range paths {
		name, err := syscall.UTF16PtrFromString(p)
		if err != nil {
			vtui.DebugLog("CONSOLE_DND: skipped %q: %v", p, err)
			continue
		}
		var pidl uintptr
		hr, _, _ := procSHParseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
		if int32(hr) < 0 || pidl == 0 {
			vtui.DebugLog("CONSOLE_DND: SHParseDisplayName(%q) hr=0x%08X", p, uint32(hr))
			continue
		}
		pidls = append(pidls, pidl)
	}
	if len(pidls) == 0 {
		return 0, errors.New("none of the paths is known to the shell")
	}
	var array uintptr
	hr, _, _ := procSHCreateShellItemArrayFromIDLists.Call(uintptr(len(pidls)), uintptr(unsafe.Pointer(&pidls[0])), uintptr(unsafe.Pointer(&array)))
	if int32(hr) < 0 || array == 0 {
		return 0, fmt.Errorf("SHCreateShellItemArrayFromIDLists hr=0x%08X", uint32(hr))
	}
	defer comRelease(array)
	// IShellItemArray::BindToHandler is the fourth slot, after IUnknown's three.
	var data uintptr
	hr = comCall(array, 3, 0, uintptr(unsafe.Pointer(&bhidDataObject)), uintptr(unsafe.Pointer(&iidIDataObject)), uintptr(unsafe.Pointer(&data)))
	if int32(hr) < 0 || data == 0 {
		return 0, fmt.Errorf("IShellItemArray::BindToHandler(BHID_DataObject) hr=0x%08X", uint32(hr))
	}
	return data, nil
}

func releaseData(req *request) {
	if req.data != 0 {
		comRelease(req.data)
		req.data = 0
	}
}

// readWord copies one pointer-sized word out of memory Windows owns. COM
// pointers and vtables live there, and RtlMoveMemory reads them without a
// Go pointer ever being made out of an integer.
func readWord(addr uintptr) uintptr {
	var v uintptr
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&v)), addr, unsafe.Sizeof(v))
	return v
}

// comCall calls method slot of a COM object's vtable with this as the first
// argument.
func comCall(this uintptr, slot int, args ...uintptr) uintptr {
	vtbl := readWord(this)
	fn := readWord(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0)))
	all := append([]uintptr{this}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func comRelease(this uintptr) {
	comCall(this, 2)
}
