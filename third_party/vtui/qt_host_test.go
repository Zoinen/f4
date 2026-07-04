package vtui

import (
	"io"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func newTestQtHost(t *testing.T) *QtHost {
	t.Helper()
	pr, _ := io.Pipe()
	t.Cleanup(func() { _ = pr.Close() })
	return &QtHost{
		reader: vtinput.NewReader(pr, true),
		cols:   80,
		rows:   25,
	}
}

func readQtHostTestEvent(t *testing.T, host *QtHost) *vtinput.InputEvent {
	t.Helper()
	select {
	case ev := <-host.reader.EventChan:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Qt host event")
		return nil
	}
}

func TestQtHostResizeMessage(t *testing.T) {
	host := newTestQtHost(t)
	host.handleMessage(map[string]any{"type": "resize", "cols": 100, "rows": 40})

	ev := readQtHostTestEvent(t, host)
	if ev.Type != vtinput.ResizeEventType {
		t.Fatalf("event type = %v, want resize", ev.Type)
	}
	if host.cols != 100 || host.rows != 40 {
		t.Fatalf("host size = %dx%d, want 100x40", host.cols, host.rows)
	}
}

func TestQtHostKeyMouseAndPasteMessages(t *testing.T) {
	host := newTestQtHost(t)

	host.handleMessage(map[string]any{"type": "key", "vk": int(vtinput.VK_F5), "down": true, "mods": int(vtinput.ShiftPressed)})
	key := readQtHostTestEvent(t, host)
	if key.Type != vtinput.KeyEventType || !key.KeyDown || key.VirtualKeyCode != vtinput.VK_F5 || key.ControlKeyState != vtinput.ShiftPressed {
		t.Fatalf("unexpected key event: %#v", key)
	}

	host.handleMessage(map[string]any{"type": "mouse", "x": 3, "y": 4, "button": vtinput.FromLeft1stButtonPressed, "flags": vtinput.MouseMoved, "down": true})
	mouse := readQtHostTestEvent(t, host)
	if mouse.Type != vtinput.MouseEventType || mouse.MouseX != 3 || mouse.MouseY != 4 || mouse.ButtonState != vtinput.FromLeft1stButtonPressed || mouse.MouseEventFlags != vtinput.MouseMoved {
		t.Fatalf("unexpected mouse event: %#v", mouse)
	}

	host.handleMessage(map[string]any{"type": "paste", "text": "xy"})
	start := readQtHostTestEvent(t, host)
	first := readQtHostTestEvent(t, host)
	second := readQtHostTestEvent(t, host)
	end := readQtHostTestEvent(t, host)
	if start.Type != vtinput.PasteEventType || !start.PasteStart || first.Char != 'x' || second.Char != 'y' || end.Type != vtinput.PasteEventType || end.PasteStart {
		t.Fatalf("unexpected paste event sequence: %#v %#v %#v %#v", start, first, second, end)
	}
}
