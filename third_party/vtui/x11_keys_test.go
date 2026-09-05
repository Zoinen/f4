package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestX11_KeysymMapping(t *testing.T) {
	tests := []struct {
		keysym uint32
		wantVK uint16
	}{
		{0xff51, vtinput.VK_LEFT},
		{0xff8d, vtinput.VK_RETURN},  // KP_Enter
		{0xffb5, vtinput.VK_NUMPAD5}, // KP_5
		{0xffab, vtinput.VK_ADD},     // KP_Add
		{0x0061, vtinput.VK_A},       // 'a'
		{0x0041, vtinput.VK_A},       // 'A'
	}

	for _, tt := range tests {
		got := keysymToVK(tt.keysym)
		if got != tt.wantVK {
			t.Errorf("keysymToVK(0x%x) = 0x%x, want 0x%x", tt.keysym, got, tt.wantVK)
		}
	}
}

func TestX11_EnhancedKeyMapping(t *testing.T) {
	tests := []struct {
		name   string
		keysym uint32
		want   vtinput.ControlKeyState
	}{
		{"navigation delete", 0xffff, vtinput.EnhancedKey},
		{"navigation insert", 0xff63, vtinput.EnhancedKey},
		{"navigation home", 0xff50, vtinput.EnhancedKey},
		{"keypad delete", 0xff9f, 0},
		{"keypad insert", 0xff9e, 0},
		{"keypad home", 0xff95, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := enhancedKeyForX11Keysym(tt.keysym); got != tt.want {
				t.Errorf("enhancedKeyForX11Keysym(0x%x) = %v, want %v", tt.keysym, got, tt.want)
			}
		})
	}
}
