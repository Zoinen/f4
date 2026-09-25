package keymap

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestNavigationToggleTextOnlyHotkey(t *testing.T) {
	for _, char := range []rune{'`', '~', 'ё', 'Ё'} {
		for _, key := range []string{"CtrlVK_C0", "RCtrlVK_C0", "CtrlShiftVK_C0", "RCtrlShiftVK_C0"} {
			event := ParseFarKey(key)
			event.VirtualKeyCode, event.Char = 0, char
			if got := EventToHotkeyString(event); got != key {
				t.Errorf("text-only %q %s = %s", char, key, got)
			}
		}
	}
	// Ordinary Cyrillic typing and known virtual keys retain their identity.
	for _, event := range []*vtinput.InputEvent{
		{Char: 'ё'},
		{Char: 'ё', VirtualKeyCode: vtinput.VK_E, ControlKeyState: vtinput.LeftCtrlPressed},
	} {
		if EventToHotkeyString(event) != EventToFarString(event) {
			t.Fatalf("unrelated key changed: %+v", event)
		}
	}
}
