package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func newFastFindPanelsFrame(t *testing.T) (*PanelsFrame, *FileSystemPanel) {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	pf.ResizeConsole(100, 30)
	vtui.FrameManager.Push(pf)
	fsp := pf.getActivePanel()
	if fsp == nil {
		t.Fatal("active file panel is missing")
	}
	fsp.fastFindMode = true
	fsp.fastFindStr = "needle"
	t.Cleanup(func() { pf.Close() })
	return pf, fsp
}

func TestFastFindClosesWhenOverlayTakesFocus(t *testing.T) {
	_, fsp := newFastFindPanelsFrame(t)
	vtui.FrameManager.Push(vtui.NewVMenu(" Menu "))
	if fsp.fastFindMode || fsp.fastFindStr != "" {
		t.Fatalf("Fast Find survived overlay focus: mode=%v query=%q", fsp.fastFindMode, fsp.fastFindStr)
	}
}

func TestFastFindClosesBeforeInPlacePanelAction(t *testing.T) {
	_, fsp := newFastFindPanelsFrame(t)
	if !RunAction("Panel.QuickView") {
		t.Fatal("Panel.QuickView action was not handled")
	}
	if fsp.fastFindMode || fsp.fastFindStr != "" {
		t.Fatalf("Fast Find survived in-place panel action: mode=%v query=%q", fsp.fastFindMode, fsp.fastFindStr)
	}
}
