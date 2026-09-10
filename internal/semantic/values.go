package semantic

func AppMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func AppMapSlice(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m := AppMap(item); m != nil {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func AppStringSlice(value any) []string {
	switch items := value.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			out = append(out, String(item))
		}
		return out
	default:
		return nil
	}
}

func AppIntSlice(value any) []int {
	switch items := value.(type) {
	case []int:
		return items
	case []any:
		out := make([]int, 0, len(items))
		for _, item := range items {
			out = append(out, Int(item))
		}
		return out
	default:
		return nil
	}
}

func AppBool(value any) bool {
	b, _ := value.(bool)
	return b
}

func AppBoolDefault(value any, fallback bool) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

func AppInt64(value any) int64 {
	switch n := value.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		if n > uint64(^uint(0)>>1) {
			return 0
		}
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
