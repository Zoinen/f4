package cmdline

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// CompletionMenu applies command-line shortcuts to the shared suggestion menu.
type CompletionMenu struct {
	*vtui.AutoCompleteMenu
}

func NewCompletionMenu(edit *vtui.Edit) *CompletionMenu {
	return &CompletionMenu{AutoCompleteMenu: vtui.NewAutoCompleteMenu(edit)}
}

func (menu *CompletionMenu) ProcessKey(event *vtinput.InputEvent) bool {
	ctrl := event.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	if event.Type == vtinput.KeyEventType && event.VirtualKeyCode == vtinput.VK_RETURN && ctrl {
		if !event.KeyDown {
			return true
		}
		// Tab accepts the chosen suggestion (or the first match) without
		// queuing Enter for the command executor. Keep the original event intact.
		if menu.Edit.AutoCompletePreview && menu.SelectPos() == 0 {
			menu.AutoCompleteMenu.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN,
			})
		}
		accept := *event
		accept.VirtualKeyCode = vtinput.VK_TAB
		accept.Char = 0
		accept.ControlKeyState = 0
		return menu.AutoCompleteMenu.ProcessKey(&accept)
	}
	wasDone := menu.IsDone()
	handled := menu.AutoCompleteMenu.ProcessKey(event)
	modifiers := vtinput.ShiftPressed | vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed |
		vtinput.LeftAltPressed | vtinput.RightAltPressed
	if menu.Edit.AutoCompletePreview && !wasDone && menu.IsDone() &&
		event.Type == vtinput.KeyEventType && event.KeyDown && event.VirtualKeyCode == vtinput.VK_RETURN &&
		event.ControlKeyState&modifiers == 0 && vtui.FrameManager != nil {
		// The shared preview menu only accepts text. A plain Enter in the
		// command line also submits it, just as in the non-preview menu.
		vtui.FrameManager.InjectEvents([]*vtinput.InputEvent{{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
		}})
	}
	return handled
}

// AsCompletionMenu exposes the shared popup data for console overlay rendering.
func AsCompletionMenu(frame vtui.Frame) (*vtui.AutoCompleteMenu, bool) {
	switch menu := frame.(type) {
	case *CompletionMenu:
		return menu.AutoCompleteMenu, true
	case *vtui.AutoCompleteMenu:
		return menu, true
	default:
		return nil, false
	}
}
