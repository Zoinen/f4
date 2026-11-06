//go:build darwin

package app

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// darwinMouseModifierTracker repairs GoGPU mouse presses, which omit the
// modifiers that its key events and mouse-move events already report.
type darwinMouseModifierTracker struct {
	held vtinput.ControlKeyState
}

func (t *darwinMouseModifierTracker) normalize(e *vtinput.InputEvent, backend string) {
	if e == nil || backend != "gogpu" {
		return
	}
	if e.Type == vtinput.FocusEventType && !e.SetFocus {
		t.held = 0
		return
	}
	if e.Type == vtinput.KeyEventType {
		t.updateKey(e)
		return
	}
	if e.Type != vtinput.MouseEventType || t.held == 0 {
		return
	}

	missing := t.held &^ e.ControlKeyState
	e.ControlKeyState |= t.held
	if missing != 0 && e.KeyDown && e.MouseEventFlags&vtinput.MouseMoved == 0 {
		vtui.DebugLog("[FIX] restored Darwin GoGPU mouse modifiers: %#x", missing)
	}
}

func (t *darwinMouseModifierTracker) updateKey(e *vtinput.InputEvent) {
	mask := darwinModifierForKey(e)
	if mask == 0 {
		return
	}
	if e.KeyDown {
		t.held |= mask
		return
	}
	t.held &^= mask
}

func darwinModifierForKey(e *vtinput.InputEvent) vtinput.ControlKeyState {
	switch e.VirtualKeyCode {
	case vtinput.VK_LCONTROL:
		return vtinput.LeftCtrlPressed
	case vtinput.VK_RCONTROL:
		return vtinput.RightCtrlPressed
	case vtinput.VK_CONTROL:
		if ctrl := e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed); ctrl != 0 {
			return ctrl
		}
		return vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed
	case vtinput.VK_LMENU:
		return vtinput.LeftAltPressed
	case vtinput.VK_RMENU:
		return vtinput.RightAltPressed
	case vtinput.VK_MENU:
		if alt := e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed); alt != 0 {
			return alt
		}
		return vtinput.LeftAltPressed | vtinput.RightAltPressed
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT:
		return vtinput.ShiftPressed
	default:
		return 0
	}
}
