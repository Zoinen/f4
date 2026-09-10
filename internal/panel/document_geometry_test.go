package panel

import (
	errors "errors"
	vtui "github.com/unxed/vtui"
	testing "testing"
)

type lifecyclePTY struct {
	mockPty
	sizes    [][2]int
	failNext bool
}

func (pty *lifecyclePTY) SetSize(columns, rows int) {
	pty.sizes = append(pty.sizes, [2]int{columns, rows})
}

func (pty *lifecyclePTY) SetSizeChecked(columns, rows int) error {
	pty.SetSize(columns, rows)
	if pty.failNext {
		pty.failNext = false
		return errors.New("temporary resize failure")
	}
	return nil
}

type lifecycleDocumentFrame struct{ vtui.BaseFrame }

func (*lifecycleDocumentFrame) GetType() vtui.FrameType { return vtui.TypeUser }

func TestDocumentTransitionsKeepHiddenPTYGeometry(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	vtui.FrameManager.ConfigureWorkspaceTabs(vtui.WorkspaceTabsMultiple, vtui.WorkspaceCtrlTabDirect)
	panels := NewPanelsFrame()
	defer panels.Close()
	pty := &lifecyclePTY{}
	panels.PtyMutex.Lock()
	panels.Pty = pty
	panels.PtyMutex.Unlock()
	panels.ResizeConsole(80, 25)
	vtui.FrameManager.Push(panels)
	initialWidth, initialHeight := panels.TermView.Width, panels.TermView.Height
	if len(pty.sizes) != 1 {
		t.Fatalf("initial geometry sends=%d", len(pty.sizes))
	}
	vtui.FrameManager.AddScreen(&lifecycleDocumentFrame{})
	if len(pty.sizes) != 1 || panels.TermView.Width != initialWidth || panels.TermView.Height != initialHeight {
		t.Fatalf("opening document resized hidden terminal: sends=%+v grid=%dx%d", pty.sizes, panels.TermView.Width, panels.TermView.Height)
	}
	vtui.FrameManager.CloseActiveScreen()
	if len(pty.sizes) != 1 {
		t.Fatalf("closing document renegotiated unchanged terminal: %+v", pty.sizes)
	}
	vtui.FrameManager.AddScreen(&lifecycleDocumentFrame{})
	screen.AllocBuf(104, 39)
	vtui.FrameManager.ResizeAllScreens()
	if len(pty.sizes) != 1 || !panels.terminalGeometryPending {
		t.Fatalf("hidden resize applied early: %+v pending=%v", pty.sizes, panels.terminalGeometryPending)
	}
	vtui.FrameManager.SwitchScreen(0)
	if len(pty.sizes) != 2 || panels.TermView.Width != 104 || panels.terminalGeometryPending {
		t.Fatalf("activation did not apply newest geometry once: %+v grid=%dx%d pending=%v", pty.sizes, panels.TermView.Width, panels.TermView.Height, panels.terminalGeometryPending)
	}
	last := pty.sizes[len(pty.sizes)-1]
	if last[0] != panels.TermView.Width || last[1] != panels.TermView.Height {
		t.Fatalf("child/grid mismatch: %+v grid=%dx%d", last, panels.TermView.Width, panels.TermView.Height)
	}
}

func TestPTYGeometryNegotiationTracksLiveChildrenWithoutRedundantResize(t *testing.T) {
	panels := &PanelsFrame{}
	first, second := &lifecyclePTY{}, &lifecyclePTY{}
	panels.negotiatePTYGeometry(first, 80, 24, 10, 20)
	panels.negotiatePTYGeometry(first, 80, 24, 12, 24) // A non-pixel backend sees no change.
	panels.negotiatePTYGeometry(second, 80, 24, 10, 20)
	panels.negotiatePTYGeometry(second, 80, 24, 10, 20)
	panels.negotiatePTYGeometry(first, 100, 30, 10, 20)
	if len(first.sizes) != 2 || len(second.sizes) != 1 {
		t.Fatalf("first=%+v second=%+v", first.sizes, second.sizes)
	}
}

func TestPTYGeometryFailedResizeIsRetried(t *testing.T) {
	panels := &PanelsFrame{}
	pty := &lifecyclePTY{failNext: true}
	panels.negotiatePTYGeometry(pty, 80, 24, 0, 0)
	if _, known := panels.lastPTYGeometry[pty]; known {
		t.Fatal("failed geometry was acknowledged")
	}
	panels.negotiatePTYGeometry(pty, 80, 24, 0, 0)
	panels.negotiatePTYGeometry(pty, 80, 24, 0, 0)
	if len(pty.sizes) != 2 {
		t.Fatalf("resize attempts=%d, want failed attempt plus successful retry", len(pty.sizes))
	}
}
