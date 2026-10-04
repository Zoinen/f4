package panel

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type progressTaskDialog struct{ *vtui.Window }

func (d *progressTaskDialog) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.KeyEventType && e.KeyDown && e.VirtualKeyCode == vtinput.VK_C &&
		e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 &&
		e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed|vtinput.ShiftPressed) == 0 {
		vtui.DebugLog("[FIX:remote-open] cancelling progress task through Ctrl+C")
		d.SetExitCode(1)
		return true
	}
	return d.Window.ProcessKey(e)
}
