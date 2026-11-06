//go:build darwin

package panel

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func isFileContextMenuClick(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.MouseEventType || !e.KeyDown || e.WheelDirection != 0 ||
		e.MouseEventFlags&vtinput.MouseMoved != 0 || e.ButtonState != vtinput.FromLeft1stButtonPressed {
		return false
	}
	ctrlMask := vtinput.ControlKeyState(vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)
	if vtui.ActiveBackend() == "gogpu" {
		// GoGPU maps Command to LeftCtrl and physical Control to RightCtrl on
		// macOS. Finder's Control-click must not replace Command-click.
		ctrlMask = vtinput.RightCtrlPressed
	}
	ctrl := e.ControlKeyState&ctrlMask != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0
	return ctrl && !alt && !shift
}
