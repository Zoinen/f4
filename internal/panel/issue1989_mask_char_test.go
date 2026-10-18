package panel

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestMaskCharFallbackApplies(t *testing.T) {
	tests := []struct {
		name string
		e    vtinput.InputEvent
		want bool
	}{
		{"main keyboard minus on exact input", vtinput.InputEvent{Char: '-', VirtualKeyCode: vtinput.VK_OEM_MINUS}, false},
		{"main keyboard plus on exact input", vtinput.InputEvent{Char: '+', VirtualKeyCode: vtinput.VK_OEM_PLUS}, false},
		{"main keyboard asterisk on exact input", vtinput.InputEvent{Char: '*', VirtualKeyCode: '8'}, false},
		{"gray minus", vtinput.InputEvent{Char: '-', VirtualKeyCode: vtinput.VK_SUBTRACT}, true},
		{"gray plus", vtinput.InputEvent{Char: '+', VirtualKeyCode: vtinput.VK_ADD}, true},
		{"gray asterisk", vtinput.InputEvent{Char: '*', VirtualKeyCode: vtinput.VK_MULTIPLY}, true},
		{"legacy character", vtinput.InputEvent{Char: '-', VirtualKeyCode: vtinput.VK_OEM_MINUS, IsLegacy: true}, true},
		{"character without virtual key", vtinput.InputEvent{Char: '-'}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskCharFallbackApplies(&tt.e); got != tt.want {
				t.Fatalf("maskCharFallbackApplies = %v, want %v", got, tt.want)
			}
		})
	}
}
