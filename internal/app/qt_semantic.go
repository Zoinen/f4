package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"strings"
	"time"
)

func HandleSemanticAction(action map[string]any) bool {
	if action == nil {
		return false
	}
	if semantic.HandleStandaloneDocumentViewport(action) {
		return true
	}
	actionName := semantic.String(action["action"])
	if strings.HasPrefix(actionName, "queue.") && fileops.HandleQueueDropdownAction(action) {
		return true
	}
	target := semantic.String(action["target"])
	if actionName == "workspace.dragActivate" || actionName == "workspace.dropFiles" {
		return panel.HandleSemanticWorkspaceDrag(action)
	}
	if actionName == "toast.dismiss" {
		return vtui.FrameManager.HandleSemanticAction(action)
	}
	if target == "app" && (actionName == "presentation.toggle" || actionName == "toggle_presentation") {
		ToggleGuiPresentation()
		return true
	}
	if handled, claimed := fileops.HandleOperationsQueueWorkspaceClose(action); claimed {
		return handled
	}
	if strings.HasPrefix(actionName, "workspace.") || actionName == "tab.activate" || strings.HasPrefix(target, "workspace-") {
		return vtui.FrameManager.HandleSemanticAction(action)
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return vtui.FrameManager.EmitCommand(semantic.Int(action["command"]), action["args"])
	}
	if actionName == "menuBar.itemSelect" || actionName == "menuBar.itemActivate" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			menuIndex := semantic.Int(action["menuIndex"])
			itemIndex := semantic.Int(action["index"])
			if menuIndex >= 0 && menuIndex < len(mb.Items) &&
				itemIndex >= 0 && itemIndex < len(mb.Items[menuIndex].SubItems) &&
				!mb.Items[menuIndex].SubItems[itemIndex].Separator {
				if actionName == "menuBar.itemSelect" {
					// A pointer-hover action is deliberately non-activating. It may
					// arrive after the pointer has already moved to another top-level
					// menu, so accepting it must never reopen the stale submenu.
					if !mb.Active || mb.SelectPos != menuIndex {
						return false
					}
					menu := activeMenuBarSubmenu(mb, semantic.String(action["target"]))
					if menu == nil {
						return false
					}
					menu.SetSelectPos(itemIndex)
					vtui.FrameManager.DeclareSemanticMenuState()
					return true
				}

				// Click activation is atomic and may legitimately target a submenu
				// which QML was previewing before Go had materialized it.
				mb.Active = true
				if mb.SelectPos != menuIndex {
					mb.ActivateSubMenu(menuIndex)
				}
				if menu := activeMenuBarSubmenu(mb, ""); menu != nil {
					menu.SetSelectPos(itemIndex)
					handled := menu.ProcessKey(&vtinput.InputEvent{
						Type:           vtinput.KeyEventType,
						KeyDown:        true,
						VirtualKeyCode: vtinput.VK_RETURN,
						InputSource:    "qt_semantic",
					})
					if handled {
						// Semantic actions execute outside FrameManager's normal
						// keyboard-dispatch loop. Complete the same post-dispatch
						// lifecycle here: close the menu bar and remove every frame
						// whose command marked it done (for example viewer/editor
						// after File > Exit).
						vtui.FrameManager.EmitCommand(vtui.CmMenuClose, nil)
						frames := append([]vtui.Frame(nil),
							vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)...)
						for i := len(frames) - 1; i >= 0; i-- {
							if frames[i].IsDone() && frames[i].GetType() == vtui.TypeMenu {
								vtui.FrameManager.RemoveFrame(frames[i])
							}
						}

						frames = append(frames[:0],
							vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)...)
						closeWholeScreen := len(vtui.FrameManager.Screens) > 1
						hasDoneFrame := false
						for _, frame := range frames {
							if frame.IsDone() {
								hasDoneFrame = true
								continue
							}
							if frame.GetType() != vtui.TypeDesktop {
								closeWholeScreen = false
							}
						}
						if hasDoneFrame && closeWholeScreen {
							// Viewer/editor live on their own vtui screen. Closing
							// that screen restores the previous commander panels;
							// merely removing the document frame would expose its
							// otherwise-empty Desktop fallback.
							vtui.FrameManager.CloseActiveScreen()
						} else {
							for i := len(frames) - 1; i >= 0; i-- {
								if frames[i].IsDone() {
									vtui.FrameManager.RemoveFrame(frames[i])
								}
							}
						}
						vtui.FrameManager.Redraw()
					}
					return handled
				}
			}
		}
		return false
	}
	if actionName == "menu_bar_activate" || actionName == "menuBar.activate" || actionName == "menuBar.toggle" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			idx := semantic.Int(action["index"])
			if idx >= 0 && idx < len(mb.Items) {
				if actionName == "menuBar.toggle" && mb.Active && mb.SelectPos == idx {
					frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
					for i := len(frames) - 1; i >= 0; i-- {
						if menu, _ := nativeui.FrameVMenu(frames[i]); menu != nil {
							frames[i].Close()
							break
						}
					}
					mb.Active = false
					vtui.FrameManager.DeclareSemanticMenuState()
					return true
				}
				mb.Active = true
				mb.ActivateSubMenu(idx)
				return true
			}
		}
	}

	activeIdx := vtui.FrameManager.ActiveIdx
	frames := vtui.FrameManager.GetActiveFrames(activeIdx)
	// Command-line autocomplete is a modal frame in the terminal frontend, but
	// its native QML popup keeps the selection locally and submits an explicit
	// semantic action. Route that action straight to the owning PanelsFrame:
	// otherwise the modal overlay may consume it before the shell ever writes
	// the command to its PTY.
	if actionName == "command.submit" || actionName == "submit_command" ||
		actionName == "command.complete" || actionName == "complete_command" {
		for i := len(frames) - 1; i >= 0; i-- {
			if panels, ok := frames[i].(*panel.PanelsFrame); ok {
				if panels.HandleSemanticAction(action) {
					if !vtui.FrameManager.CurrentTaskDeclaredUnchanged() {
						vtui.FrameManager.Redraw()
					}
					return true
				}
			}
		}
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if h, ok := frames[i].(vtui.SemanticActionHandler); ok && h.HandleSemanticAction(action) {
			if !vtui.FrameManager.CurrentTaskDeclaredUnchanged() {
				vtui.FrameManager.Redraw()
			}
			return true
		}
	}

	target = semantic.String(action["target"])
	if target == "" {
		return false
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if handleSemanticFrameAction(frames[i], target, action) {
			if !semanticMenuPresentationAction(actionName) {
				vtui.FrameManager.Redraw()
			}
			return true
		}
	}
	return false
}

func semanticMenuPresentationAction(actionName string) bool {
	switch actionName {
	case "menu_select", "menu.select", "menu_scroll", "menu.scroll",
		"menu_open_submenu", "menu.openSubmenu", "menu_close_submenu", "menu.closeSubmenu",
		"menuBar.itemSelect", "menu_bar_activate", "menuBar.activate",
		"menuBar.toggle":
		return true
	default:
		return false
	}
}

func activeMenuBarSubmenu(mb *vtui.MenuBar, target string) *vtui.VMenu {
	if vtui.FrameManager == nil || mb == nil {
		return nil
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		menu, _ := nativeui.FrameVMenu(frames[i])
		if menu == nil || !nativeui.IsMenuBarSubmenu(frames[i], mb) {
			continue
		}
		if target != "" && vtui.SemanticID(frames[i]) != target {
			continue
		}
		return menu
	}
	return nil
}

func ToggleGuiPresentation() config.GuiPresentationMode {
	config.App.GuiPresentation = config.NextGuiPresentationMode(config.App.GuiPresentation)
	config.SaveConfig()
	if vtui.FrameManager != nil {
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				if config.App.GuiPresentation == config.GuiPresentationText {
					semantic.ApplyNativeDocumentViewport(frame, semantic.NativeDocumentGeometry{})
				} else {
					semantic.SeedNativeDocumentViewport(frame)
				}
			}
		}
		vtui.FrameManager.HardRefresh()
		message := i18n.Msg("Presentation.GUI")
		if config.App.GuiPresentation == config.GuiPresentationText {
			message = i18n.Msg("Presentation.Text")
		}
		vtui.ShowToast(message, 2*time.Second)
	}
	return config.App.GuiPresentation
}

func handleSemanticFrameAction(frame vtui.Frame, target string, action map[string]any) bool {
	if vtui.SemanticID(frame) == target {
		switch semantic.String(action["action"]) {
		case "close", "dialog.close", "window.close":
			frame.Close()
			return true
		case "menu_close_chain", "menu.closeChain":
			if menu, _ := nativeui.FrameVMenu(frame); menu != nil {
				menu.CloseChain()
				return true
			}
		case "menu_close_submenu", "menu.closeSubmenu":
			if menu, _ := nativeui.FrameVMenu(frame); menu != nil {
				menu.CloseSubmenu()
				return true
			}
		case "menu_open_submenu", "menu.openSubmenu":
			if menu, _ := nativeui.FrameVMenu(frame); menu != nil {
				idx := semantic.Int(action["index"])
				return menu.OpenSubmenu(idx)
			}
		case "menu_activate", "menu.activate":
			if menu, _ := nativeui.FrameVMenu(frame); menu != nil {
				idx := semantic.Int(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator &&
					!menu.Items[idx].Header && !menu.Items[idx].Disabled {
					menu.SetSelectPos(idx)
					// A frame may wrap VMenu to add authoritative activation
					// semantics while retaining the shared menu projection. Route
					// Enter through that actual frame so the wrapper is not bypassed.
					return frame.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		case "menu_select", "menu.select":
			if menu, _ := nativeui.FrameVMenu(frame); menu != nil {
				idx := semantic.Int(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator &&
					!menu.Items[idx].Header && !menu.Items[idx].Disabled {
					menu.SetSelectPos(idx)
					vtui.FrameManager.DeclareSemanticMenuState()
					return true
				}
			}
		case "menu_scroll", "menu.scroll":
			if menu, _ := nativeui.FrameVMenu(frame); menu != nil {
				if top, absolute := action["top"]; absolute {
					menu.ScrollBy(semantic.Int(top) - menu.TopPos)
				} else {
					menu.ScrollBy(semantic.Int(action["delta"]))
				}
				vtui.FrameManager.DeclareSemanticMenuState()
				return true
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
