package panel

import (
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func (pf *PanelsFrame) pendingDocumentOpenOwnsKey(event *vtinput.InputEvent) bool {
	return HasPendingDocumentOpen(pf) && event != nil &&
		event.Type == vtinput.KeyEventType && event.KeyDown && event.VirtualKeyCode == vtinput.VK_ESCAPE &&
		event.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed|vtinput.ShiftPressed) == 0
}

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

func (pf *PanelsFrame) negotiatePTYGeometry(pty terminal.PtyBackend, columns, rows, cellWidth, cellHeight int) {
	if pty == nil {
		return
	}
	if _, pixels := pty.(terminal.PtyPixelSizer); !pixels {
		cellWidth, cellHeight = 0, 0
	}
	geometry := PtyGeometry{columns, rows, cellWidth, cellHeight}
	if previous, known := pf.lastPTYGeometry[pty]; known && previous == geometry {
		return
	}
	acknowledged, err := terminal.SetPtySize(pty, columns, rows, cellWidth, cellHeight)
	if err != nil || !acknowledged {
		delete(pf.lastPTYGeometry, pty)
		if err != nil {
			vtui.DebugLog("PTY: resize to %dx%d failed: %v", columns, rows, err)
		}
		return
	}
	if pf.lastPTYGeometry == nil {
		pf.lastPTYGeometry = make(map[terminal.PtyBackend]PtyGeometry)
	}
	pf.lastPTYGeometry[pty] = geometry
	navtrace.NavigationBenchmarkUIEvent("terminal.geometry.negotiated", "columns", columns, "rows", rows)
}
