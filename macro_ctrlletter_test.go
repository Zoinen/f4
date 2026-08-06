package main

import (
	"testing"

	"github.com/unxed/vtinput"
)

// TestEventToFarString_CtrlLetter_LowercaseChar guards against a regression
// where the Wayland/X11 gui backends deliver Ctrl+letter events with Char
// set to the lowercase typed letter. EventToFarString must ignore that and
// produce the canonical uppercase form used by every default binding —
// otherwise Ctrl+V paste, Ctrl+A select-all, Ctrl+O toggle-panels, and
// every other Ctrl+letter shortcut silently fail on gui-linux.
func TestEventToFarString_CtrlLetter_LowercaseChar(t *testing.T) {
	cases := []struct {
		name string
		vk   uint16
		char rune
		mods vtinput.ControlKeyState
		want string
	}{
		{"Ctrl+V (Wayland, lowercase Char)", vtinput.VK_V, 'v', vtinput.LeftCtrlPressed, "CtrlV"},
		{"Ctrl+A (Wayland, lowercase Char)", vtinput.VK_A, 'a', vtinput.LeftCtrlPressed, "CtrlA"},
		{"Ctrl+O (Wayland, lowercase Char)", vtinput.VK_O, 'o', vtinput.LeftCtrlPressed, "CtrlO"},
		{"Alt+F (Wayland, lowercase Char)", vtinput.VK_F, 'f', vtinput.LeftAltPressed, "AltF"},
		{"Ctrl+Shift+Z (uppercase Char)", vtinput.VK_Z, 'Z', vtinput.LeftCtrlPressed | vtinput.ShiftPressed, "CtrlShiftZ"},
		{"Ctrl+V (tty, no Char)", vtinput.VK_V, 0, vtinput.LeftCtrlPressed, "CtrlV"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				VirtualKeyCode:  tc.vk,
				Char:            tc.char,
				ControlKeyState: tc.mods,
			}
			if got := EventToFarString(e); got != tc.want {
				t.Errorf("EventToFarString = %q, want %q", got, tc.want)
			}
		})
	}
}
