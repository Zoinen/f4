package settings

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// Export the choice set independently of the terminal's inline/wrapped layout.
// Keyboard focus and selection remain separate, as in the console control.
func (r *settingsRadios) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	node := r.Group.SemanticNode(ctx)
	delete(node, "children")
	items := make([]string, 0, len(r.buttons))
	disabled := []int{}
	selected, focused := -1, -1
	for i, button := range r.buttons {
		items = append(items, button.label)
		if button.Selected {
			selected = i
		}
		if r.GetFocusedItem() == button {
			focused = i
		}
		if button.IsDisabled() {
			disabled = append(disabled, i)
		}
	}
	node["kind"] = "radioGroup"
	node["items"], node["selected"], node["focusIndex"] = items, selected, focused
	node["disabled"], node["disabledItems"] = r.IsDisabled(), disabled
	node["wrapText"] = true
	return node
}

func (r *settingsRadios) HandleSemanticAction(action map[string]any) bool {
	if semantic.String(action["target"]) != vtui.SemanticID(r) || r.IsDisabled() {
		return false
	}
	switch semantic.String(action["action"]) {
	case "select", "control.select":
		value, exists := action["index"]
		index := semantic.Int(value)
		if !exists || index < 0 || index >= len(r.buttons) || r.buttons[index].IsDisabled() {
			return false
		}
		r.hover = -1
		r.SetFocusedItem(r.buttons[index])
		r.buttons[index].Select()
		return true
	case "focus", "control.focus":
		r.hover = -1
		r.SetFocus(true)
		return true
	}
	return false
}
