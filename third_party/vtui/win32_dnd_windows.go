//go:build windows

package vtui

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	ole32DLL          = syscall.NewLazyDLL("ole32.dll")
	procOleInitialize = ole32DLL.NewProc("OleInitialize")
	procDoDragDrop    = ole32DLL.NewProc("DoDragDrop")
)

var (
	oleInitOnce             sync.Once
	initComVtablesOnce      sync.Once
	globalDropSourceVtbl    *dropSourceVtbl
	globalDataObjectVtbl    *dataObjectVtbl
	globalEnumFormatEtcVtbl *enumFormatEtcVtbl
)

func oleInit() {
	oleInitOnce.Do(func() {
		procOleInitialize.Call(0)
	})
}

// Every COM object below is an ordinary Go value, and OLE is handed nothing
// but its address, which the garbage collector cannot see. The enumerator
// returned by IDataObject::EnumFormatEtc shows the hole at its sharpest: it
// is allocated inside a callback, its address is written into caller-owned
// memory, and it is unreachable from Go the instant that callback returns,
// so the collector may reuse it while the drop target is still walking it.
// The target then finds no CF_HDROP and refuses the drop, which is the "no"
// cursor. The data object and the drop source have the same hole for any
// drag that outlives the frame which created them.
//
// comLive is the Go reference COM cannot provide: an object enters it when
// it is born with a reference count of one and leaves it when that count
// reaches zero, which is exactly the lifetime COM promises. runtime.Pinner
// is deliberately not used here -- a Pinner collected without Unpin panics,
// so a client that never releases to zero would take the process down
// instead of merely leaking one object.
var (
	comLiveMu sync.Mutex
	comLive   = map[uintptr]any{}
)

// comRetain makes obj reachable from Go for as long as OLE holds it, and
// returns the address OLE will know it by.
func comRetain(this uintptr, obj any) uintptr {
	comLiveMu.Lock()
	comLive[this] = obj
	comLiveMu.Unlock()
	return this
}

// comReleaseFinal drops the last Go reference to an object whose reference
// count has just reached zero. After this the address may be reused, so no
// caller may touch the object again.
func comReleaseFinal(this uintptr) {
	comLiveMu.Lock()
	delete(comLive, this)
	comLiveMu.Unlock()
}

// comLiveCount reports how many objects are still held. Only tests need it.
func comLiveCount() int {
	comLiveMu.Lock()
	defer comLiveMu.Unlock()
	return len(comLive)
}

func initComVtables() {
	initComVtablesOnce.Do(func() {
		globalDropSourceVtbl = &dropSourceVtbl{
			QueryInterface:    syscall.NewCallback(comDropSourceQueryInterface),
			AddRef:            syscall.NewCallback(comDropSourceAddRef),
			Release:           syscall.NewCallback(comDropSourceRelease),
			QueryContinueDrag: syscall.NewCallback(comDropSourceQueryContinueDrag),
			GiveFeedback:      syscall.NewCallback(comDropSourceGiveFeedback),
		}
		globalDataObjectVtbl = &dataObjectVtbl{
			QueryInterface:        syscall.NewCallback(comDataObjectQueryInterface),
			AddRef:                syscall.NewCallback(comDataObjectAddRef),
			Release:               syscall.NewCallback(comDataObjectRelease),
			GetData:               syscall.NewCallback(comDataObjectGetData),
			GetDataHere:           syscall.NewCallback(comDataObjectGetDataHere),
			QueryGetData:          syscall.NewCallback(comDataObjectQueryGetData),
			GetCanonicalFormatEtc: syscall.NewCallback(comDataObjectGetCanonicalFormatEtc),
			SetData:               syscall.NewCallback(comDataObjectSetData),
			EnumFormatEtc:         syscall.NewCallback(comDataObjectEnumFormatEtc),
			DAdvise:               syscall.NewCallback(comDataObjectDAdvise),
			DUnadvise:             syscall.NewCallback(comDataObjectDUnadvise),
			EnumDAdvise:           syscall.NewCallback(comDataObjectEnumDAdvise),
		}
		globalEnumFormatEtcVtbl = &enumFormatEtcVtbl{
			QueryInterface: syscall.NewCallback(comEnumFormatEtcQueryInterface),
			AddRef:         syscall.NewCallback(comEnumFormatEtcAddRef),
			Release:        syscall.NewCallback(comEnumFormatEtcRelease),
			Next:           syscall.NewCallback(comEnumFormatEtcNext),
			Skip:           syscall.NewCallback(comEnumFormatEtcSkip),
			Reset:          syscall.NewCallback(comEnumFormatEtcReset),
			Clone:          syscall.NewCallback(comEnumFormatEtcClone),
		}
	})
}

// Win32 OLE / COM constants and HRESULTs
const (
	sOK                        = 0x00000000
	sFalse                     = 0x00000001
	eNoInterface               = 0x80004002
	ePointer                   = 0x80004003
	eNotImpl                   = 0x80004001
	eOutOfMemory               = 0x8007000E
	dvEFormatEtc               = 0x80040064
	dvETymed                   = 0x80040069
	dragDropSDrop              = 0x00040100
	dragDropSCancel            = 0x00040101
	dragDropSUseDefaultCursors = 0x00040102

	dropEffectNone = 0
	dropEffectCopy = 1
	dropEffectMove = 2
	dropEffectLink = 4

	cfText        = 1
	cfUnicodeText = 13
	cfHDROP       = 15
	tymedHGLOBAL  = 1
	dataDirGet    = 1
)

func buildUnicodeText(paths []string) (uintptr, error) {
	if len(paths) == 0 {
		return 0, ErrDragNoData
	}
	text := ""
	for i, p := range paths {
		if i > 0 {
			text += "\r\n"
		}
		text += p
	}
	u16, err := syscall.UTF16FromString(text)
	if err != nil {
		return 0, err
	}
	totalBytes := uintptr(len(u16) * 2)

	const gmemMoveable = 0x0002
	const gmemZeroInit = 0x0040
	hGlobal, _, _ := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, totalBytes)
	if hGlobal == 0 {
		return 0, fmt.Errorf("GlobalAlloc failed for %d bytes", totalBytes)
	}

	ptr, _, _ := procGlobalLock.Call(hGlobal)
	if ptr == 0 {
		procGlobalFree.Call(hGlobal)
		return 0, fmt.Errorf("GlobalLock failed")
	}

	destSlice := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(u16))
	copy(destSlice, u16)

	procGlobalUnlock.Call(hGlobal)
	return hGlobal, nil
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	iidIUnknown       = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIDropSource    = guid{0x00000121, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIDataObject    = guid{0x0000010E, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIEnumFORMATETC = guid{0x00000103, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

func guidEqual(a, b guid) bool {
	return a.Data1 == b.Data1 && a.Data2 == b.Data2 && a.Data3 == b.Data3 && a.Data4 == b.Data4
}

// formatEtc represents the COM FORMATETC struct with portable 32/64-bit alignment.
type formatEtc struct {
	cfFormat uint16
	_        uint16
	ptd      uintptr
	dwAspect uint32
	lindex   int32
	tymed    uint32
}

// stgMedium represents the COM STGMEDIUM struct.
type stgMedium struct {
	tymed          uint32
	handle         uintptr
	pUnkForRelease uintptr
}

// dropFiles represents the Windows DROPFILES struct for CF_HDROP.
type dropFiles struct {
	pFiles uint32 // Offset of file list from beginning of structure (20 bytes)
	ptX    int32
	ptY    int32
	fNC    int32 // Non-client area flag (0 = client)
	fWide  int32 // 1 = UTF-16, 0 = ANSI
}

// buildHDROP creates a GlobalAlloc HGLOBAL containing a CF_HDROP structure for paths.
func buildHDROP(paths []string) (uintptr, error) {
	if len(paths) == 0 {
		return 0, ErrDragNoData
	}
	var utf16Buf []uint16
	for _, p := range paths {
		if p == "" {
			continue
		}
		u, err := syscall.UTF16FromString(p)
		if err != nil {
			continue
		}
		utf16Buf = append(utf16Buf, u...)
	}
	if len(utf16Buf) == 0 {
		return 0, ErrDragNoData
	}
	// Double null terminator for the list
	utf16Buf = append(utf16Buf, 0)

	dfSize := uint32(unsafe.Sizeof(dropFiles{}))
	totalBytes := uintptr(dfSize) + uintptr(len(utf16Buf)*2)

	const gmemMoveable = 0x0002
	const gmemZeroInit = 0x0040
	hGlobal, _, _ := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, totalBytes)
	if hGlobal == 0 {
		return 0, fmt.Errorf("GlobalAlloc failed for %d bytes", totalBytes)
	}

	ptr, _, _ := procGlobalLock.Call(hGlobal)
	if ptr == 0 {
		procGlobalFree.Call(hGlobal)
		return 0, fmt.Errorf("GlobalLock failed")
	}

	df := (*dropFiles)(unsafe.Pointer(ptr))
	df.pFiles = dfSize
	df.fWide = 1 // UTF-16

	destSlice := unsafe.Slice((*uint16)(unsafe.Pointer(ptr+uintptr(dfSize))), len(utf16Buf))
	copy(destSlice, utf16Buf)

	procGlobalUnlock.Call(hGlobal)
	return hGlobal, nil
}

func dropActionToDropEffect(allowed DropAction) uint32 {
	var eff uint32
	if allowed&DropCopy != 0 {
		eff |= dropEffectCopy
	}
	if allowed&DropMove != 0 {
		eff |= dropEffectMove
	}
	if allowed&DropLink != 0 {
		eff |= dropEffectLink
	}
	return eff
}

func dropEffectToDropAction(eff uint32) DropAction {
	switch {
	case eff&dropEffectCopy != 0:
		return DropCopy
	case eff&dropEffectMove != 0:
		return DropMove
	case eff&dropEffectLink != 0:
		return DropLink
	default:
		return DropNone
	}
}

// --- IDropSource Implementation ---

type dropSourceVtbl struct {
	QueryInterface    uintptr
	AddRef            uintptr
	Release           uintptr
	QueryContinueDrag uintptr
	GiveFeedback      uintptr
}

type comDropSource struct {
	lpVtbl   *dropSourceVtbl
	refCount int32
}

func newComDropSource() *comDropSource {
	initComVtables()
	s := &comDropSource{
		lpVtbl:   globalDropSourceVtbl,
		refCount: 1,
	}
	comRetain(uintptr(unsafe.Pointer(s)), s)
	return s
}

func (s *comDropSource) toIUnknown() uintptr {
	return uintptr(unsafe.Pointer(s))
}

func comDropSourceQueryInterface(this uintptr, riid *guid, ppvObject *uintptr) uintptr {
	if ppvObject == nil {
		return ePointer
	}
	if riid == nil {
		return eNoInterface
	}
	if guidEqual(*riid, iidIUnknown) || guidEqual(*riid, iidIDropSource) {
		*ppvObject = this
		s := (*comDropSource)(unsafe.Pointer(this))
		atomic.AddInt32(&s.refCount, 1)
		return sOK
	}
	*ppvObject = 0
	return eNoInterface
}

func comDropSourceAddRef(this uintptr) uintptr {
	s := (*comDropSource)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&s.refCount, 1))
}

func comDropSourceRelease(this uintptr) uintptr {
	s := (*comDropSource)(unsafe.Pointer(this))
	count := atomic.AddInt32(&s.refCount, -1)
	if count <= 0 {
		comReleaseFinal(this)
	}
	return uintptr(count)
}

func comDropSourceQueryContinueDrag(this uintptr, fEscapePressed uintptr, grfKeyState uintptr) uintptr {
	if fEscapePressed != 0 {
		DebugLog("WIN32_DND_COM: QueryContinueDrag -> ESC pressed, cancel")
		return dragDropSCancel
	}
	// MK_LBUTTON (0x0001) or MK_RBUTTON (0x0002) released -> drop
	if (grfKeyState & 0x0003) == 0 {
		DebugLog("WIN32_DND_COM: QueryContinueDrag -> mouse released, drop")
		return dragDropSDrop
	}
	return sOK
}

func comDropSourceGiveFeedback(this uintptr, dwEffect uintptr) uintptr {
	return dragDropSUseDefaultCursors
}

// --- IDataObject Implementation ---

type dataObjectVtbl struct {
	QueryInterface        uintptr
	AddRef                uintptr
	Release               uintptr
	GetData               uintptr
	GetDataHere           uintptr
	QueryGetData          uintptr
	GetCanonicalFormatEtc uintptr
	SetData               uintptr
	EnumFormatEtc         uintptr
	DAdvise               uintptr
	DUnadvise             uintptr
	EnumDAdvise           uintptr
}

type comDataObject struct {
	lpVtbl   *dataObjectVtbl
	refCount int32
	paths    []string
}

func newComDataObject(paths []string) *comDataObject {
	initComVtables()
	d := &comDataObject{
		lpVtbl:   globalDataObjectVtbl,
		refCount: 1,
		paths:    paths,
	}
	comRetain(uintptr(unsafe.Pointer(d)), d)
	return d
}

func (d *comDataObject) toIUnknown() uintptr {
	return uintptr(unsafe.Pointer(d))
}

func comDataObjectQueryInterface(this uintptr, riid *guid, ppvObject *uintptr) uintptr {
	if ppvObject == nil {
		return ePointer
	}
	if riid == nil {
		return eNoInterface
	}
	if guidEqual(*riid, iidIUnknown) || guidEqual(*riid, iidIDataObject) {
		*ppvObject = this
		d := (*comDataObject)(unsafe.Pointer(this))
		atomic.AddInt32(&d.refCount, 1)
		return sOK
	}
	*ppvObject = 0
	return eNoInterface
}

func comDataObjectAddRef(this uintptr) uintptr {
	d := (*comDataObject)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&d.refCount, 1))
}

func comDataObjectRelease(this uintptr) uintptr {
	d := (*comDataObject)(unsafe.Pointer(this))
	count := atomic.AddInt32(&d.refCount, -1)
	if count <= 0 {
		comReleaseFinal(this)
	}
	return uintptr(count)
}

func comDataObjectGetData(this uintptr, pFormatEtcIn *formatEtc, pMedium *stgMedium) uintptr {
	if pFormatEtcIn == nil || pMedium == nil {
		return ePointer
	}
	DebugLog("WIN32_DND_COM: IDataObject.GetData requested cfFormat=%d, tymed=%d", pFormatEtcIn.cfFormat, pFormatEtcIn.tymed)
	if pFormatEtcIn.tymed&tymedHGLOBAL == 0 {
		return dvETymed
	}

	d := (*comDataObject)(unsafe.Pointer(this))

	if pFormatEtcIn.cfFormat == cfHDROP {
		hGlobal, err := buildHDROP(d.paths)
		if err != nil {
			DebugLog("WIN32_DND_COM: IDataObject.GetData buildHDROP failed: %v", err)
			return eOutOfMemory
		}
		pMedium.tymed = tymedHGLOBAL
		pMedium.handle = hGlobal
		pMedium.pUnkForRelease = 0
		DebugLog("WIN32_DND_COM: IDataObject.GetData supplied CF_HDROP for %d path(s)", len(d.paths))
		return sOK
	}

	if pFormatEtcIn.cfFormat == cfUnicodeText {
		hGlobal, err := buildUnicodeText(d.paths)
		if err != nil {
			DebugLog("WIN32_DND_COM: IDataObject.GetData buildUnicodeText failed: %v", err)
			return eOutOfMemory
		}
		pMedium.tymed = tymedHGLOBAL
		pMedium.handle = hGlobal
		pMedium.pUnkForRelease = 0
		DebugLog("WIN32_DND_COM: IDataObject.GetData supplied CF_UNICODETEXT for %d path(s)", len(d.paths))
		return sOK
	}

	return dvEFormatEtc
}

func comDataObjectGetDataHere(this uintptr, pFormatEtc *formatEtc, pMedium *stgMedium) uintptr {
	return eNotImpl
}

func comDataObjectQueryGetData(this uintptr, pFormatEtc *formatEtc) uintptr {
	if pFormatEtc == nil {
		return ePointer
	}
	DebugLog("WIN32_DND_COM: IDataObject.QueryGetData cfFormat=%d, tymed=%d", pFormatEtc.cfFormat, pFormatEtc.tymed)
	if (pFormatEtc.cfFormat == cfHDROP || pFormatEtc.cfFormat == cfUnicodeText) && (pFormatEtc.tymed&tymedHGLOBAL) != 0 {
		return sOK
	}
	return dvEFormatEtc
}

func comDataObjectGetCanonicalFormatEtc(this uintptr, pFormatEtcIn *formatEtc, pFormatEtcOut *formatEtc) uintptr {
	return eNotImpl
}

func comDataObjectSetData(this uintptr, pFormatEtc *formatEtc, pMedium *stgMedium, fRelease uintptr) uintptr {
	return eNotImpl
}

func comDataObjectEnumFormatEtc(this uintptr, dwDirection uintptr, ppEnumFormatEtc *uintptr) uintptr {
	if ppEnumFormatEtc == nil {
		return ePointer
	}
	DebugLog("WIN32_DND_COM: IDataObject.EnumFormatEtc dwDirection=%d", dwDirection)
	if dwDirection != dataDirGet {
		return eNotImpl
	}

	formats := []formatEtc{
		{
			cfFormat: cfHDROP,
			dwAspect: 1, // DVASPECT_CONTENT
			lindex:   -1,
			tymed:    tymedHGLOBAL,
		},
		{
			cfFormat: cfUnicodeText,
			dwAspect: 1, // DVASPECT_CONTENT
			lindex:   -1,
			tymed:    tymedHGLOBAL,
		},
	}
	enumObj := newComEnumFormatEtc(formats)
	*ppEnumFormatEtc = enumObj.toIUnknown()
	return sOK
}

func comDataObjectDAdvise(this, pFormatEtc, advf, pAdvSink, pdwConnection uintptr) uintptr {
	return eNotImpl
}

func comDataObjectDUnadvise(this, dwConnection uintptr) uintptr {
	return eNotImpl
}

func comDataObjectEnumDAdvise(this, ppenumAdvise uintptr) uintptr {
	return eNotImpl
}

// --- IEnumFORMATETC Implementation ---

type enumFormatEtcVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	Next           uintptr
	Skip           uintptr
	Reset          uintptr
	Clone          uintptr
}

type comEnumFormatEtc struct {
	lpVtbl   *enumFormatEtcVtbl
	refCount int32
	formats  []formatEtc
	index    int
}

func newComEnumFormatEtc(formats []formatEtc) *comEnumFormatEtc {
	initComVtables()
	e := &comEnumFormatEtc{
		lpVtbl:   globalEnumFormatEtcVtbl,
		refCount: 1,
		formats:  formats,
		index:    0,
	}
	comRetain(uintptr(unsafe.Pointer(e)), e)
	return e
}

func (e *comEnumFormatEtc) toIUnknown() uintptr {
	return uintptr(unsafe.Pointer(e))
}

func comEnumFormatEtcQueryInterface(this uintptr, riid *guid, ppvObject *uintptr) uintptr {
	if ppvObject == nil {
		return ePointer
	}
	if riid == nil {
		return eNoInterface
	}
	if guidEqual(*riid, iidIUnknown) || guidEqual(*riid, iidIEnumFORMATETC) {
		*ppvObject = this
		e := (*comEnumFormatEtc)(unsafe.Pointer(this))
		atomic.AddInt32(&e.refCount, 1)
		return sOK
	}
	*ppvObject = 0
	return eNoInterface
}

func comEnumFormatEtcAddRef(this uintptr) uintptr {
	e := (*comEnumFormatEtc)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&e.refCount, 1))
}

func comEnumFormatEtcRelease(this uintptr) uintptr {
	e := (*comEnumFormatEtc)(unsafe.Pointer(this))
	count := atomic.AddInt32(&e.refCount, -1)
	if count <= 0 {
		comReleaseFinal(this)
	}
	return uintptr(count)
}

func comEnumFormatEtcNext(this uintptr, celt uintptr, rgelt *formatEtc, pceltFetched *uint32) uintptr {
	if rgelt == nil {
		return ePointer
	}
	e := (*comEnumFormatEtc)(unsafe.Pointer(this))

	requested := int(celt)
	fetched := 0

	outSlice := unsafe.Slice(rgelt, requested)
	for fetched < requested && e.index < len(e.formats) {
		outSlice[fetched] = e.formats[e.index]
		e.index++
		fetched++
	}

	if pceltFetched != nil {
		*pceltFetched = uint32(fetched)
	}

	if fetched == requested {
		return sOK
	}
	return sFalse
}

func comEnumFormatEtcSkip(this uintptr, celt uintptr) uintptr {
	e := (*comEnumFormatEtc)(unsafe.Pointer(this))
	e.index += int(celt)
	if e.index > len(e.formats) {
		e.index = len(e.formats)
		return sFalse
	}
	return sOK
}

func comEnumFormatEtcReset(this uintptr) uintptr {
	e := (*comEnumFormatEtc)(unsafe.Pointer(this))
	e.index = 0
	return sOK
}

func comEnumFormatEtcClone(this uintptr, ppenum *uintptr) uintptr {
	if ppenum == nil {
		return ePointer
	}
	e := (*comEnumFormatEtc)(unsafe.Pointer(this))
	cloned := newComEnumFormatEtc(e.formats)
	cloned.index = e.index
	*ppenum = cloned.toIUnknown()
	return sOK
}

// win32DoDragDrop invokes ole32!DoDragDrop for the given file paths and allowed actions.
func win32DoDragDrop(paths []string, allowed DropAction) (DropAction, error) {
	if len(paths) == 0 {
		return DropNone, ErrDragNoData
	}
	oleInit()

	dataObj := newComDataObject(paths)
	dropSrc := newComDropSource()

	dwOKEffects := dropActionToDropEffect(allowed)
	var dwEffect uint32

	DebugLog("WIN32_DND: calling ole32!DoDragDrop with %d path(s), dwOKEffects=0x%X...", len(paths), dwOKEffects)
	r1, _, _ := procDoDragDrop.Call(
		dataObj.toIUnknown(),
		dropSrc.toIUnknown(),
		uintptr(dwOKEffects),
		uintptr(unsafe.Pointer(&dwEffect)),
	)
	DebugLog("WIN32_DND: ole32!DoDragDrop returned hr=0x%08X, dwEffect=0x%X", uint32(r1), dwEffect)

	// Release COM objects
	comDataObjectRelease(dataObj.toIUnknown())
	comDropSourceRelease(dropSrc.toIUnknown())

	if r1 == uintptr(dragDropSDrop) {
		return dropEffectToDropAction(dwEffect), nil
	}
	if r1 == uintptr(dragDropSCancel) {
		return DropNone, nil
	}
	if r1 != 0 {
		return DropNone, fmt.Errorf("DoDragDrop failed with HRESULT: 0x%08X", uint32(r1))
	}
	return dropEffectToDropAction(dwEffect), nil
}
