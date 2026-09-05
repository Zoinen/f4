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
func TestAutoLayoutDemoDialog_Layout(t *testing.T) {
	vtui.SetDefaultPalette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dlg := buildAutoLayoutDialog()
	vtui.AssertLayout(t, dlg)
}
func TestReactiveDemoDialog_Layout(t *testing.T) {
	vtui.SetDefaultPalette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dlg := buildReactiveDemoDialog()
	vtui.AssertLayout(t, dlg)
}

func TestDemoSeparatorGrowsWithWindow(t *testing.T) {
	dlg := vtui.NewWindow(0, 0, 79, 21, "demo")
	separator := vtui.NewSeparator(2, 13, 76, true, true)
	separator.SetGrowMode(vtui.GrowHiX)
	dlg.AddItem(separator)

	dlg.ChangeSize(100, 22)
	if x1, _, x2, _ := separator.GetPosition(); x1 != 2 || x2 != 97 {
		t.Fatalf("separator position after resize = (%d, %d), want (2, 97)", x1, x2)
	}
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
func TestTestApp_ConsoleRendererSelection(t *testing.T) {
	scrWinAPI := vtui.NewScreenBuf()
	scrWinAPI.Renderer = vtui.NewWin32ConsoleRenderer(scrWinAPI)
	if _, ok := scrWinAPI.Renderer.(*vtui.Win32ConsoleRenderer); !ok {
		t.Errorf("Expected Win32ConsoleRenderer, got %T", scrWinAPI.Renderer)
	}

	scrANSI := vtui.NewScreenBuf()
	if _, ok := scrANSI.Renderer.(*vtui.AnsiRenderer); !ok {
		t.Errorf("Expected AnsiRenderer by default, got %T", scrANSI.Renderer)
	}
}
