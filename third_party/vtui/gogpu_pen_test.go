//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"io"
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

func TestPenPointerActionTurnsAPenIntoMouseActions(t *testing.T) {
	pen := func(typ gpucontext.PointerEventType, b gpucontext.Button) gpucontext.PointerEvent {
		return gpucontext.PointerEvent{Type: typ, PointerType: gpucontext.PointerTypePen, Button: b}
	}
	cases := []struct {
		name   string
		ev     gpucontext.PointerEvent
		action penAction
		button gpucontext.MouseButton
		ok     bool
	}{
		{"touch down", pen(gpucontext.PointerDown, gpucontext.ButtonLeft), penPress, gpucontext.MouseButtonLeft, true},
		{"release", pen(gpucontext.PointerUp, gpucontext.ButtonLeft), penRelease, gpucontext.MouseButtonLeft, true},
		{"barrel button", pen(gpucontext.PointerDown, gpucontext.ButtonRight), penPress, gpucontext.MouseButtonRight, true},
		{"middle", pen(gpucontext.PointerDown, gpucontext.ButtonMiddle), penPress, gpucontext.MouseButtonMiddle, true},
		{"hover or drag", pen(gpucontext.PointerMove, gpucontext.ButtonNone), penMove, 0, true},
	}
	for _, c := range cases {
		action, button, ok := penPointerAction(c.ev)
		if ok != c.ok || (ok && (action != c.action || (action != penMove && button != c.button))) {
			t.Errorf("%s: got (%v, %v, %v), want (%v, %v, %v)", c.name, action, button, ok, c.action, c.button, c.ok)
		}
	}

	// A mouse already reaches the mouse callbacks, a finger is left to the
	// operating system's own emulation, and other pen events say nothing to do.
	for _, typ := range []gpucontext.PointerType{gpucontext.PointerTypeMouse, gpucontext.PointerTypeTouch} {
		if _, _, ok := penPointerAction(gpucontext.PointerEvent{Type: gpucontext.PointerDown, PointerType: typ}); ok {
			t.Errorf("pointer type %v must not be forwarded", typ)
		}
	}
	if _, _, ok := penPointerAction(pen(gpucontext.PointerCancel, gpucontext.ButtonNone)); ok {
		t.Error("a cancelled pen event is not a mouse action")
	}
}

func TestGogpuHostPenPointerDrivesTheMouseHandling(t *testing.T) {
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	defer reader.Close()
	host := &GogpuHost{reader: reader, cellW: 8, cellH: 16}
	next := func() *vtinput.InputEvent {
		select {
		case ev := <-reader.EventChan:
			return ev
		default:
			return nil
		}
	}
	pen := func(typ gpucontext.PointerEventType, b gpucontext.Button, x, y float64) gpucontext.PointerEvent {
		return gpucontext.PointerEvent{Type: typ, PointerType: gpucontext.PointerTypePen, Button: b, X: x, Y: y}
	}

	host.penPointer(pen(gpucontext.PointerDown, gpucontext.ButtonLeft, 80, 48))
	if ev := next(); ev == nil || !ev.KeyDown || ev.MouseX != 10 || ev.MouseY != 3 || ev.ButtonState != uint32(vtinput.FromLeft1stButtonPressed) {
		t.Fatalf("pen down = %+v", ev)
	}
	// Dragging: a move into another cell carries the held button; a move inside
	// the same cell says nothing.
	host.penPointer(pen(gpucontext.PointerMove, gpucontext.ButtonNone, 96, 48))
	if ev := next(); ev == nil || ev.MouseEventFlags&vtinput.MouseMoved == 0 || ev.MouseX != 12 || ev.ButtonState != uint32(vtinput.FromLeft1stButtonPressed) {
		t.Fatalf("pen drag = %+v", ev)
	}
	host.penPointer(pen(gpucontext.PointerMove, gpucontext.ButtonNone, 97, 49))
	if ev := next(); ev != nil {
		t.Fatalf("a move inside the same cell reported %+v", ev)
	}
	host.penPointer(pen(gpucontext.PointerUp, gpucontext.ButtonLeft, 96, 48))
	if ev := next(); ev == nil || ev.KeyDown || ev.ButtonState != 0 {
		t.Fatalf("pen up = %+v", ev)
	}
	// The barrel button is the right button, the middle one stays the middle.
	host.penPointer(pen(gpucontext.PointerDown, gpucontext.ButtonRight, 0, 0))
	if ev := next(); ev == nil || ev.ButtonState != uint32(vtinput.RightmostButtonPressed) {
		t.Fatalf("barrel button = %+v", ev)
	}
	host.penPointer(pen(gpucontext.PointerDown, gpucontext.ButtonMiddle, 0, 0))
	if ev := next(); ev == nil || ev.ButtonState != uint32(vtinput.FromLeft2ndButtonPressed) {
		t.Fatalf("middle button = %+v", ev)
	}
	host.mousePress(gpucontext.MouseButton(99), 0, 0) // an unknown button counts as the left one
	if ev := next(); ev == nil || ev.ButtonState != uint32(vtinput.FromLeft1stButtonPressed) {
		t.Fatalf("unknown button = %+v", ev)
	}
	// A mouse pointer is not the pen's business.
	host.penPointer(gpucontext.PointerEvent{Type: gpucontext.PointerDown, PointerType: gpucontext.PointerTypeMouse})
	if ev := next(); ev != nil {
		t.Fatalf("a mouse pointer reported %+v", ev)
	}
}

func TestClampCellSaturatesInsteadOfWrapping(t *testing.T) {
	for in, want := range map[int]int16{0: 0, 12: 12, -5: -5, 40000: 32767, -40000: -32768, 1 << 40: 32767} {
		if got := clampCell(in); got != want {
			t.Errorf("clampCell(%d) = %d, want %d", in, got, want)
		}
	}
}
