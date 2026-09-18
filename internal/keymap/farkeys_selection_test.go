package keymap

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestSelectionKeyNames(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want string
	}{
		{key: "Ctrl=", want: "CtrlVK_BB"},
		{key: "Ctrl-", want: "CtrlVK_BD"},
		{key: "CtrlShift+", want: "CtrlShiftVK_BB"},
		{key: "CtrlShift_", want: "CtrlShiftVK_BD"},
		{key: "Alt=", want: "AltVK_BB"},
		{key: "CtrlAdd", want: "CtrlAdd"},
		{key: "CtrlSubtract", want: "CtrlSubtract"},
		{key: "Num5", want: "Num5"},
	} {
		e := ParseFarKey(tc.key)
		if got := EventToHotkeyString(e); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
		e.Char = 0
		if got := EventToHotkeyString(e); got != tc.want {
			t.Errorf("%s without Char = %q, want %q", tc.key, got, tc.want)
		}
	}
	if got := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_CLEAR}); got != "Num5" {
		t.Errorf("Num Lock off: %q", got)
	}
	ctrlClear := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_CLEAR, ControlKeyState: vtinput.LeftCtrlPressed}
	if got := EventToFarString(ctrlClear); got != "CtrlVK_C" {
		t.Errorf("Ctrl+Clear layout shortcut changed to %q", got)
	}
	for _, tc := range []struct {
		sequence string
		want     string
	}{
		{sequence: "\x1b[61;5u", want: "CtrlVK_BB"},
		{sequence: "\x1b[45;5u", want: "CtrlVK_BD"},
	} {
		e, _, err := vtinput.ParseKitty([]byte(tc.sequence))
		if err != nil {
			t.Fatal(err)
		}
		if got := EventToHotkeyString(e); got != tc.want {
			t.Errorf("kitty %q = %q, want %q", tc.sequence, got, tc.want)
		}
	}
}
