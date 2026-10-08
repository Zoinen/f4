package mediainfo

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// SemanticNode presents the custom terminal report through the native list
// renderer. Export every line so scrolling is not limited to the TUI viewport.
func (view *reportTextView) SemanticNode(_ *vtui.SemanticContext) map[string]any {
	x1, y1, x2, y2 := view.GetPosition()
	items := make([]string, len(view.lines))
	fields := make([]map[string]any, len(view.lines))
	fieldWidth := 0
	for index, line := range view.lines {
		items[index] = line.text
		fields[index] = map[string]any{"name": line.fieldName, "value": line.value}
		fieldWidth = max(fieldWidth, line.fieldColumn)
	}
	return map[string]any{
		"id": vtui.SemanticID(view), "kind": "listBox",
		"x": x1, "y": y1, "w": x2 - x1 + 1, "h": y2 - y1 + 1,
		"visible": view.IsVisible(), "focused": view.IsFocused(),
		"disabled": view.IsDisabled(), "items": items,
		"itemFields": fields, "fieldColumnWidth": fieldWidth,
		"cursor": view.SelectPos, "top": view.TopPos,
		// Plain report text must preserve ampersands, markup and blank lines.
		"wrapText": true,
	}
}

func (view *reportTextView) HandleSemanticAction(action map[string]any) bool {
	if view.IsDisabled() {
		return false
	}
	switch semantic.String(action["action"]) {
	case "focus", "control.focus":
		view.SetFocus(true)
		return true
	case "select", "control.select":
		if action["index"] == nil {
			return false
		}
		index := semantic.Int(action["index"])
		if index < 0 || index >= len(view.lines) {
			return false
		}
		view.SetSelectPos(index)
		return true
	}
	return false
}
