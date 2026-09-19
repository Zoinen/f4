package envman

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// SemanticNode exposes the pane, shortcut and profile states painted by the TUI.
func (window *managerWindow) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	window.layoutPaneButtons()
	node := window.Window.SemanticNode(ctx)
	node["layout"] = "environmentProfiles"
	node["keyHints"] = window.footerHints()
	rightTitle, active := window.detailsTitle, "left"
	if window.controller != nil && window.controller.editing {
		rightTitle, active = window.editingTitle, "right"
	}
	node["paneSplit"] = map[string]any{
		"x": window.X1 + window.splitOffset, "leftTitle": window.profilesTitle,
		"rightTitle": rightTitle, "active": active,
	}
	if window.controller != nil {
		controller := window.controller
		roles := map[string]string{
			vtui.SemanticID(controller.list):           "profiles",
			vtui.SemanticID(controller.nameLabel):      "name-label",
			vtui.SemanticID(controller.nameEdit):       "name",
			vtui.SemanticID(controller.enabledEdit):    "enabled",
			vtui.SemanticID(controller.variablesLabel): "variables-label",
			vtui.SemanticID(controller.variablesEdit):  "variables",
			vtui.SemanticID(controller.addButton):      "add",
			vtui.SemanticID(controller.saveButton):     "save",
			vtui.SemanticID(controller.cancelButton):   "cancel",
		}
		for _, child := range semantic.AppMapSlice(node["children"]) {
			child["layoutRole"] = roles[semantic.String(child["id"])]
			if child["id"] == vtui.SemanticID(window.controller.list) {
				window.profileListPresentation(child)
			}
		}
	}
	return node
}

func (window *managerWindow) profileListPresentation(node map[string]any) {
	items := semantic.AppStringSlice(node["items"])
	states := make([]map[string]any, len(items))
	for index := range items {
		checkable, checked, dimmed := false, false, false
		if index < len(window.controller.config.Entries) {
			entry := window.controller.config.Entries[index]
			checkable = entry.Kind == KindProfile
			checked = checkable && entry.Enabled
			dimmed = !checked
			if checkable {
				items[index], _ = profileMenuLabel(entry.Name)
			}
		}
		states[index] = map[string]any{"checkable": checkable, "checked": checked, "dimmed": dimmed}
	}
	node["items"], node["itemStates"] = items, states
}

func (window *managerWindow) HandleSemanticAction(action map[string]any) bool {
	controller := window.controller
	if action["target"] == vtui.SemanticID(window.Window) && window.handleFooterAction(semantic.String(action["action"])) {
		return true
	}
	if controller != nil && action["target"] == vtui.SemanticID(controller.list) &&
		action["action"] == "control.toggle" {
		if controller.editing || controller.list.IsDisabled() || action["index"] == nil {
			return false
		}
		index := semantic.Int(action["index"])
		if index < 0 || index >= len(controller.config.Entries) ||
			controller.config.Entries[index].Kind != KindProfile {
			return false
		}
		vtui.DebugLog("[FIX:envman-checkbox] toggle row=%d", index)
		window.SetFocusedItem(controller.list)
		return controller.toggle(index, true, false)
	}
	return window.Window.HandleSemanticAction(action)
}
