package app

import (
	"context"
	"testing"
	"time"

	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// callMacroHost runs a host method while the test drives the silent UI task
// queue. This mirrors the real UI goroutine without starting a renderer.
func callMacroHost[T any](t *testing.T, call func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- call() }()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case result := <-done:
			return result
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timer.C:
			t.Fatal("timed out waiting for macro host UI task")
		}
	}
}

type macroHostScreenSize struct {
	width, height int
}

func callMacroHostScreenSize(t *testing.T, host F4MacroHost) macroHostScreenSize {
	return callMacroHost(t, func() macroHostScreenSize {
		w, h := host.ScreenSize()
		return macroHostScreenSize{width: w, height: h}
	})
}

func runMacroHostTask(t *testing.T) {
	t.Helper()
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queued macro host task")
	}
}

func TestF4MacroHostNilFrameManager(t *testing.T) {
	oldFrameManager := vtui.FrameManager
	vtui.FrameManager = nil
	defer func() { vtui.FrameManager = oldFrameManager }()

	host := F4MacroHost{}
	if got := host.CurrentArea(); got != "" {
		t.Fatalf("CurrentArea with no frame manager = %q, want empty", got)
	}
	if info := host.Panel(true); info.Visible || info.Path != "" || info.ItemCount != 0 {
		t.Fatalf("Panel with no frame manager = %+v, want zero info", info)
	}
	if got := host.CommandLine(); got != "" {
		t.Fatalf("CommandLine with no frame manager = %q, want empty", got)
	}
	if width, height := host.ScreenSize(); width != 0 || height != 0 {
		t.Fatalf("ScreenSize with no frame manager = %dx%d, want 0x0", width, height)
	}
	if got := host.WindowTitle(); got != "" {
		t.Fatalf("WindowTitle with no frame manager = %q, want empty", got)
	}
	if host.RunAction("missing") {
		t.Fatal("RunAction with no frame manager unexpectedly succeeded")
	}

	// These are no-ops when the UI is unavailable, including the empty-key path.
	host.Message("title", "text")
	host.InjectKeys(nil)
	host.InjectKeys([]*vtinput.InputEvent{{}})
	host.Log("macro host nil-manager test")

	oldMacroManager := macro.MacroMgr
	macro.MacroMgr = nil
	if actionReloadLuaMacros() {
		t.Fatal("actionReloadLuaMacros with no macro manager unexpectedly succeeded")
	}
	macro.MacroMgr = oldMacroManager
}

func TestF4MacroHostUIState(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	host := F4MacroHost{}

	oldMacroManager := macro.MacroMgr
	macro.MacroMgr = nil
	if got := callMacroHost(t, host.CurrentArea); got != "Common" {
		t.Fatalf("CurrentArea without macro manager = %q, want Common", got)
	}
	macro.MacroMgr = &macro.MacroManager{}
	if got := callMacroHost(t, host.CurrentArea); got != "Common" {
		t.Fatalf("CurrentArea with macro manager = %q, want Common", got)
	}
	macro.MacroMgr = oldMacroManager

	if info := callMacroHost(t, func() macro.MacroPanelInfo { return host.Panel(true) }); info.Visible {
		t.Fatalf("Panel without panels frame = %+v, want invisible", info)
	}
	if got := callMacroHost(t, host.CommandLine); got != "" {
		t.Fatalf("CommandLine without panels frame = %q, want empty", got)
	}
	size := callMacroHostScreenSize(t, host)
	if size.width != 0 || size.height != 0 {
		t.Fatalf("ScreenSize without panels frame = %dx%d, want 0x0", size.width, size.height)
	}

	pf := paneltest.SetupMockPanelsFrame(t)
	vtui.FrameManager.Push(pf)
	pf.LastW, pf.LastH = 80, 25
	t.Cleanup(func() { pf.Close() })

	active := callMacroHost(t, func() macro.MacroPanelInfo { return host.Panel(true) })
	if !active.Visible || active.ItemCount < 0 || active.CurPos < 0 {
		t.Fatalf("active panel info = %+v, want visible panel state", active)
	}
	other := callMacroHost(t, func() macro.MacroPanelInfo { return host.Panel(false) })
	if !other.Visible || active.Left == other.Left {
		t.Fatalf("active=%+v other=%+v, want opposite panel sides", active, other)
	}
	if got := callMacroHost(t, host.CommandLine); got != pf.CmdLine.Edit.GetText() {
		t.Fatalf("CommandLine = %q, want %q", got, pf.CmdLine.Edit.GetText())
	}
	size = callMacroHostScreenSize(t, host)
	if size.width != 80 || size.height != 25 {
		t.Fatalf("ScreenSize = %dx%d, want 80x25", size.width, size.height)
	}
	_ = callMacroHost(t, host.WindowTitle)
	if callMacroHost(t, func() bool { return host.RunAction("missing") }) {
		t.Fatal("RunAction for missing action unexpectedly succeeded")
	}

	host.Message("title", "text")
	runMacroHostTask(t)
	host.InjectKeys([]*vtinput.InputEvent{{Type: vtinput.KeyEventType, KeyDown: true}})
	runMacroHostTask(t)
	host.Log("macro host UI-state test")

	err := callMacroHost(t, func() error { _, err := host.CallPlugin(context.Background(), "missing-plugin", nil); return err })
	if err == nil {
		t.Fatal("CallPlugin for missing plugin returned nil error")
	}
}
