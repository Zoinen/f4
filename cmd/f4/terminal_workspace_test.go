package main

import (
	"testing"

	"github.com/unxed/vtui"
)

// Issue #128: the console should be able to live in a workspace of its own, so
// that Ctrl+Tab flips between files and terminal, instead of always taking the
// place of the panels it was opened from.
func TestPanelsFrame_NewTerminalWorkspaceKeepsPanelsWhereTheyAre(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.shellMode = ShellModeOwn
	fm.Push(pf)

	if !actionWorkspaceNewTerminal() {
		t.Fatal("Workspace.NewTerminal reported the request as unhandled")
	}
	if len(fm.Screens) != 2 {
		t.Fatalf("action created %d workspaces, want 2", len(fm.Screens))
	}
	clone, ok := fm.GetTopFrame().(*PanelsFrame)
	if !ok {
		t.Fatalf("new workspace top frame = %T, want *PanelsFrame", fm.GetTopFrame())
	}
	defer clone.Close()
	if clone == pf {
		t.Fatal("the action reused the original panels instead of forking them")
	}
	if clone.showPanels {
		t.Error("the new workspace did not switch to its console view")
	}
	if !pf.showPanels {
		t.Error("opening a terminal workspace must leave the original panels visible")
	}
	if got := clone.GetWorkspaceTabTitle(); got != "Terminal" {
		t.Errorf("workspace tab title = %q, want \"Terminal\"", got)
	}

	waitForLoad(t, pf.panels[0].(*FileSystemPanel))
	waitForLoad(t, pf.panels[1].(*FileSystemPanel))
	waitForLoad(t, clone.panels[0].(*FileSystemPanel))
	waitForLoad(t, clone.panels[1].(*FileSystemPanel))
}

// Where command output can only be captured into a dialog there is no console
// view to hand a workspace to. The action then reports the environment, as
// Ctrl+O does, rather than leaving a second identical copy of the panels open.
func TestPanelsFrame_NewTerminalWorkspaceSkippedWithoutConsoleView(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.shellMode = ShellModeSimpleCaptured
	fm.Push(pf)

	if !actionWorkspaceNewTerminal() {
		t.Fatal("Workspace.NewTerminal reported the request as unhandled")
	}
	if len(fm.Screens) != 1 {
		t.Fatalf("captured-output environment created %d workspaces, want 1", len(fm.Screens))
	}
	if !pf.showPanels {
		t.Error("panels must stay visible where no console view exists")
	}

	waitForLoad(t, pf.panels[0].(*FileSystemPanel))
	waitForLoad(t, pf.panels[1].(*FileSystemPanel))
}
