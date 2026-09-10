package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/paneltest"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestHostConsole_LeaveReleasesStaleMouseCapture_Issue856(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	oldNavigationMode := config.App.NavigationMode
	config.App.NavigationMode = config.NavigationClassic
	t.Cleanup(func() { config.App.NavigationMode = oldNavigationMode })

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	pf := paneltest.SetupMockPanelsFrame(t)
	t.Cleanup(pf.Close)
	pf.ShellMode = terminal.ShellModeHost
	pf.ShowPanels = false
	pf.ResizeConsole(80, 25)
	pf.TermView.MouseTrackingMode = 1003
	pf.HostConsoleActive = true
	vtui.FrameManager.Push(pf)

	// A console application with mouse tracking can leave f4 with a button
	// down but no release. The frame manager then captures the hidden panels
	// frame and routes subsequent UI clicks back to it.
	vtui.FrameManager.InjectEvents([]*vtinput.InputEvent{{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      10,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}})
	if !vtui.FrameManager.Step(0) {
		t.Fatal("mouse-down event stopped the frame manager")
	}
	vtui.FrameManager.SyncCurrentScreen()
	if got := vtui.FrameManager.Screens[0].CapturedFrame; got != pf {
		t.Fatalf("mouse-down did not capture PanelsFrame: got %T", got)
	}

	// endExecution makes the panels visible before leaving the host console.
	pf.ShowPanels = true
	pf.LeaveHostConsole()

	// The queued neutral release must be processed before the next real click.
	if !vtui.FrameManager.Step(0) {
		t.Fatal("queued mouse release stopped the frame manager")
	}
	vtui.FrameManager.SyncCurrentScreen()
	if got := vtui.FrameManager.Screens[0].CapturedFrame; got != nil {
		t.Fatalf("stale mouse capture survived host-console return: %T", got)
	}
}
