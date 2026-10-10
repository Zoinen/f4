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

func TestRussianLayoutShortcutUsesPhysicalLatinKey(t *testing.T) {
	for _, tc := range []struct {
		char rune
		want uint16
		key  string
	}{
		{char: 'т', want: vtinput.VK_N, key: "CtrlN"},
		{char: 'Т', want: vtinput.VK_N, key: "CtrlN"},
		{char: 'ц', want: vtinput.VK_W, key: "CtrlW"},
		{char: 'ф', want: vtinput.VK_A, key: "CtrlA"},
	} {
		e := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			Char:            tc.char,
			ControlKeyState: vtinput.LeftCtrlPressed,
		}
		if got := EventToHotkeyString(e); got != tc.key {
			t.Errorf("%q = %q, want %q", string(tc.char), got, tc.key)
		}
		if !NormalizeLayoutShortcut(e) || e.VirtualKeyCode != tc.want {
			t.Errorf("NormalizeLayoutShortcut(%q) = VK %d, want VK %d", string(tc.char), e.VirtualKeyCode, tc.want)
		}
	}
}

func TestRussianTextWithoutShortcutModifierStaysUnicode(t *testing.T) {
	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'т'}
	if NormalizeLayoutShortcut(e) {
		t.Fatal("unmodified Cyrillic text must not become a Latin virtual key")
	}
	if got := EventToHotkeyString(e); got != "Т" {
		t.Fatalf("unmodified Cyrillic text = %q, want Т", got)
	}
}
