package keymap

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestTranslateKeyToKittyIgnoresUnreportedKeyUp(t *testing.T) {
	event := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_A, Char: 'a'}
	if got := TranslateKeyToKitty(event, 0, false); got != "" {
		t.Fatalf("unreported key-up = %q, want empty", got)
	}
}

func TestTranslateKeyToKittyIgnoresPlainReportedReturnRelease(t *testing.T) {
	event := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_RETURN, Char: '\r'}
	if got := TranslateKeyToKitty(event, 2, false); got != "" {
		t.Fatalf("plain Return release = %q, want empty", got)
	}
}

func TestTranslateKeyToKittyCapsLockAddsLockModifier(t *testing.T) {
	event := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_A,
		Char:            'A',
		ControlKeyState: vtinput.CapsLockOn,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[97;65u"; got != want {
		t.Fatalf("CapsLock key = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyNumLockAddsLockModifier(t *testing.T) {
	event := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_NUMPAD1,
		Char:            '1',
		ControlKeyState: vtinput.NumLockOn,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[49;129u"; got != want {
		t.Fatalf("NumLock key = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyMapsOEMMinus(t *testing.T) {
	event := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_OEM_MINUS, KeyDown: true}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[45u"; got != want {
		t.Fatalf("OEM minus = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyEnhancedDelete(t *testing.T) {
	event := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_DELETE, ControlKeyState: vtinput.EnhancedKey, KeyDown: true}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[3~"; got != want {
		t.Fatalf("enhanced Delete = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyF12(t *testing.T) {
	event := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F12, KeyDown: true}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[24~"; got != want {
		t.Fatalf("F12 = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyLeftAltInKittyMode(t *testing.T) {
	event := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_LMENU,
		ControlKeyState: vtinput.LeftAltPressed,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[57443;3u"; got != want {
		t.Fatalf("left Alt = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyEnhancedRightAltInKittyMode(t *testing.T) {
	event := &vtinput.InputEvent{
		VirtualKeyCode:  vtinput.VK_RMENU,
		ControlKeyState: vtinput.RightAltPressed | vtinput.EnhancedKey,
		KeyDown:         true,
	}
	if got, want := TranslateKeyToKitty(event, 8, false), "\x1b[57449;3u"; got != want {
		t.Fatalf("enhanced right Alt = %q, want %q", got, want)
	}
}

func TestTranslateKeyToKittyFunctionReleaseUsesReleaseSuffix(t *testing.T) {
	event := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F3}
	if got, want := TranslateKeyToKitty(event, 2, false), "\x1b[13;1:3~"; got != want {
		t.Fatalf("F3 release = %q, want %q", got, want)
	}
}
