package vtui

import (
	"golang.org/x/image/font"
)

// Win32GuiRenderer renders the character grid into a 32-bit software bitmap
// and blits it to a native Win32 window HDC using GDI (SetDIBitsToDevice/BitBlt).
// The grid itself is composed by the embedded gridRaster, which the Cocoa
// backend shares; what is here is the GDI side of it.
type Win32GuiRenderer struct {
	gridRaster
	host *Win32GuiHost

	bgraBuf []byte

	// Windows-only double-buffer handles (GDI memory DC + compatible
	// bitmap). Typed as uintptr, not syscall.Handle, so this struct stays
	// buildable on every platform: this file has no build tag, only
	// win32_gui_windows.go (which does) touches these as GDI handles.
	// See blitTo() in win32_gui_windows.go and f4 issue #514.
	memDC      uintptr
	memBitmap  uintptr
	memBits    uintptr
	memW, memH int
}

func NewWin32GuiRenderer(host *Win32GuiHost, face font.Face, cellW, cellH int) *Win32GuiRenderer {
	scale := 1
	if host != nil && host.scale > 1 {
		scale = host.scale
	}
	return &Win32GuiRenderer{
		gridRaster: newGridRaster(face, cellW, cellH, scale),
		host:       host,
	}
}

// SetFont changes the font of the already-open window without recreating
// it: see Win32GuiHost.SetFont. It reports true whenever it has a host to
// forward to -- Wayland, X11 and Win32 are, as of this part, the GUI
// backends implementing font hot-swap (vtui #136) -- and false only for a
// renderer built without one, the same nil guard ToggleMaximized uses.
func (r *Win32GuiRenderer) SetFont(fontName string, fontSize float64) bool {
	if r.host == nil {
		return false
	}
	r.host.SetFont(fontName, fontSize)
	return true
}

func (r *Win32GuiRenderer) SetWindowTitle(title string) {
	if r.host != nil {
		r.host.SetTitle(title)
	}
}

func (r *Win32GuiRenderer) ResizeWindow(cols, rows int) {
	if r.host != nil {
		r.host.ResizeGrid(cols, rows)
	}
}

// ToggleMaximized maximizes the window or restores it.
func (r *Win32GuiRenderer) ToggleMaximized() bool {
	if r.host == nil {
		return false
	}
	return r.host.ToggleMaximized()
}

func (r *Win32GuiRenderer) Flush() {
	r.mu.Lock()
	dirty := r.dirty
	r.dirty = false
	r.mu.Unlock()

	if r.host == nil {
		return
	}
	// Re-invalidate while a previous invalidation has not yet produced a
	// painted frame. Without this the backend gets exactly one chance per
	// content change: BeginPaint validates the update region whether or not
	// anything was blitted, and an idle UI never changes a row again, so a
	// single lost or empty WM_PAINT leaves the window blank until the next
	// keystroke -- or forever. FrameManager already ticks this renderer at
	// ~250ms via the software-blink heartbeat, so recovery is automatic.
	if dirty || r.host.paintOutstanding() {
		r.host.Invalidate()
	}
}

func (r *Win32GuiRenderer) syncBGRA() (w, h int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.syncBGRALocked()
}

// syncBGRALocked converts the composed RGBA frame into the BGRA scratch buffer
// GDI expects. The caller must hold r.mu, and must keep holding it for as long
// as it uses bgraBuf: Render() reallocates imgBuf when the grid resizes, and
// bgraBuf follows it here.
func (r *Win32GuiRenderer) syncBGRALocked() (w, h int, ok bool) {
	if r.imgBuf == nil || len(r.imgBuf.Pix) == 0 {
		return 0, 0, false
	}
	w = r.imgBuf.Rect.Dx()
	h = r.imgBuf.Rect.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}

	lineStride := w * 4
	if len(r.bgraBuf) != len(r.imgBuf.Pix) {
		r.bgraBuf = make([]byte, len(r.imgBuf.Pix))
	}

	for y := 0; y < h; y++ {
		off := y * lineStride
		rgbaToBGRA(r.bgraBuf[off:off+lineStride], r.imgBuf.Pix[off:off+lineStride], lineStride)
	}
	return w, h, true
}

// needsIdleBlinkHeartbeat marks Win32GuiRenderer as needing the idle
// blink heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *Win32GuiRenderer) needsIdleBlinkHeartbeat() {}
