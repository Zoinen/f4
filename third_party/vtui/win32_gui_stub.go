//go:build !windows

package vtui

import (
	"fmt"
)

type Win32GuiHost struct {
	scale     int
	closeChan chan struct{}
	closed    bool
}

func (h *Win32GuiHost) SetTitle(title string)     {}
func (h *Win32GuiHost) ResizeGrid(cols, rows int) {}
func (h *Win32GuiHost) Invalidate()               {}
func (h *Win32GuiHost) paintOutstanding() bool    { return false }
func (h *Win32GuiHost) PostQuit() {
	if !h.closed {
		h.closed = true
		if h.closeChan != nil {
			close(h.closeChan)
		}
	}
}

func RunWin32GuiHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return fmt.Errorf("Win32 GUI backend is only supported on Windows and Wine")
}

func runInWin32Window(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return fmt.Errorf("Win32 GUI backend is only supported on Windows and Wine")
}
