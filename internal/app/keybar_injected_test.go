package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestKeyBarClick_DispatchesHotkeyAction guards the regression introduced by
// the KeyBind refactor (commit 5b91218): mouse clicks on the F-key bar reach
// panel.PanelsFrame via FrameManager.InjectEvents, which sets is_injected=true and
// therefore skips FrameManager.EventFilter — the macro.MacroMgr.Filter path where
// configured hotkey actions (F3=View, F4=Edit, F5=Copy, …) are dispatched.
// Before the fix, the injected VK_F5 fell through panel.PanelsFrame.ProcessKey
// unhandled, so clicking F5 in the bottom bar did nothing.
func TestKeyBarClick_DispatchesHotkeyAction(t *testing.T) {
	preserveActionRegistry(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	if keymap.GlobalHotkeysMgr == nil {
		keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	}
	if macro.MacroMgr == nil {
		macro.MacroMgr = macro.NewMacroManager("")
	}

	// Register a probe action bound to F5 in the Shell area, overriding
	// whatever the registry ships with. This isolates the test from the
	// actual File.Copy handler (which would try to open dialogs).
	called := false
	action.RegisterAction(action.Action{
		Name: "Test.KeyBarF5Probe",
		Area: "Shell",
		Handler: func() bool {
			called = true
			return true
		},
	})
	prev := keymap.GlobalHotkeysMgr.GetAction("Shell", "F5")
	keymap.GlobalHotkeysMgr.Bind("Shell", "F5", "Test.KeyBarF5Probe")
	defer func() {
		if prev == "" {
			keymap.GlobalHotkeysMgr.Unbind("Shell", "F5")
		} else {
			keymap.GlobalHotkeysMgr.Bind("Shell", "F5", prev)
		}
	}()

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	// Simulate what vtui's KeyBar.ProcessMouse synthesizes on an F5 click:
	// a KeyEvent that reaches ProcessKey directly (via InjectEvents),
	// bypassing macro.MacroMgr.Filter.
	ev := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F5,
	}
	if !pf.ProcessKey(ev) {
		t.Fatal("PanelsFrame.ProcessKey returned false for injected F5; the KeyBar click would silently disappear")
	}
	if !called {
		t.Fatal("Injected F5 did not invoke the bound Shell action; KeyBar mouse click still broken")
	}
}
