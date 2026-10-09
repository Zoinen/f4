package panel

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestConsoleOverlayKeyBarTracksStandaloneModifiers(t *testing.T) {
	pf := &PanelsFrame{}

	pf.updateConsoleOverlayModifiers(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SHIFT,
	})
	if !pf.consoleOverlayShift || pf.consoleOverlayCtrl || pf.consoleOverlayAlt {
		t.Fatalf("Shift down state = %v/%v/%v, want true/false/false", pf.consoleOverlayShift, pf.consoleOverlayCtrl, pf.consoleOverlayAlt)
	}

	pf.updateConsoleOverlayModifiers(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F1,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if !pf.consoleOverlayCtrl || pf.consoleOverlayShift || pf.consoleOverlayAlt {
		t.Fatalf("Ctrl F1 state = %v/%v/%v, want false/true/false", pf.consoleOverlayShift, pf.consoleOverlayCtrl, pf.consoleOverlayAlt)
	}

	pf.updateConsoleOverlayModifiers(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_SHIFT,
		ControlKeyState: vtinput.ShiftPressed,
	})
	if pf.consoleOverlayShift {
		t.Fatal("Shift release must clear the overlay modifier state")
	}

	pf.updateConsoleOverlayModifiers(&vtinput.InputEvent{Type: vtinput.FocusEventType})
	if pf.consoleOverlayShift || pf.consoleOverlayCtrl || pf.consoleOverlayAlt {
		t.Fatal("focus loss must clear all overlay modifier state")
	}
}

func TestConsoleOverlayKeyBarUsesActiveModifierLabels(t *testing.T) {
	labels := &vtui.KeySet{
		Normal: vtui.KeyBarLabels{"normal"},
		Shift:  vtui.KeyBarLabels{"shift"},
		Ctrl:   vtui.KeyBarLabels{"ctrl"},
		Alt:    vtui.KeyBarLabels{"alt"},
	}

	tests := []struct {
		name        string
		shift, ctrl bool
		alt         bool
		want        string
	}{
		{name: "normal", want: "normal"},
		{name: "alt", alt: true, want: "alt"},
		{name: "ctrl", ctrl: true, want: "ctrl"},
		{name: "shift", shift: true, want: "shift"},
		{name: "shift wins", shift: true, ctrl: true, alt: true, want: "shift"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consoleOverlayLabels(labels, tt.shift, tt.ctrl, tt.alt)[0]
			if got != tt.want {
				t.Fatalf("label = %q, want %q", got, tt.want)
			}
		})
	}
}
