package panel

import (
	"testing"

	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/vtui"
)

func TestPanelsFrame_WorkspaceAndDriveMenuCommandsAreHandled(t *testing.T) {
	oldAppCommand := AppCommand
	oldWorkspaceClose := WorkspaceClose
	t.Cleanup(func() {
		AppCommand = oldAppCommand
		WorkspaceClose = oldWorkspaceClose
	})

	var appCommand int
	AppCommand = func(_ *PanelsFrame, cmd int, _ any) bool {
		appCommand = cmd
		return true
	}
	closed := false
	WorkspaceClose = func() bool {
		closed = true
		return true
	}

	oldManager := vtui.FrameManager
	t.Cleanup(func() { vtui.FrameManager = oldManager })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	if !pf.HandleCommand(appcmd.CmWorkspaceNew, nil) || appCommand != appcmd.CmWorkspaceNew {
		t.Fatalf("CmWorkspaceNew was not routed to AppCommand: handled=%v command=%d", appCommand == appcmd.CmWorkspaceNew, appCommand)
	}
	if !pf.HandleCommand(appcmd.CmWorkspaceClose, nil) || !closed {
		t.Fatal("CmWorkspaceClose did not invoke WorkspaceClose")
	}
	if !pf.HandleCommand(appcmd.CmLeftDriveMenu, nil) {
		t.Fatal("CmLeftDriveMenu was not handled")
	}
	if vtui.FrameManager.GetTopFrame() == pf {
		t.Fatal("CmLeftDriveMenu did not open a menu frame")
	}
}

func TestPanelsFrame_NativeWorkspaceForkStillReachesFrame(t *testing.T) {
	oldManager := vtui.FrameManager
	t.Cleanup(func() { vtui.FrameManager = oldManager })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	pf.ShowPanels = false
	vtui.FrameManager.Push(pf)

	if !vtui.FrameManager.EmitCommand(vtui.CmResize, "fork") {
		t.Fatal("native workspace fork command was not handled")
	}
	if len(vtui.FrameManager.Screens) != 2 {
		t.Fatalf("native workspace fork created %d screens, want 2", len(vtui.FrameManager.Screens))
	}
}
