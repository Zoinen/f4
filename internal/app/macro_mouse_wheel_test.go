package app

import (
	"slices"
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func wheelEvent(direction int, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, WheelDirection: direction, ControlKeyState: mods,
		MouseEventFlags: vtinput.MouseWheeled}
}

func TestMouseWheelHasAFarName(t *testing.T) {
	for _, tc := range []struct {
		e    *vtinput.InputEvent
		want string
	}{
		{wheelEvent(1, 0), "MsWheelUp"},
		{wheelEvent(-1, 0), "MsWheelDown"},
		{wheelEvent(1, vtinput.LeftCtrlPressed), "CtrlMsWheelUp"},
		{wheelEvent(-1, vtinput.LeftAltPressed), "AltMsWheelDown"},
		{&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true}, ""},
		{&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F1}, ""},
	} {
		if got := keymap.MouseWheelToFarString(tc.e); got != tc.want {
			t.Errorf("MouseWheelToFarString = %q, want %q", got, tc.want)
		}
	}
}

// A macro bound to the wheel plays when the wheel turns, and the wheel is
// left alone otherwise, and while a macro is being recorded (f4#1870).
func TestMacroPlaysOnTheMouseWheel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	seq := []*vtinput.InputEvent{{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}}
	m := &macro.MacroManager{Macros: map[string]map[string][]*vtinput.InputEvent{"Common": {"CtrlMsWheelDown": seq}}}

	if !macroFilter(m, wheelEvent(-1, vtinput.LeftCtrlPressed)) {
		t.Fatal("Ctrl+wheel down did not play the macro bound to CtrlMsWheelDown")
	}
	if macroFilter(m, wheelEvent(-1, 0)) {
		t.Fatal("a bare wheel turn was taken although no macro is bound to MsWheelDown")
	}
	m.Recording = true
	if macroFilter(m, wheelEvent(-1, vtinput.LeftCtrlPressed)) {
		t.Fatal("the wheel played a macro while one was being recorded")
	}
}

func TestMacroFreeKeysOfferTheMouseWheel(t *testing.T) {
	m := &macro.MacroManager{Macros: map[string]map[string][]*vtinput.InputEvent{"Shell": {"MsWheelUp": nil}}}
	free := m.FreeKeys("Shell", nil)
	if !slices.Contains(free, "MsWheelDown") || !slices.Contains(free, "CtrlMsWheelUp") || slices.Contains(free, "MsWheelUp") {
		t.Fatalf("free keys %v: want the wheel offered, but not MsWheelUp, which a macro uses", free)
	}
}
