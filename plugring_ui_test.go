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

func TestPlugRingRowColorsFollowDialogTheme(t *testing.T) {
	vtui.SetDefaultPalette()
	oldHighlight := vtui.Palette[vtui.ColDialogHighlightText]
	oldText := vtui.Palette[vtui.ColDialogText]
	t.Cleanup(func() {
		vtui.Palette[vtui.ColDialogHighlightText] = oldHighlight
		vtui.Palette[vtui.ColDialogText] = oldText
	})

	vtui.Palette[vtui.ColDialogHighlightText] = vtui.SetRGBFore(oldHighlight, 0x123456)
	vtui.Palette[vtui.ColDialogText] = vtui.SetRGBFore(oldText, 0xABCDEF)
	selected := vtui.SetRGBBoth(0, 0xFFFFFF, 0x654321)

	header := plugRingRow{header: "Category"}.GetCellAttr(0, selected)
	if got := vtui.GetRGBFore(header); got != 0x123456 {
		t.Fatalf("header foreground = %#x, want theme foreground", got)
	}
	if got := vtui.GetRGBBack(header); got != 0x654321 {
		t.Fatalf("header background = %#x, want selected background", got)
	}

	installed := plugRingRow{status: "Installed"}.GetCellAttr(0, selected)
	if got := vtui.GetRGBFore(installed); got != 0xABCDEF {
		t.Fatalf("installed foreground = %#x, want dialog text foreground", got)
	}
}
