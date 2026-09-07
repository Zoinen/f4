package main

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestTranslateKeyToKitty(t *testing.T) {
	tests := []struct {
		name  string
		event *vtinput.InputEvent
		flags int
		want  string
	}{
		{
			name: "Normal Char 'a'",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_A,
				Char:           'a',
				KeyDown:        true,
			},
			flags: 1,  // Disambiguate on
			want:  "", // Normal characters don't generate kitty sequences unless mode 8
		},
		{
			name: "Ctrl+Alt+A",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_A,
				Char:            0,
				ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed,
				KeyDown:         true,
			},
			flags: 1,
			want:  "\x1b[97;7u", // modifiers: shift(1) off, alt(2) on, ctrl(4) on -> 1+2+4 = 7
		},
		{
			name: "F3 with Disambiguate",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_F3,
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[13~", // F3 is special cased to 13~
		},
		{
			name: "Escape",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_ESCAPE,
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[27u",
		},
		{
			name: "Key Release (Mode 2)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_A,
				Char:           'a',
				KeyDown:        false,
			},
			flags: 3,              // Disambiguate + Report Events
			want:  "\x1b[97;1:3u", // 1 = modifiers (none), :3 = release
		},
		{
			name: "Alternate Keys (Mode 4)",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_A,
				Char:            'A',
				ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
				KeyDown:         true,
			},
			flags: 1 | 4,
			want:  "\x1b[97:65;6u", // 97 (a) : 65 (A) ; 6 (ctrl+shift) u (base is same as keycode so it is omitted)
		},
		{
			name: "Numpad 0 (Not Enhanced)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_NUMPAD0,
				Char:           '0',
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[48u", // 48 is ASCII '0'
		},
		{
			name: "Numpad 5 without NumLock (VK_CLEAR)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_CLEAR,
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[57427~",
		},
		{
			name: "Numpad 5 with NumLock",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_NUMPAD5,
				Char:            '5',
				ControlKeyState: vtinput.NumLockOn,
				KeyDown:         true,
			},
			flags: 1,
			want:  "", // Falls back to legacy text generation
		},
		{
			name: "Numpad Insert (Enhanced)",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_INSERT,
				ControlKeyState: vtinput.EnhancedKey,
				KeyDown:         true,
			},
			flags: 1,
			want:  "", // Falls back to legacy "\x1b[2~" because it's a non-CSIu function key in Mode 1
		},
		{
			name: "Numpad Insert (Not Enhanced)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_INSERT,
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[57425u", // CSIu mapping
		},
		{
			name: "Left Arrow (Enhanced)",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_LEFT,
				ControlKeyState: vtinput.EnhancedKey,
				KeyDown:         true,
			},
			flags: 1,
			want:  "", // Falls back to legacy "\x1b[D" because suffix is 'D' (non-CSIu) and mode 1
		},
		{
			name: "Left Arrow (Not Enhanced)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_LEFT,
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[57417u",
		},
		{
			name: "F1 (Flags 1)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_F1,
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[P",
		},
		{
			name: "Mode 8 - All Keys as Esc",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_A,
				Char:            'A',
				ControlKeyState: vtinput.ShiftPressed,
				KeyDown:         true,
			},
			flags: 8 | 4,
			want:  "\x1b[97:65;2u",
		},
		{
			name: "Mode 16 - Shift+Space",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_SPACE,
				Char:            ' ',
				ControlKeyState: vtinput.ShiftPressed,
				KeyDown:         true,
			},
			flags: 8 | 16,
			want:  "\x1b[32;2;32u", // keycode;mods;char
		},
		{
			name: "Mode 16 - Space (No Mods, double semicolon)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_SPACE,
				Char:           ' ',
				KeyDown:        true,
			},
			flags: 8 | 16,
			want:  "\x1b[32;;32u", // keycode;;char (double semicolon because mods skipped)
		},
		{
			name: "Mode 8 - Modifier Key Press (Left Ctrl)",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_LCONTROL,
				ControlKeyState: vtinput.LeftCtrlPressed,
				KeyDown:         true,
			},
			flags: 8,
			want:  "\x1b[57442;5u", // Left Ctrl = 57442, modifier = 4+1 = 5
		},
		{
			name: "Mode 8 - Modifier Key Press (Right Ctrl)",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_RCONTROL,
				ControlKeyState: vtinput.RightCtrlPressed | vtinput.EnhancedKey,
				KeyDown:         true,
			},
			flags: 8,
			want:  "\x1b[57448;5u", // Right Ctrl = 57448, modifier = 5
		},
		{
			name: "Noname IME KeyUp (should be ignored)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: 0xFC,
				Char:           'x',
				KeyDown:        false,
			},
			flags: 1,
			want:  "",
		},
		{
			name: "Noname IME KeyDown (should be sent as text)",
			event: &vtinput.InputEvent{
				VirtualKeyCode: 0xFC,
				Char:           'x',
				KeyDown:        true,
			},
			flags: 1,
			want:  "x",
		},
		{
			name: "Numpad Divide without Char",
			event: &vtinput.InputEvent{
				VirtualKeyCode: vtinput.VK_DIVIDE,
				Char:           0, // Char is missing
				KeyDown:        true,
			},
			flags: 1,
			want:  "\x1b[57410u", // Should fall back to PUA code for divide
		},
		{
			name: "Numpad Enter (Enhanced)",
			event: &vtinput.InputEvent{
				VirtualKeyCode:  vtinput.VK_RETURN,
				ControlKeyState: vtinput.EnhancedKey,
				Char:            '\r',
				KeyDown:         true,
			},
			flags: 1,
			want:  "\x1b[13u", // Enhanced Enter mapped to 13
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TranslateKeyToKitty(tt.event, tt.flags, false)
			if got != tt.want {
				t.Errorf("TranslateKeyToKitty() = %q, want %q", got, tt.want)
			}
		})
	}
}
