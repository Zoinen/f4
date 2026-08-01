package main

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestTranslateInput(t *testing.T) {
	tests := []struct {
		name       string
		e          *vtinput.InputEvent
		kittyFlags int
		want       string
	}{
		{"Char 'a'", &vtinput.InputEvent{Char: 'a', KeyDown: true}, 0, "a"},
		{"Ctrl+a", &vtinput.InputEvent{Char: 'a', ControlKeyState: vtinput.LeftCtrlPressed, KeyDown: true}, 0, string(rune(1))},
		{"Up Arrow", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_UP, KeyDown: true}, 0, "\x1b[A"},
		{"Shift+Up Arrow", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_UP, ControlKeyState: vtinput.ShiftPressed, KeyDown: true}, 0, "\x1b[1;2A"},
		{"F1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F1, KeyDown: true}, 0, "\x1bOP"},
		{"F5", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F5, KeyDown: true}, 0, "\x1b[15~"},
		{"Alt+a", &vtinput.InputEvent{Char: 'a', ControlKeyState: vtinput.LeftAltPressed, KeyDown: true}, 0, "\x1ba"},
		{"Alt+Enter", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: vtinput.LeftAltPressed, KeyDown: true}, 0, "\x1b\r"},
		{"Standalone Modifier", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_SHIFT, KeyDown: true}, 0, ""},

		// Kitty fallback tests via TranslateInput (should generate final legacy strings since TranslateKeyToKitty returned "")
		{"Numpad Insert (Enhanced) Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_INSERT, ControlKeyState: vtinput.EnhancedKey, KeyDown: true}, 1, "\x1b[2~"},
		{"Left Arrow (Enhanced) Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.EnhancedKey, KeyDown: true}, 1, "\x1b[D"},
		{"Ctrl+I Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_I, Char: 9, ControlKeyState: vtinput.LeftCtrlPressed, KeyDown: true}, 1, "\x1b[9;5u"},
		{"Tab Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_TAB, Char: '\t', KeyDown: true}, 1, "\t"},
		{"Shift+Tab Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_TAB, Char: '\t', ControlKeyState: vtinput.ShiftPressed, KeyDown: true}, 1, "\x1b[9;2u"},
		{"Ctrl+Alt+A Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_A, Char: 0, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed, KeyDown: true}, 1, "\x1b[97;7u"},
		{"Shift+Space Kitty Mode 1", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', ControlKeyState: vtinput.ShiftPressed, KeyDown: true}, 1, " "},
		{"Shift+Space Kitty Mode 8", &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', ControlKeyState: vtinput.ShiftPressed, KeyDown: true}, 8, "\x1b[32;2u"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TranslateInput(tt.e, false, tt.kittyFlags, false)
			if got != tt.want {
				t.Errorf("TranslateInput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslateInput_Win32(t *testing.T) {
	e := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		VirtualKeyCode:  65,
		VirtualScanCode: 30,
		Char:            'A',
		KeyDown:         true,
		ControlKeyState: 8,
		RepeatCount:     1,
	}
	res := TranslateInput(e, true, 0, false)
	expected := "\x1b[65;30;65;1;8;1_"
	if res != expected {
		t.Errorf("Expected %q, got %q", expected, res)
	}
}
func TestTranslateMouseInput(t *testing.T) {
	tests := []struct {
		name string
		e    *vtinput.InputEvent
		want string
	}{
		{
			"Left Click Press",
			&vtinput.InputEvent{KeyDown: true, MouseX: 10, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed},
			"\x1b[<0;11;6M",
		},
		{
			"Left Click Release",
			&vtinput.InputEvent{KeyDown: false, MouseX: 10, MouseY: 5},
			"\x1b[<3;11;6m",
		},
		{
			"Right Click Move",
			&vtinput.InputEvent{KeyDown: true, MouseX: 20, MouseY: 15, ButtonState: vtinput.RightmostButtonPressed, MouseEventFlags: vtinput.MouseMoved},
			"\x1b[<34;21;16M",
		},
		{
			"Wheel Up with Shift",
			&vtinput.InputEvent{WheelDirection: 1, MouseX: 0, MouseY: 0, ControlKeyState: vtinput.ShiftPressed},
			"\x1b[<68;1;1M",
		},
		{
			"Wheel Down with Ctrl+Alt",
			&vtinput.InputEvent{WheelDirection: -1, MouseX: 5, MouseY: 5, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed},
			"\x1b[<89;6;6M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TranslateMouseInput(tt.e)
			if got != tt.want {
				t.Errorf("TranslateMouseInput() = %q, want %q", got, tt.want)
			}
		})
	}
}
