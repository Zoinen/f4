package main

import (
	"testing"

	"github.com/unxed/vtinput"
)

// A terminal viewport that starts below a workspace tab bar must hand the
// child process its own coordinates, not the raw screen ones (#87).
func TestRebaseTerminalMouseEventSubtractsViewportOrigin(t *testing.T) {
	e := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      7,
		MouseY:      4,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}
	got := rebaseTerminalMouseEvent(e, 0, 1, 80, 24)
	if got.MouseX != 7 || got.MouseY != 3 {
		t.Fatalf("rebased event = (%d,%d), want (7,3)", got.MouseX, got.MouseY)
	}
	if e.MouseX != 7 || e.MouseY != 4 {
		t.Fatalf("source event was mutated: (%d,%d)", e.MouseX, e.MouseY)
	}
	if seq := TranslateMouseInput(got); seq != "\x1b[<0;8;4M" {
		t.Fatalf("encoded sequence = %q, want %q", seq, "\x1b[<0;8;4M")
	}
}

// With no tab bar the viewport already starts at the top-left corner, so the
// event must travel untouched.
func TestRebaseTerminalMouseEventKeepsUnshiftedViewport(t *testing.T) {
	e := &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 7, MouseY: 4}
	if got := rebaseTerminalMouseEvent(e, 0, 0, 80, 25); got != e {
		t.Fatalf("event was copied for an unshifted viewport")
	}
}

// Rows above the viewport (the tab bar itself) and coordinates past its last
// cell must not turn into negative or out-of-range cells.
func TestRebaseTerminalMouseEventClampsToViewport(t *testing.T) {
	above := &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 1, MouseY: 0}
	if got := rebaseTerminalMouseEvent(above, 2, 1, 80, 24); got.MouseX != 0 || got.MouseY != 0 {
		t.Fatalf("event above the viewport = (%d,%d), want (0,0)", got.MouseX, got.MouseY)
	}
	below := &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 200, MouseY: 200}
	if got := rebaseTerminalMouseEvent(below, 0, 1, 80, 24); got.MouseX != 79 || got.MouseY != 23 {
		t.Fatalf("event past the viewport = (%d,%d), want (79,23)", got.MouseX, got.MouseY)
	}
}
