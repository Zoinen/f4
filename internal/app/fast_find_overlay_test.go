package app

import (
	"github.com/unxed/f4/internal/panel"
	"testing"

	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func newFastFindPanelsFrame(t *testing.T) (*panel.PanelsFrame, *panel.FileSystemPanel) {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	pf := panel.NewPanelsFrame()
	pf.ResizeConsole(100, 30)
	vtui.FrameManager.Push(pf)
	fsp := pf.GetActivePanel()
	if fsp == nil {
		t.Fatal("active file panel is missing")
	}
	fsp.FastFindMode = true
	fsp.FastFindStr = "needle"
	t.Cleanup(func() { pf.Close() })
	return pf, fsp
}

func TestFastFindClosesWhenOverlayTakesFocus(t *testing.T) {
	_, fsp := newFastFindPanelsFrame(t)
	vtui.FrameManager.Push(vtui.NewVMenu(" Menu "))
	if fsp.FastFindMode || fsp.FastFindStr != "" {
		t.Fatalf("Fast Find survived overlay focus: mode=%v query=%q", fsp.FastFindMode, fsp.FastFindStr)
	}
}

func TestFastFindClosesBeforeInPlacePanelAction(t *testing.T) {
	_, fsp := newFastFindPanelsFrame(t)
	if !RunAction("Panel.QuickView") {
		t.Fatal("Panel.QuickView action was not handled")
	}
	if fsp.FastFindMode || fsp.FastFindStr != "" {
		t.Fatalf("Fast Find survived in-place panel action: mode=%v query=%q", fsp.FastFindMode, fsp.FastFindStr)
	}
}
