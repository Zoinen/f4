package editor

func semanticValueContainsKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for candidate, nested := range typed {
			if candidate == key || semanticValueContainsKey(nested, key) {
				return true
			}
		}
	case []map[string]any:
		for _, nested := range typed {
			if semanticValueContainsKey(nested, key) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if semanticValueContainsKey(nested, key) {
				return true
			}
		}
	}
	return false
}
