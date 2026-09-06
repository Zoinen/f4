package main

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Only physical application focus events pass through EventFilter. Frame
// activation (switching tabs, dismissing a document or dialog) is sent directly
// to ProcessKey and must not reread shared macro files.
func applicationActivationFilter(next func(*vtinput.InputEvent) bool) func(*vtinput.InputEvent) bool {
	focused := true // Startup already loaded the macros.
	return func(event *vtinput.InputEvent) bool {
		if event != nil && event.Type == vtinput.FocusEventType {
			if event.SetFocus && !focused && MacroMgr != nil {
				MacroMgr.Load()
				navigationBenchmarkUIEvent("application.activation.macros_reload")
			}
			focused = event.SetFocus
		}
		return next != nil && next(event)
	}
}

func (pf *PanelsFrame) pendingDocumentOpenOwnsKey(event *vtinput.InputEvent) bool {
	return pendingDocumentOpens[pf] != nil && event != nil &&
		event.Type == vtinput.KeyEventType && event.KeyDown && event.VirtualKeyCode == vtinput.VK_ESCAPE &&
		event.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed|vtinput.ShiftPressed) == 0
}

// An unregistered frame is still being constructed and may initialize its
// terminal. A registered background workspace retains its current terminal
// geometry until activation, even while ordinary panel layout is resized.
func (pf *PanelsFrame) terminalWorkspaceActive() bool {
	if vtui.FrameManager == nil {
		return true
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if frame == pf {
			return true
		}
	}
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			if frame == pf {
				return false
			}
		}
	}
	return true
}

type ptyGeometry struct {
	columns, rows, cellWidth, cellHeight int
}

// The caller holds ptyMutex. This state belongs to the live child connection:
// repeated layout calls send no ioctl/ConPTY request when nothing changed.
func (pf *PanelsFrame) negotiatePTYGeometry(pty PtyBackend, columns, rows, cellWidth, cellHeight int) {
	if pty == nil {
		return
	}
	if _, pixels := pty.(PtyPixelSizer); !pixels {
		cellWidth, cellHeight = 0, 0
	}
	geometry := ptyGeometry{columns, rows, cellWidth, cellHeight}
	if previous, known := pf.lastPTYGeometry[pty]; known && previous == geometry {
		return
	}
	acknowledged, err := setPtySize(pty, columns, rows, cellWidth, cellHeight)
	if err != nil || !acknowledged {
		delete(pf.lastPTYGeometry, pty)
		if err != nil {
			vtui.DebugLog("PTY: resize to %dx%d failed: %v", columns, rows, err)
		}
		return
	}
	if pf.lastPTYGeometry == nil {
		pf.lastPTYGeometry = make(map[PtyBackend]ptyGeometry)
	}
	pf.lastPTYGeometry[pty] = geometry
	navigationBenchmarkUIEvent("terminal.geometry.negotiated", "columns", columns, "rows", rows)
}
