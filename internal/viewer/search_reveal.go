package viewer

// SelectSearchMatch reveals an offscreen result while preserving the position
// of results already visible in the current visual rows.
func (vv *ViewerView) SelectSearchMatch(offset int64, length int) {
	if !vv.searchMatchVisible(offset, length) {
		vv.beginViewerNavigationIntent()
		if vv.HexMode {
			vv.TopOffset = offset &^ int64(15)
		} else if vv.DecodeMode {
			vv.TopOffset = offset
		} else {
			vv.TopOffset = offset
			vv.SemanticNeedsReflow = true
			if resolved, ready := vv.semanticResolveTextWindowOffset(offset); ready {
				vv.TopOffset = resolved
				vv.SemanticNeedsReflow = false
			}
		}
	}
	vv.LastSearchOffset = offset
	vv.LastSearchTopOffset = vv.TopOffset
	vv.LastSearchMatchLen = int64(length)
	vv.LastSearchFound = true
}

func (vv *ViewerView) searchMatchVisible(offset int64, length int) bool {
	if length <= 0 || offset < vv.TopOffset {
		return false
	}
	width, height := vv.viewportWidth(), vv.viewportHeight()
	if width <= 0 || height <= 0 {
		return false
	}
	if vv.HexMode {
		return offset+int64(length) <= vv.TopOffset+int64(height*16)
	}
	current := vv.TopOffset
	column, err := vv.viewerRowOrigin(current, width)
	if err != nil {
		return false
	}
	remaining := offset
	end := offset + int64(length)
	for y := 0; y < height && current < vv.Backend.Size(); y++ {
		row, err := vv.projectRowAt(current, width, column)
		if err != nil || row.end <= current {
			return false
		}
		visibleEnd := row.end
		if !vv.DecodeMode {
			visibleEnd = current + int64(len(row.text))
		}
		if remaining >= current && remaining < visibleEnd {
			if end <= visibleEnd {
				return true
			}
			// A hidden suffix of an unwrapped line cannot count as visible.
			if visibleEnd < row.end {
				return false
			}
			remaining = row.end
		}
		current, column = row.end, row.nextColumn
	}
	return false
}
