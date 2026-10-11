package keymap

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestTranslateKeyToKittyNonameWithoutCharacterIsEmptyCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{VirtualKeyCode: 0xFC, KeyDown: true}
	if got := TranslateKeyToKitty(e, 8, false); got != "" {
		t.Fatalf("noname without character = %q, want empty", got)
	}
}

func TestTranslateKeyToKittyPlainReportedTabAndBackspaceReleaseAreEmptyCoverageBatch39(t *testing.T) {
	for _, vk := range []uint16{vtinput.VK_TAB, vtinput.VK_BACK} {
		e := &vtinput.InputEvent{VirtualKeyCode: vk, KeyDown: false}
		if got := TranslateKeyToKitty(e, 2, false); got != "" {
			t.Errorf("plain release vk=%d = %q, want empty", vk, got)
		}
	}
}

func TestTranslateKeyToKittyMapsOEMPlusCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_OEM_PLUS, KeyDown: true}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[61u"; got != want {
		t.Fatalf("OEM plus = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyMapsOEMSlashCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_OEM_2, KeyDown: true}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[47u"; got != want {
		t.Fatalf("OEM slash = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyMapsNonEnhancedDeleteCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_DELETE, KeyDown: true}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[57426u"; got != want {
		t.Fatalf("non-enhanced Delete = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyEnhancedF4UsesKittyFunctionSuffixCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F4, KeyDown: true}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[S"; got != want {
		t.Fatalf("enhanced F4 = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyLeftShiftModifierCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_LSHIFT,
		ControlKeyState: vtinput.ShiftPressed,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[57441;2u"; got != want {
		t.Fatalf("left Shift = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyRightShiftScanCodeCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_RSHIFT,
		VirtualScanCode: vtinput.ScanCodeRightShift,
		ControlKeyState: vtinput.ShiftPressed,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[57447;2u"; got != want {
		t.Fatalf("right Shift = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyGenericMenuModifierCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_MENU,
		ControlKeyState: vtinput.LeftAltPressed,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[57443;3u"; got != want {
		t.Fatalf("generic Menu = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyShiftCapsLetterUsesLockModifierCoverageBatch39(t *testing.T) {
	e := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_A,
		Char:            'A',
		ControlKeyState: vtinput.ShiftPressed | vtinput.CapsLockOn,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(e, 8, false), "\x1b[97;66u"; got != want {
		t.Fatalf("Shift+Caps A = %q, want %q", got, want)
	}
}
