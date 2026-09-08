package main

import (
	"testing"

	"github.com/unxed/vtinput"
)

// withMacKeys puts the process into a known Mac-keyboard state for one test:
// the layout forced on, and a backend that separates Command from Control.
func withMacKeys(t *testing.T, commandIsDistinct bool) {
	t.Helper()

	// applyMacKeys deliberately respects the same handover guard as the live
	// key path. Give every test a neutral manager so a shuffled predecessor
	// cannot leave a hidden PanelsFrame that suppresses the test's mapping.
	t.Cleanup(swapFrameManager(t))

	previousMode := AppConfig.MacKeyboard
	AppConfig.MacKeyboard = MacKeysOn
	t.Cleanup(func() { AppConfig.MacKeyboard = previousMode })

	// The real probe asks for darwin and for a backend that folds Command
	// into the left Ctrl channel, neither of which the test binary has.
	previousProbe := macCommandIsDistinct
	macCommandIsDistinct = func() bool { return commandIsDistinct }
	t.Cleanup(func() { macCommandIsDistinct = previousProbe })
}

// The probe still has to name a backend that exists, or the mode would be
// unreachable on the machine it was written for.
func TestMacCommandBackendsNamesTheMacOSDefault(t *testing.T) {
	if !macCommandBackends["qt"] {
		t.Error("Qt must preserve Command and physical Control for native dialog and editor keys")
	}
	if !macCommandBackends["gogpu"] {
		t.Error("the macOS GUI backend is not listed as separating Command from Control")
	}
}

func macKeyEvent(mods vtinput.ControlKeyState, vk uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		ControlKeyState: mods | vtinput.EnhancedKey,
	}
}

func TestParseMacKeysMode(t *testing.T) {
	for value, want := range map[string]string{
		"":         MacKeysAuto,
		"auto":     MacKeysAuto,
		" AUTO ":   MacKeysAuto,
		"nonsense": MacKeysAuto,
		"on":       MacKeysOn,
		"1":        MacKeysOn,
		"On":       MacKeysOn,
		"off":      MacKeysOff,
		"0":        MacKeysOff,
	} {
		if got := ParseMacKeysMode(value); got != want {
			t.Errorf("ParseMacKeysMode(%q) = %q, want %q", value, got, want)
		}
	}
}

// The Command chords are what the issue asks for: the edges of the line and
// of the document, not a word jump.
func TestMacKeysRewritesCommandChords(t *testing.T) {
	withMacKeys(t, true)

	e := macKeyEvent(vtinput.LeftCtrlPressed, vtinput.VK_LEFT)
	if !applyMacKeys("Editor", e) {
		t.Fatal("Cmd+Left was not rewritten")
	}
	if got := EventToHotkeyString(e); got != "Home" {
		t.Errorf("Cmd+Left = %q, want %q", got, "Home")
	}

	e = macKeyEvent(vtinput.LeftCtrlPressed|vtinput.ShiftPressed, vtinput.VK_UP)
	if !applyMacKeys("Editor", e) {
		t.Fatal("Cmd+Shift+Up was not rewritten")
	}
	if got := EventToHotkeyString(e); got != "CtrlShiftHome" {
		t.Errorf("Cmd+Shift+Up = %q, want %q", got, "CtrlShiftHome")
	}
}

// The whole point of reading Command off the left Ctrl channel is that the
// physical Control key, which arrives on the right one, keeps the Far layout.
func TestMacKeysLeavesPhysicalControlAlone(t *testing.T) {
	withMacKeys(t, true)

	e := macKeyEvent(vtinput.RightCtrlPressed, vtinput.VK_LEFT)
	if applyMacKeys("Editor", e) {
		t.Fatalf("Ctrl+Left was rewritten to %q", EventToHotkeyString(e))
	}
	if got := EventToHotkeyString(e); got != "RCtrlLeft" {
		t.Errorf("Ctrl+Left = %q, want %q", got, "RCtrlLeft")
	}
}

// Where the backend cannot tell Command from Control — a terminal on macOS —
// a left-Ctrl chord is the physical Control key and must not be touched.
func TestMacKeysSkipsCommandRulesWithoutTheChannelSplit(t *testing.T) {
	withMacKeys(t, false)

	e := macKeyEvent(vtinput.LeftCtrlPressed, vtinput.VK_LEFT)
	if applyMacKeys("Editor", e) {
		t.Fatalf("Ctrl+Left was rewritten to %q", EventToHotkeyString(e))
	}

	// Option needs no such split: it reaches f4 as Alt on every backend.
	e = macKeyEvent(vtinput.LeftAltPressed, vtinput.VK_LEFT)
	if !applyMacKeys("Editor", e) {
		t.Fatal("Opt+Left was not rewritten")
	}
	if got := EventToHotkeyString(e); got != "CtrlLeft" {
		t.Errorf("Opt+Left = %q, want %q", got, "CtrlLeft")
	}
}

func TestMacKeysRewritesOptionWordNavigation(t *testing.T) {
	withMacKeys(t, true)

	e := macKeyEvent(vtinput.LeftAltPressed|vtinput.ShiftPressed, vtinput.VK_RIGHT)
	if !applyMacKeys("Editor", e) {
		t.Fatal("Opt+Shift+Right was not rewritten")
	}
	if got := EventToHotkeyString(e); got != "CtrlShiftRight" {
		t.Errorf("Opt+Shift+Right = %q, want %q", got, "CtrlShiftRight")
	}
}

// The panels keep the Far layout: Alt+Left walks the folder history there and
// Ctrl+Left moves the split, and neither may be taken over.
func TestMacKeysLeavesThePanelsAlone(t *testing.T) {
	withMacKeys(t, true)

	for _, area := range []string{"Shell", "Terminal", "Menu", "Viewer"} {
		e := macKeyEvent(vtinput.LeftAltPressed, vtinput.VK_LEFT)
		if applyMacKeys(area, e) {
			t.Errorf("Opt+Left was rewritten in area %s", area)
		}
		e = macKeyEvent(vtinput.LeftCtrlPressed, vtinput.VK_LEFT)
		if applyMacKeys(area, e) {
			t.Errorf("Cmd+Left was rewritten in area %s", area)
		}
	}
}

func TestMacKeysOffLeavesEverythingAlone(t *testing.T) {
	withMacKeys(t, true)
	AppConfig.MacKeyboard = MacKeysOff

	e := macKeyEvent(vtinput.LeftCtrlPressed, vtinput.VK_LEFT)
	if applyMacKeys("Editor", e) {
		t.Error("Cmd+Left was rewritten with the mode off")
	}
	e = macKeyEvent(vtinput.LeftAltPressed, vtinput.VK_LEFT)
	if applyMacKeys("Editor", e) {
		t.Error("Opt+Left was rewritten with the mode off")
	}
}

// A modifier pressed on its own is not a chord.
func TestMacKeysIgnoresBareModifiers(t *testing.T) {
	withMacKeys(t, true)

	e := macKeyEvent(vtinput.LeftCtrlPressed, vtinput.VK_LCONTROL)
	if applyMacKeys("Editor", e) {
		t.Error("a bare Command press was rewritten")
	}
}
