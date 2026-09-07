//go:build windows && (amd64 || arm64)

package vtui

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/unxed/vtinput"
)

// An OLE drop target is what makes the window visible to a drag. Without
// one the window has only WS_EX_ACCEPTFILES, which is a one-way door: it
// delivers WM_DROPFILES after a drop but says nothing at all while the
// pointer is moving, so an OLE source finds no target under the cursor and
// draws the "no" sign the whole way. That includes our own DoDragDrop, so
// dragging from one panel of the application to the other -- the one
// direction Wine can support, since Wine has no bridge from OLE out to
// XDND -- could never work before this.
//
// With a target registered we also learn what the source allows, which
// modifiers are held, and where the pointer is, and we get to answer with a
// copy / move / link effect, which is what turns the cursor into something
// other than a refusal.

var (
	procRegisterDragDrop = ole32DLL.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32DLL.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32DLL.NewProc("ReleaseStgMedium")
	procGlobalSize       = kernel32.NewProc("GlobalSize")
)

var iidIDropTarget = guid{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

const (
	// Key state bits OLE reports during a drag. They are the same MK_*
	// values a mouse message carries.
	mkShift   = 0x0004
	mkControl = 0x0008
	mkAlt     = 0x0020

	dvAspectContent = 1

	// IDataObject::GetData is the fourth entry of that interface's vtable,
	// after the three IUnknown methods.
	slotDataObjectGetData = 3
)

type dropTargetVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	DragEnter      uintptr
	DragOver       uintptr
	DragLeave      uintptr
	Drop           uintptr
}

// comDropTarget is one window's IDropTarget. lpVtbl has to stay the first
// field: its address is the interface pointer OLE is given.
type comDropTarget struct {
	lpVtbl   *dropTargetVtbl
	refCount int32

	mu    sync.Mutex
	host  *Win32GuiHost
	paths []string
}

var (
	dropTargetVtblOnce   sync.Once
	globalDropTargetVtbl *dropTargetVtbl
)

func initDropTargetVtbl() {
	dropTargetVtblOnce.Do(func() {
		globalDropTargetVtbl = &dropTargetVtbl{
			QueryInterface: syscall.NewCallback(comDropTargetQueryInterface),
			AddRef:         syscall.NewCallback(comDropTargetAddRef),
			Release:        syscall.NewCallback(comDropTargetRelease),
			DragEnter:      syscall.NewCallback(comDropTargetDragEnter),
			DragOver:       syscall.NewCallback(comDropTargetDragOver),
			DragLeave:      syscall.NewCallback(comDropTargetDragLeave),
			Drop:           syscall.NewCallback(comDropTargetDrop),
		}
	})
}

func newComDropTarget(h *Win32GuiHost) *comDropTarget {
	initDropTargetVtbl()
	t := &comDropTarget{lpVtbl: globalDropTargetVtbl, refCount: 1, host: h}
	comRetain(uintptr(unsafe.Pointer(t)), t)
	return t
}

func (t *comDropTarget) toIUnknown() uintptr { return uintptr(unsafe.Pointer(t)) }

func comDropTargetQueryInterface(this uintptr, riid *guid, ppvObject *uintptr) uintptr {
	if ppvObject == nil {
		return ePointer
	}
	if riid == nil {
		return eNoInterface
	}
	if guidEqual(*riid, iidIUnknown) || guidEqual(*riid, iidIDropTarget) {
		*ppvObject = this
		t := (*comDropTarget)(unsafe.Pointer(this))
		atomic.AddInt32(&t.refCount, 1)
		return sOK
	}
	*ppvObject = 0
	return eNoInterface
}

func comDropTargetAddRef(this uintptr) uintptr {
	t := (*comDropTarget)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&t.refCount, 1))
}

func comDropTargetRelease(this uintptr) uintptr {
	t := (*comDropTarget)(unsafe.Pointer(this))
	count := atomic.AddInt32(&t.refCount, -1)
	if count <= 0 {
		comReleaseFinal(this)
	}
	return uintptr(count)
}

// The POINTL argument of DragEnter, DragOver and Drop is passed by value.
// It is two LONGs, eight bytes, and on both 64-bit Windows ABIs a composite
// of that size travels in a single register, so it arrives here as one
// uintptr with x in the low half and y in the high half. That is what the
// amd64/arm64 constraint on this file protects: under 32-bit stdcall the
// same struct occupies two stack slots and would have to be declared as two
// separate arguments.

func comDropTargetDragEnter(this, pDataObj, grfKeyState, pt uintptr, pdwEffect *uint32) uintptr {
	t := (*comDropTarget)(unsafe.Pointer(this))
	paths := win32PathsFromDataObject(pDataObj)
	t.mu.Lock()
	t.paths = paths
	t.mu.Unlock()
	DebugLog("WIN32_DND: IDropTarget.DragEnter carrying %d path(s)", len(paths))
	return t.answer(DragEnter, grfKeyState, pt, pdwEffect)
}

func comDropTargetDragOver(this, grfKeyState, pt uintptr, pdwEffect *uint32) uintptr {
	t := (*comDropTarget)(unsafe.Pointer(this))
	return t.answer(DragOver, grfKeyState, pt, pdwEffect)
}

func comDropTargetDragLeave(this uintptr) uintptr {
	t := (*comDropTarget)(unsafe.Pointer(this))
	t.mu.Lock()
	t.paths = nil
	t.mu.Unlock()
	DebugLog("WIN32_DND: IDropTarget.DragLeave")
	DeliverDragEvent(&DragEvent{Phase: DragLeave})
	return sOK
}

func comDropTargetDrop(this, pDataObj, grfKeyState, pt uintptr, pdwEffect *uint32) uintptr {
	t := (*comDropTarget)(unsafe.Pointer(this))
	// The data object handed to Drop is the authoritative one: a source is
	// free to fill in during the drop a format it only promised during the
	// drag, so ask again rather than trusting what DragEnter saw.
	if paths := win32PathsFromDataObject(pDataObj); len(paths) > 0 {
		t.mu.Lock()
		t.paths = paths
		t.mu.Unlock()
	}
	hr := t.answer(DragDrop, grfKeyState, pt, pdwEffect)
	t.mu.Lock()
	count := len(t.paths)
	t.paths = nil
	t.mu.Unlock()
	if pdwEffect != nil {
		DebugLog("WIN32_DND: IDropTarget.Drop of %d path(s) -> effect 0x%X", count, *pdwEffect)
	}
	return hr
}

// answer asks the application what a drop at this point would do and
// reports the verdict back through pdwEffect, which is what gives the
// pointer its copy / move cursor instead of a refusal.
func (t *comDropTarget) answer(phase DragPhase, grfKeyState, pt uintptr, pdwEffect *uint32) uintptr {
	if pdwEffect == nil {
		return ePointer
	}
	// On the way in pdwEffect holds the effects the source is willing to
	// perform; on the way out it holds the single one we chose.
	allowed := dropEffectToAllowedActions(*pdwEffect)

	t.mu.Lock()
	paths := t.paths
	host := t.host
	t.mu.Unlock()

	payload := DragPayload{Paths: paths}
	if len(paths) > 0 {
		payload.Kinds = []string{"text/uri-list"}
	}

	x, y := host.screenPointToCell(pt)
	mods := win32DragModifiers(grfKeyState)

	action := DeliverDragEvent(&DragEvent{
		Phase:     phase,
		X:         x,
		Y:         y,
		Modifiers: mods,
		Allowed:   allowed,
		Suggested: defaultDropAction(allowed, mods),
		Payload:   payload,
	})
	*pdwEffect = dropActionToDropEffect(action)
	return sOK
}

// dropEffectToAllowedActions turns a DROPEFFECT mask into the set of
// actions it permits. dropEffectToDropAction answers a different question
// -- which single action was chosen -- so it cannot serve here.
func dropEffectToAllowedActions(eff uint32) DropAction {
	var a DropAction
	if eff&dropEffectCopy != 0 {
		a |= DropCopy
	}
	if eff&dropEffectMove != 0 {
		a |= DropMove
	}
	if eff&dropEffectLink != 0 {
		a |= DropLink
	}
	return a
}

// defaultDropAction is what the drop would do if the target had no opinion:
// the modifier the user holds, read the way every Windows shell reads it,
// and copy when none is held because it is the one that cannot lose data.
// The target is free to override it; this only fills in Suggested.
func defaultDropAction(allowed DropAction, mods vtinput.ControlKeyState) DropAction {
	ctrl := mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	shift := mods&vtinput.ShiftPressed != 0
	switch {
	case ctrl && shift && allowed.Has(DropLink):
		return DropLink
	case ctrl && !shift && allowed.Has(DropCopy):
		return DropCopy
	case shift && !ctrl && allowed.Has(DropMove):
		return DropMove
	case allowed.Has(DropCopy):
		return DropCopy
	case allowed.Has(DropMove):
		return DropMove
	}
	return DropNone
}

// win32DragModifiers translates the key state OLE reports during a drag.
// That word cannot tell left from right, so ctrl and alt arrive as their
// left-hand variants; every reader of ControlKeyState tests for either.
func win32DragModifiers(grfKeyState uintptr) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if grfKeyState&mkShift != 0 {
		mods |= vtinput.ShiftPressed
	}
	if grfKeyState&mkControl != 0 {
		mods |= vtinput.LeftCtrlPressed
	}
	if grfKeyState&mkAlt != 0 {
		mods |= vtinput.LeftAltPressed
	}
	return mods
}

// screenPointToCell converts the screen-coordinate POINTL that OLE passes
// by value into a cell of our grid.
func (h *Win32GuiHost) screenPointToCell(pt uintptr) (int, int) {
	if h == nil {
		return 0, 0
	}
	p := win32Point{x: int32(uint32(pt)), y: int32(uint32(pt >> 32))}
	h.mu.Lock()
	hwnd, cellW, cellH := h.hwnd, h.cellW, h.cellH
	h.mu.Unlock()
	if hwnd == 0 || cellW <= 0 || cellH <= 0 {
		return 0, 0
	}
	procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&p)))
	return floorDiv(int(p.x), cellW), floorDiv(int(p.y), cellH)
}

// floorDiv rounds towards minus infinity. Go's own division truncates
// towards zero, which would map every pixel in the row just above the
// client area onto row 0 -- "just outside" must not read as "on the first
// line", or a drop aimed at the title bar lands in a panel.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// win32PathsFromDataObject asks a foreign IDataObject for CF_HDROP and
// decodes the DROPFILES block it hands back. Nothing here may assume the
// object is one of ours, so the call goes through its vtable.
func win32PathsFromDataObject(pDataObj uintptr) []string {
	if pDataObj == 0 {
		return nil
	}
	fe := formatEtc{
		cfFormat: cfHDROP,
		dwAspect: dvAspectContent,
		lindex:   -1,
		tymed:    tymedHGLOBAL,
	}
	var med stgMedium
	vtbl := *(**[16]uintptr)(unsafe.Pointer(pDataObj))
	hr, _, _ := syscall.SyscallN(vtbl[slotDataObjectGetData],
		pDataObj, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&med)))
	if uint32(hr) != sOK {
		DebugLog("WIN32_DND: IDataObject.GetData(CF_HDROP) failed hr=0x%08X", uint32(hr))
		return nil
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&med)))
	if med.tymed != tymedHGLOBAL || med.handle == 0 {
		return nil
	}
	size, _, _ := procGlobalSize.Call(med.handle)
	ptr, _, _ := procGlobalLock.Call(med.handle)
	if ptr == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(med.handle)
	return parseHDROP(ptr, size)
}

// parseHDROP walks the file list of a DROPFILES block: a run of
// NUL-terminated names closed by an empty one. size bounds the walk, so a
// block that is malformed or missing its final terminator cannot send this
// off the end of the allocation.
func parseHDROP(base, size uintptr) []string {
	header := unsafe.Sizeof(dropFiles{})
	if size < header {
		return nil
	}
	df := (*dropFiles)(unsafe.Pointer(base))
	off := uintptr(df.pFiles)
	if off < header || off >= size {
		return nil
	}

	var out []string
	if df.fWide != 0 {
		for off+2 <= size {
			var name []uint16
			for off+2 <= size {
				c := *(*uint16)(unsafe.Pointer(base + off))
				off += 2
				if c == 0 {
					break
				}
				name = append(name, c)
			}
			if len(name) == 0 {
				break
			}
			out = append(out, string(utf16.Decode(name)))
		}
		return out
	}
	for off < size {
		var name []byte
		for off < size {
			c := *(*byte)(unsafe.Pointer(base + off))
			off++
			if c == 0 {
				break
			}
			name = append(name, c)
		}
		if len(name) == 0 {
			break
		}
		out = append(out, string(name))
	}
	return out
}

// win32RegisterDropTarget makes the window an OLE drop target. It must run
// on the thread that called OleInitialize and that pumps the window's
// messages, because that is the apartment OLE will call the target back on.
//
// WS_EX_ACCEPTFILES is deliberately left in place: a source that only knows
// how to post WM_DROPFILES still works, and if RegisterDragDrop fails the
// old path is all that is left.
func win32RegisterDropTarget(h *Win32GuiHost, hwnd syscall.Handle) {
	if h == nil || hwnd == 0 {
		return
	}
	oleInit()
	t := newComDropTarget(h)
	hr, _, _ := procRegisterDragDrop.Call(uintptr(hwnd), t.toIUnknown())
	if uint32(hr) != sOK {
		DebugLog("WIN32_DND: RegisterDragDrop failed hr=0x%08X, only WM_DROPFILES will work", uint32(hr))
		comDropTargetRelease(t.toIUnknown())
		return
	}
	h.mu.Lock()
	h.dropTarget = t.toIUnknown()
	h.mu.Unlock()
	DebugLog("WIN32_DND: RegisterDragDrop succeeded, the window now answers drags")
}

// win32RevokeDropTarget undoes the registration. Like the registration it
// belongs on the message thread, which is why it is called from WM_DESTROY
// rather than from Close.
func win32RevokeDropTarget(h *Win32GuiHost, hwnd syscall.Handle) {
	if h == nil {
		return
	}
	h.mu.Lock()
	this := h.dropTarget
	h.dropTarget = 0
	h.mu.Unlock()
	if this == 0 {
		return
	}
	if hwnd != 0 {
		procRevokeDragDrop.Call(uintptr(hwnd))
	}
	comDropTargetRelease(this)
}
