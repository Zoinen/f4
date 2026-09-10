package semantic

func SemanticWrappedRowSpan(wraps []bool, index, count int) (int, int) {
	if index < 0 || index >= count {
		return index, index + 1
	}
	start := index
	for start > 0 && start-1 < len(wraps) && wraps[start-1] {
		start--
	}
	end := index + 1
	for end < count && end-1 < len(wraps) && wraps[end-1] {
		end++
	}
	return start, end
}
