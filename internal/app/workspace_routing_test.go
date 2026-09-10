package app

import (
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
	"testing"
)

// screenIndexOfFrame reports which screen holds f, or -1.
func screenIndexOfFrame(f vtui.Frame) int {
	for i, s := range vtui.FrameManager.Screens {
		if s == nil {
			continue
		}
		for _, fr := range s.Frames {
			if fr == f {
				return i
			}
		}
	}
	return -1
}

func waitForWorkspacePanels(t *testing.T, frames ...*panel.PanelsFrame) {
	t.Helper()
	for _, frame := range frames {
		paneltest.WaitForLoad(t, frame.Panels[0].(*panel.FileSystemPanel))
		paneltest.WaitForLoad(t, frame.Panels[1].(*panel.FileSystemPanel))
	}
}

// Issue #424: with two workspaces open, actions and hotkey conditions must
// read the workspace the user is looking at, not the oldest one.
func TestFindPanelsFrameAnyScreen_PrefersActiveWorkspace(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := panel.NewPanelsFrame()
	defer first.Close()
	vtui.FrameManager.Push(first)

	second := panel.NewPanelsFrame()
	defer second.Close()
	vtui.FrameManager.AddScreen(second)
	waitForWorkspacePanels(t, first, second)

	if got := panel.FindPanelsFrameAnyScreen(); got != second {
		t.Fatalf("after Ctrl+N the new workspace is active, got %v", got == first)
	}

	firstIdx := screenIndexOfFrame(first)
	if firstIdx < 0 {
		t.Fatalf("the first workspace left the screen list")
	}
	vtui.FrameManager.SwitchScreen(firstIdx)

	if got := panel.FindPanelsFrameAnyScreen(); got != first {
		t.Fatalf("after switching back the first workspace is active, got the other one")
	}
}

// The hotkey conditions read state off the frame this function returns, so
// they follow the active workspace too.
func TestConditionsFollowActiveWorkspace(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := panel.NewPanelsFrame()
	defer first.Close()
	vtui.FrameManager.Push(first)

	second := panel.NewPanelsFrame()
	defer second.Close()
	vtui.FrameManager.AddScreen(second)
	waitForWorkspacePanels(t, first, second)

	second.CmdLine.Edit.SetText("ls -la")

	emptyCmdLine, ok := keymap.LookupCondition("emptycommandline")
	if !ok {
		t.Fatalf("emptycommandline condition is not registered")
	}
	if emptyCmdLine() {
		t.Errorf("the active workspace has a command typed, condition must be false")
	}

	firstIdx := screenIndexOfFrame(first)
	if firstIdx < 0 {
		t.Fatalf("the first workspace left the screen list")
	}
	vtui.FrameManager.SwitchScreen(firstIdx)

	if !emptyCmdLine() {
		t.Errorf("the first workspace has an empty command line, condition must be true")
	}
}

// A headless screen (arkanoid, a modal dialog) has no panel.PanelsFrame of its own.
// The search must then fall back to the workspace used most recently, which
// is what keeps Ctrl+[ and friends working from a full-screen editor.
func TestFindPanelsFrameAnyScreen_FallsBackToMostRecent(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := panel.NewPanelsFrame()
	defer first.Close()
	vtui.FrameManager.Push(first)

	second := panel.NewPanelsFrame()
	defer second.Close()
	vtui.FrameManager.AddScreen(second)
	waitForWorkspacePanels(t, first, second)

	firstIdx := screenIndexOfFrame(first)
	if firstIdx < 0 {
		t.Fatalf("the first workspace left the screen list")
	}
	vtui.FrameManager.SwitchScreen(firstIdx)

	arkanoid := NewArkanoidFrame()
	t.Cleanup(arkanoid.Close)
	vtui.FrameManager.AddScreenHeadless(arkanoid)

	if got := panel.FindPanelsFrameAnyScreen(); got != first {
		t.Fatalf("the fallback must reach the workspace used last, not the oldest one")
	}
}

// A frame already closed is not somewhere the user can be working.
func TestFindPanelsFrameAnyScreen_SkipsClosedFrame(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := panel.NewPanelsFrame()
	defer first.Close()
	vtui.FrameManager.Push(first)

	second := panel.NewPanelsFrame()
	defer second.Close()
	vtui.FrameManager.AddScreen(second)
	waitForWorkspacePanels(t, first, second)

	second.Closed = true

	if got := panel.FindPanelsFrameAnyScreen(); got != first {
		t.Fatalf("a closed frame on the active screen must be skipped")
	}
}
