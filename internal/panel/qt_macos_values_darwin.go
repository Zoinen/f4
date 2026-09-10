//go:build darwin

package panel

import "fmt"

func platformMessageMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = item
		}
		return out
	default:
		return nil
	}
}

func platformAnyString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func platformAnyBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int8:
		return typed != 0
	case int64:
		return typed != 0
	case uint64:
		return typed != 0
	default:
		return false
	}
}
