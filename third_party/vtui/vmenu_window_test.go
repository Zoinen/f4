package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestWindowMenuControls(t *testing.T) {
	screen := NewSilentScreenBuf()
	screen.AllocBuf(100, 40)
	FrameManager.Init(screen)
	m := NewVMenu("History")
	m.SemanticPresentation = "window"
	m.SetPosition(10, 5, 69, 24)
	FrameManager.PushMenu(m)
	defer FrameManager.RemoveFrame(m)
	press := func(x, y int) {
		t.Helper()
		if !m.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType,
			KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: int16(x), MouseY: int16(y)}) {
			t.Fatal("window did not consume mouse press")
		}
	}
	m.Show(screen)
	press(62, 5) // Standard maximize control, seven cells from the right edge.
	if m.X1 != 0 || m.Y1 != 0 || m.X2 != 99 || m.Y2 != 38 {
		t.Fatalf("maximize bounds: %d,%d,%d,%d", m.X1, m.Y1, m.X2, m.Y2)
	}
	press(m.X2-7, m.Y1)
	if m.X1 != 10 || m.Y1 != 5 || m.X2 != 69 || m.Y2 != 24 {
		t.Fatal("restore lost original bounds")
	}
	press(69, 24)
	m.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved, MouseX: 79, MouseY: 29})
	m.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 79, MouseY: 29})
	if m.X2 != 79 || m.Y2 != 29 || m.ViewHeight != 23 {
		t.Fatalf("resize failed: %d,%d height=%d", m.X2, m.Y2, m.ViewHeight)
	}
	if !m.HandleSemanticAction(map[string]any{"target": SemanticID(m), "action": "menu.geometry",
		"x": 3, "y": 2, "w": 50, "h": 15}) {
		t.Fatal("native geometry was not accepted")
	}
	if m.X1 != 3 || m.Y1 != 2 || m.X2 != 52 || m.Y2 != 16 {
		t.Fatal("native geometry did not update console bounds")
	}
	press(m.X2-4, m.Y1)
	if !m.IsDone() || m.exitCode != -1 {
		t.Fatal("close must cancel rather than execute a history entry")
	}
}
