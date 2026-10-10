package vtui

import (
	"golang.org/x/image/font"
)

// CocoaGuiRenderer renders the character grid into a 32-bit software bitmap
// with the raster the Win32 GDI backend uses (gridRaster), and hands each
// frame to Core Animation as the contents of the window's layer; see
// presentFrame in cocoa_gui_darwin.go. Like Win32GuiRenderer it has no build
// tag: only the host behind it is macOS-specific.
type CocoaGuiRenderer struct {
	gridRaster
	host *CocoaGuiHost
}

func NewCocoaGuiRenderer(host *CocoaGuiHost, face font.Face, cellW, cellH int) *CocoaGuiRenderer {
	scale := 1
	if host != nil && host.scale > 1 {
		scale = host.scale
	}
	return &CocoaGuiRenderer{
		gridRaster: newGridRaster(face, cellW, cellH, scale),
		host:       host,
	}
}

func (r *CocoaGuiRenderer) SetWindowTitle(title string) {
	if r.host != nil {
		r.host.SetTitle(title)
	}
}

func (r *CocoaGuiRenderer) ResizeWindow(cols, rows int) {
	if r.host != nil {
		r.host.ResizeGrid(cols, rows)
	}
}

// ToggleMaximized zooms the window to fill the screen, or back.
func (r *CocoaGuiRenderer) ToggleMaximized() bool {
	if r.host == nil {
		return false
	}
	return r.host.ToggleMaximized()
}

// WindowPosition returns the top-left corner of the window on screen, in
// points from the top-left corner of the main display.
func (r *CocoaGuiRenderer) WindowPosition() (x, y int, ok bool) {
	if r == nil || r.host == nil {
		return 0, 0, false
	}
	return r.host.WindowPosition()
}

// SetWindowPosition moves the window, in the coordinates WindowPosition
// reports.
func (r *CocoaGuiRenderer) SetWindowPosition(x, y int) {
	if r != nil && r.host != nil {
		r.host.SetWindowPosition(x, y)
	}
}

// Flush asks the window for a display pass when the last Render changed
// anything. The pass itself runs on the main thread, where AppKit wants it,
// and takes whatever frame is newest by then; see CocoaGuiHost.Invalidate.
func (r *CocoaGuiRenderer) Flush() {
	r.mu.Lock()
	dirty := r.dirty
	r.dirty = false
	r.mu.Unlock()

	if dirty && r.host != nil {
		r.host.Invalidate()
	}
}

// composeCanvas lays the newest frame onto a w x h canvas (see
// composeCocoaCanvas) under the renderer's lock, so Render cannot replace
// the frame halfway through the copy.
func (r *CocoaGuiRenderer) composeCanvas(dst []byte, w, h, stride int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	composeCocoaCanvas(dst, w, h, stride, r.imgBuf)
}

// needsIdleBlinkHeartbeat marks CocoaGuiRenderer as needing the idle blink
// heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *CocoaGuiRenderer) needsIdleBlinkHeartbeat() {}
