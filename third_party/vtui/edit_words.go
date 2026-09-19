package vtui

// Word deletion follows the same logical boundaries as Ctrl+Left/Right,
// including under full bidi rendering, and never splits a terminal cluster.
func (e *Edit) deleteWordLeft() {
	end := e.curPos
	start := e.prevClusterBoundary(end)
	for start > 0 {
		if stopBeforeRuneLeft(e.text[start-1], e.text[start], false) {
			break
		}
		start = e.prevClusterBoundary(start)
	}
	e.text = append(e.text[:start], e.text[end:]...)
	e.curPos = start
	DebugLog("[FIX:edit-word-delete] left start=%d end=%d", start, end)
}

func (e *Edit) deleteWordRight() {
	start := e.curPos
	end := e.nextClusterBoundary(start)
	for end < len(e.text) {
		if stopBeforeRuneRight(e.text[end-1], e.text[end], false) {
			break
		}
		end = e.nextClusterBoundary(end)
	}
	e.text = append(e.text[:start], e.text[end:]...)
	DebugLog("[FIX:edit-word-delete] right start=%d end=%d", start, end)
}
