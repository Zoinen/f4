package semantic

import (
	"reflect"
)

var SemanticEditorSurfaceStateKeys = []string{
	"cursorLine",
	"cursorPos",
	"cursorVisualRow",
	"cursorVisualColumn",
	"cursorVisible",
	"cursorShape",
	"cursorAbsoluteRow",
	"cursorAbsoluteColumn",
	"selection",
	"selectionAnchorRow",
	"selectionAnchorColumn",
	"selectionForeground",
	"selectionBackground",
	"selectionBold",
	"selectionUnderline",
	"selectionStrikeout",
	"secondaryCarets",
	"topBarRight",
}

var SemanticEditorSurfaceIdentityKeys = []string{
	"documentKey",
	"layoutRevision",
	"windowGeneration",
}

func SemanticEditorSurfaceStateValid(state map[string]any) bool {
	integer := func(value any, nonNegative bool) bool {
		if value == nil {
			return false
		}
		reflected := reflect.ValueOf(value)
		switch reflected.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return !nonNegative || reflected.Int() >= 0
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return reflected.Uint() <= uint64(^uint64(0)>>1)
		default:
			return false
		}
	}
	// These are identity preconditions, not mutable cursor/selection fields.
	// Carry all three on every compact update because QML replaces its compact
	// override atomically instead of merging it with the previous one.
	expected := len(SemanticEditorSurfaceStateKeys) + len(SemanticEditorSurfaceIdentityKeys)
	if _, present := state["secondaryCarets"]; !present {
		expected--
	}
	if len(state) != expected {
		return false
	}
	for _, key := range SemanticEditorSurfaceIdentityKeys {
		value, present := state[key]
		if !present {
			return false
		}
		if key == "documentKey" {
			text, ok := value.(string)
			if !ok || text == "" {
				return false
			}
		} else if !integer(value, true) {
			return false
		}
	}
	for _, key := range SemanticEditorSurfaceStateKeys {
		value, present := state[key]
		if !present && key == "secondaryCarets" {
			continue
		}
		if !present {
			return false
		}
		switch key {
		case "secondaryCarets":
			carets, ok := value.([]map[string]any)
			if !ok {
				return false
			}
			for _, caret := range carets {
				if len(caret) != 5 {
					return false
				}
				if _, ok := caret["selection"].(bool); !ok {
					return false
				}
				for _, coordinate := range []string{"cursorAbsoluteRow", "cursorAbsoluteColumn", "selectionAnchorRow", "selectionAnchorColumn"} {
					if !integer(caret[coordinate], true) {
						return false
					}
				}
			}

		case "cursorVisible", "selection", "selectionBold",
			"selectionUnderline", "selectionStrikeout":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "cursorShape":
			shape, ok := value.(string)
			if !ok || (shape != "underline" && shape != "block") {
				return false
			}
		case "topBarRight", "selectionForeground", "selectionBackground":
			if _, ok := value.(string); !ok {
				return false
			}
		case "cursorLine", "cursorPos", "cursorAbsoluteRow",
			"cursorAbsoluteColumn", "selectionAnchorRow", "selectionAnchorColumn":
			if !integer(value, true) {
				return false
			}
		default:
			if !integer(value, false) {
				return false
			}
		}
	}
	return true
}

func SemanticMapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			result = append(result, entry)
		}
		return result, true
	default:
		return nil, false
	}
}

func SemanticScenePanelMaps(scene map[string]any) ([]map[string]any, bool) {
	if scene == nil {
		return nil, false
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || shell == nil {
		return nil, false
	}
	switch panels := shell["panels"].(type) {
	case []map[string]any:
		return panels, len(panels) > 0
	case []any:
		result := make([]map[string]any, 0, len(panels))
		for _, value := range panels {
			panel, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			result = append(result, panel)
		}
		return result, len(result) > 0
	default:
		return nil, false
	}
}

func SemanticShallowMapCopy(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
