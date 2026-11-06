//go:build darwin

package panel

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestControlClickOpensFileContextMenuOnDarwin(t *testing.T) {
	vtui.SetActiveBackend("gogpu")
	t.Cleanup(func() { vtui.SetActiveBackend("") })
	base := vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		ControlKeyState: vtinput.RightCtrlPressed,
	}
	if !isFileContextMenuClick(&base) {
		t.Fatal("Control-click was not recognized")
	}

	for name, mutate := range map[string]func(*vtinput.InputEvent){
		"plain click":   func(e *vtinput.InputEvent) { e.ControlKeyState = 0 },
		"Command-click": func(e *vtinput.InputEvent) { e.ControlKeyState = vtinput.LeftCtrlPressed },
		"right click":   func(e *vtinput.InputEvent) { e.ButtonState = vtinput.RightmostButtonPressed },
		"drag":          func(e *vtinput.InputEvent) { e.MouseEventFlags = vtinput.MouseMoved },
		"release":       func(e *vtinput.InputEvent) { e.KeyDown = false },
		"shift":         func(e *vtinput.InputEvent) { e.ControlKeyState |= vtinput.ShiftPressed },
		"option":        func(e *vtinput.InputEvent) { e.ControlKeyState |= vtinput.LeftAltPressed },
	} {
		t.Run(name, func(t *testing.T) {
			e := base
			mutate(&e)
			if isFileContextMenuClick(&e) {
				t.Fatal("non-Control-click gesture was recognized")
			}
		})
	}
}
