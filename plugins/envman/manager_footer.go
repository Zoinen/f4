package envman

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type managerFooterCommand struct {
	action    string
	icon      string
	hintIndex int
	key       uint16
	modifiers vtinput.ControlKeyState
	keyLabel  string
}

func managerFooterCommands(editing bool) []managerFooterCommand {
	if editing {
		return []managerFooterCommand{
			{action: "envman.save", icon: "save", hintIndex: 0, key: vtinput.VK_RETURN, modifiers: vtinput.LeftCtrlPressed},
			{action: "envman.cancel", icon: "x", hintIndex: 1, key: vtinput.VK_ESCAPE},
		}
	}
	return []managerFooterCommand{
		{action: "envman.add", icon: "plus", hintIndex: 0, key: vtinput.VK_INSERT},
		{action: "envman.delete", icon: "trash-2", hintIndex: 1, key: vtinput.VK_DELETE},
		{action: "envman.edit", icon: "pencil", hintIndex: 2, key: vtinput.VK_F4},
		{action: "envman.copy", icon: "copy", hintIndex: 3, key: vtinput.VK_F5},
		{action: "envman.toggle", icon: "circle-check", hintIndex: 4, key: vtinput.VK_SPACE},
		{
			action: "envman.moveUp", icon: "arrow-up", hintIndex: 5,
			key: vtinput.VK_UP, modifiers: vtinput.LeftCtrlPressed, keyLabel: "Ctrl+↑",
		},
		{
			action: "envman.moveDown", icon: "arrow-down", hintIndex: 5,
			key: vtinput.VK_DOWN, modifiers: vtinput.LeftCtrlPressed, keyLabel: "Ctrl+↓",
		},
		{action: "envman.settings", icon: "file-cog", hintIndex: 6, key: vtinput.VK_F2},
	}
}

func (window *managerWindow) footerCommandEnabled(command managerFooterCommand) bool {
	if window.controller == nil {
		return false
	}
	_, entry, selected := window.controller.selected()
	switch command.action {
	case "envman.add", "envman.settings", "envman.save", "envman.cancel":
		return true
	case "envman.edit", "envman.toggle":
		return selected && entry.Kind == KindProfile
	default:
		return selected
	}
}

func (window *managerWindow) footerHints() []map[string]any {
	parts := parseManagerHint(window.bottomHint)
	editing := window.controller != nil && window.controller.editing
	commands := managerFooterCommands(editing)
	hints := make([]map[string]any, 0, len(commands))
	for _, command := range commands {
		if command.hintIndex >= len(parts) {
			continue
		}
		part := parts[command.hintIndex]
		if command.keyLabel != "" {
			part.key = command.keyLabel
		}
		hints = append(hints, map[string]any{
			"key": part.key, "text": part.label, "action": command.action,
			"icon":     command.icon,
			"disabled": !window.footerCommandEnabled(command),
		})
	}
	return hints
}

// Route clicks through the keyboard behavior so profile operations have one owner.
func (window *managerWindow) handleFooterAction(action string) bool {
	if window.controller == nil {
		return false
	}
	for _, command := range managerFooterCommands(window.controller.editing) {
		if action != command.action || !window.footerCommandEnabled(command) {
			continue
		}
		if !window.controller.editing {
			window.SetFocusedItem(window.controller.list)
		}
		vtui.DebugLog("[FIX:envman-footer] action=%s", command.action)
		return window.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode: command.key, ControlKeyState: command.modifiers,
		})
	}
	return false
}
