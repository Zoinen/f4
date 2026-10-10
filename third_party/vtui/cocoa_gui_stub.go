//go:build !darwin || ios || vtui_nococoa

package vtui

import (
	"fmt"
	"runtime"
)

// CocoaGuiHost is the stub for builds without the Cocoa backend: every
// platform but macOS, and macOS builds tagged vtui_nococoa. It keeps
// CocoaGuiRenderer, which has no build tag, compiling and testable.
type CocoaGuiHost struct {
	scale int
}

func (h *CocoaGuiHost) SetTitle(title string)               {}
func (h *CocoaGuiHost) ResizeGrid(cols, rows int)           {}
func (h *CocoaGuiHost) ToggleMaximized() bool               { return false }
func (h *CocoaGuiHost) Invalidate()                         {}
func (h *CocoaGuiHost) WindowPosition() (x, y int, ok bool) { return 0, 0, false }
func (h *CocoaGuiHost) SetWindowPosition(x, y int)          {}

// RunCocoaGuiHost reports that this binary has no Cocoa backend.
func RunCocoaGuiHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	if runtime.GOOS == "darwin" {
		return fmt.Errorf("cocoa GUI backend is not built into this binary")
	}
	return fmt.Errorf("cocoa GUI backend is only supported on macOS")
}

func runInCocoaWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return RunCocoaGuiHost(cols, rows, fontName, fontSize, setupApp)
}
