package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestPlugRingDialog_Layout(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()

	actionPlugRing(pf)

	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("Expected vtui.Container, got %T", top)
	}

	vtui.AssertLayout(t, dlg)

	// Clean up
	top.SetExitCode(-1)
	vtui.FrameManager.Pop()
}
