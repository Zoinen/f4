package main

import (
	"testing"

	"github.com/unxed/vtui"
)

// The Table Demo dialog must satisfy the same layout rules as framework
// dialogs: no overlaps, "air" between interactive elements, border padding.
func TestTableDemoDialog_Layout(t *testing.T) {
	vtui.SetDefaultPalette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dlg := buildTableDialog()
	vtui.AssertLayout(t, dlg)
}

func TestWorkspaceTabControlDemo(t *testing.T) {
	vtui.SetDefaultPalette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	demo := buildWorkspaceDemoWindow(80, 25, "Inspector", "◇", "Properties workspace")
	if got, want := demo.GetWorkspaceTabTitle(), "◇ Inspector"; got != want {
		t.Fatalf("workspace tab title = %q, want %q", got, want)
	}
	info := demo.GetWorkspaceMenuInfo()
	if info.Icon != "◇" || info.Primary != "◇ Inspector" || info.Secondary != "Properties workspace" {
		t.Fatalf("workspace menu info = %#v", info)
	}
}
