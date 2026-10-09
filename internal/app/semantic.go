package app

import (
	"strings"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// HandleSemanticAction глобально маршрутизирует семантические действия из внешнего GUI
func HandleSemanticAction(action map[string]any) bool {
	if action == nil {
		return false
	}
	actionName := semantic.String(action["action"])
	target := semantic.String(action["target"])
	if strings.HasPrefix(actionName, "workspace.") || actionName == "tab.activate" || strings.HasPrefix(target, "workspace-") {
		return vtui.FrameManager.HandleSemanticAction(action)
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return vtui.FrameManager.EmitCommand(semantic.Int(action["command"]), action["args"])
	}
	if semantic.String(action["action"]) == "menu_bar_activate" || semantic.String(action["action"]) == "menuBar.activate" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			idx := semantic.Int(action["index"])
			if idx >= 0 && idx < len(mb.Items) {
				mb.Active = true
				mb.ActivateSubMenu(idx)
				vtui.FrameManager.Redraw()
				return true
			}
		}
	}

	activeIdx := vtui.FrameManager.ActiveIdx
	frames := vtui.FrameManager.GetActiveFrames(activeIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		if h, ok := frames[i].(vtui.SemanticActionHandler); ok && h.HandleSemanticAction(action) {
			vtui.FrameManager.Redraw()
			return true
		}
	}

	target = semantic.String(action["target"])
	if target == "" {
		return false
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if handleSemanticFrameAction(frames[i], target, action) {
			vtui.FrameManager.Redraw()
			return true
		}
	}
	return false
}

func handleSemanticFrameAction(frame vtui.Frame, target string, action map[string]any) bool {
	if vtui.SemanticID(frame) == target {
		switch semantic.String(action["action"]) {
		case "close", "dialog.close", "window.close":
			frame.Close()
			return true
		case "menu_activate", "menu.activate":
			if menu, ok := frame.(*vtui.VMenu); ok {
				idx := semantic.Int(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					// The index names an entry of the full list, which a
					// filter typed into the menu (#263) may be hiding.
					menu.ClearFilter()
					menu.SetSelectPos(idx)
					return menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		}
	}
	if c, ok := frame.(vtui.Container); ok {
		return handleSemanticChildrenAction(c.GetChildren(), target, action)
	}
	return false
}

func handleSemanticChildrenAction(children []vtui.UIElement, target string, action map[string]any) bool {
	for _, child := range children {
		if vtui.SemanticID(child) == target {
			return handleSemanticElementAction(child, action)
		}
		if c, ok := child.(vtui.Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

func handleSemanticElementAction(el vtui.UIElement, action map[string]any) bool {
	switch semantic.String(action["action"]) {
	case "focus", "control.focus":
		el.SetFocus(true)
		return true
	case "activate", "control.activate":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "toggle", "control.toggle":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', InputSource: "qt_semantic"})
	case "set_text", "control.setText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.SetText(semantic.String(action["text"]))
			if edit.OnTextChange != nil {
				edit.OnTextChange(edit.GetText())
			}
			return true
		}
	case "insert_text", "control.insertText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.InsertString(semantic.String(action["text"]))
			return true
		}
	case "select", "control.select":
		idx := semantic.Int(action["index"])
		switch w := el.(type) {
		case *vtui.RadioGroup:
			if idx >= 0 && idx < len(w.Items) {
				w.SetData(idx)
				return true
			}
		case *vtui.ListBox:
			if idx >= 0 && idx < len(w.Items) {
				w.SetSelectPos(idx)
				return true
			}
		case *vtui.ComboBox:
			if idx >= 0 && idx < len(w.Menu.Items) {
				w.Menu.SetSelectPos(idx)
				w.Edit.SetText(w.Menu.Items[idx].Text)
				return true
			}
		}
	}
	return false
}
