package main

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestHotkeys_ShellActionsBoundInTerminalArea guards the fix for issue #354.
// With panels hidden (area="Terminal") the file-manager F-keys used to
// fall through, so F2/F7/Shift+F4/Shift+F9/F10/Alt+F1/Alt+F2/Ctrl+P did
// nothing. They now carry DefaultAreas: []string{"Terminal"} with a
// NoAltScreenApp condition so they fire in the panels-hidden idle
// terminal but stay out of the way of full-screen apps.
func TestHotkeys_ShellActionsBoundInTerminalArea_Issue354(t *testing.T) {
	hm := NewHotkeyManager("")

	cases := []struct {
		key    string
		action string
	}{
		{"F2", "Panel.UserMenu"},
		{"F7", "File.MakeDir"},
		{"F10", "App.Quit"},
		{"ShiftF4", "File.New"},
		{"ShiftF9", "App.SaveSettings"},
		{"AltF1", "Panel.LeftDriveMenu"},
		{"AltF2", "Panel.RightDriveMenu"},
		{"CtrlP", "Panel.TogglePassivePanel"},
	}
	for _, tc := range cases {
		got, ok := hm.Bindings["Terminal"][tc.key]
		if !ok {
			t.Errorf("Terminal area missing binding for %s (expected %s:NoAltScreenApp)", tc.key, tc.action)
			continue
		}
		want := tc.action + ":NoAltScreenApp"
		if got != want {
			t.Errorf("Terminal/%s = %q, want %q", tc.key, got, want)
		}
	}
}

// TestHotkeys_ShellActions_TerminalArea_GatedByAltScreen ensures the
// Terminal-area bindings do NOT fire when a full-screen application
// (mc, htop, vim, less) is active — those keys belong to the app.
// The gate is NoAltScreenApp, which returns true when panels are
// shown OR no AltScreen mode is engaged.
func TestHotkeys_ShellActions_TerminalArea_GatedByAltScreen_Issue354(t *testing.T) {
	// Reset the global frame manager and register a hidden-panels PanelsFrame
	// with an AltScreen app active — that's the state where the condition must fail.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.showPanels = false
	pf.termView.UseAltScreen = true
	vtui.FrameManager.Push(pf)

	if GlobalHotkeysMgr == nil {
		GlobalHotkeysMgr = NewHotkeyManager("")
	}

	// With AltScreen active the condition NoAltScreenApp is false, so
	// the Terminal-area binding must resolve to "" (fall-through to PTY).
	if got := GlobalHotkeysMgr.GetAction("Terminal", "F2"); got != "" {
		t.Errorf("Terminal F2 with AltScreen active: got %q, want empty (must fall through to app)", got)
	}
	if got := GlobalHotkeysMgr.GetAction("Terminal", "F10"); got != "" {
		t.Errorf("Terminal F10 with AltScreen active: got %q, want empty (must fall through to app)", got)
	}

	// Clear AltScreen — condition now passes and the actions surface.
	pf.termView.UseAltScreen = false
	if got := GlobalHotkeysMgr.GetAction("Terminal", "F2"); got != "Panel.UserMenu" {
		t.Errorf("Terminal F2 without AltScreen: got %q, want Panel.UserMenu", got)
	}
	if got := GlobalHotkeysMgr.GetAction("Terminal", "F10"); got != "App.Quit" {
		t.Errorf("Terminal F10 without AltScreen: got %q, want App.Quit", got)
	}
}

// TestPanelsFrame_F2_OpensUserMenu_WhenPanelsHidden is the end-to-end
// half of the issue #354 fix: with panels hidden, pressing F2 must
// push the user-menu frame onto the top of the stack.
func TestPanelsFrame_F2_OpensUserMenu_WhenPanelsHidden_Issue354(t *testing.T) {
	// The assertion below compares the top frame type before and after F2, so a
	// menu left on the shared manager by an earlier test makes the push
	// invisible. Start from a manager of our own.
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	previousHotkeys := GlobalHotkeysMgr
	previousMacros := MacroMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	MacroMgr = NewMacroManager("")
	t.Cleanup(func() {
		GlobalHotkeysMgr = previousHotkeys
		MacroMgr = previousMacros
	})
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.showPanels = false
	pf.termView.UseAltScreen = false

	before := topFrameType()

	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F2,
	})

	after := topFrameType()
	if after == before {
		t.Fatalf("F2 with hidden panels did not push a new top frame (still %v)", after)
	}
	if after != vtui.TypeMenu {
		t.Errorf("F2 with hidden panels pushed frame of type %v, want VMenu (Panel.UserMenu)", after)
	}
}

// TestPanelsFrame_CtrlL_RevealsHiddenPassivePanel_Issue354 exercises
// the second bug from issue #354: with the passive panel hidden
// (Ctrl+F1 or Ctrl+F2), Ctrl+L installed the InfoPanel into the
// invisible slot and looked like a no-op. It must now un-hide the
// slot so the info panel is actually visible.
func TestPanelsFrame_CtrlL_RevealsHiddenPassivePanel_Issue354(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// setupMockPanelsFrame gives activeIdx=1 (right). Hide the passive
	// left panel — the state a user reaches via Ctrl+F1 in real life.
	pf.showLeftPanel = false

	pressKey(pf, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_L,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if pf.altPanels[0] == nil {
		t.Fatal("Ctrl+L must install InfoPanel on the passive (left) slot")
	}
	if _, ok := pf.altPanels[0].(*InfoPanel); !ok {
		t.Errorf("expected *InfoPanel on left slot, got %T", pf.altPanels[0])
	}
	if !pf.showLeftPanel {
		t.Error("Ctrl+L must un-hide the passive slot — otherwise the info panel is invisible")
	}
	if pf.activeIdx != 1 {
		t.Errorf("Ctrl+L must not move active side; got activeIdx=%d, want 1", pf.activeIdx)
	}
}

// topFrameType returns the type of the top frame across all screens
// (or -1 if none) — small helper for the F2 assertion above.
func topFrameType() vtui.FrameType {
	if vtui.FrameManager == nil {
		return -1
	}
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		return -1
	}
	return top.GetType()
}
