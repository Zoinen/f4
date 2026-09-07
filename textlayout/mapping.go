package textlayout

import "sort"

// RowMapping distinguishes an authoritative source/row mapping from unfinished
// layout. Progress means another bounded UI task can continue immediately;
// otherwise source readiness (or an index update) must wake the caller.
type RowMapping struct {
	Row, Offset     int
	Ready, Progress bool
	Err             error
}

// AdvanceToVisualRow never scans more than one bounded slice, even when the
// destination is far beyond the ready layout prefix.
func (we *WrapEngine) AdvanceToVisualRow(row, maxBytes int) RowMapping {
	row = max(0, row)
	result := RowMapping{}
	remaining := max(1, maxBytes)
	we.lastReadError = nil
	for lines := 0; ; lines++ {
		known, complete := we.KnownVisualRows()
		if row < known || complete {
			line, fragment := we.GetLogLineAtVisualRow(row)
			result.Row = min(row, max(0, known-1))
			if !we.wordWrap {
				result.Offset = we.li.GetLineOffset(line)
			} else if fragments := we.fragmentCache[line].fragments; fragment < len(fragments) {
				result.Offset = fragments[fragment].ByteOffsetStart
			}
			result.Ready = true
			return result
		}
		if remaining <= 0 || lines >= 64 {
			return result
		}
		line := we.validUntil + 1
		used, progress := we.advanceLine(line, row-we.rowOffsets[line]+1, remaining, -1, -1)
		remaining -= used
		result.Progress = result.Progress || progress
		result.Err = we.lastReadError
		if !progress || result.Err != nil {
			return result
		}
	}
}

// AdvanceToOffset maps a source anchor only when its containing fragment and
// every preceding row count are ready. An unfinished fragment is never clamped
// to the last currently available row.
func (we *WrapEngine) AdvanceToOffset(offset, maxBytes int) RowMapping {
	offset = max(0, min(offset, we.pt.Size()))
	result := RowMapping{Offset: offset}
	line := we.li.GetLineAtOffset(offset)
	we.updateRowOffsets()
	if !we.wordWrap {
		result.Row, result.Ready = line, true
		return result
	}
	remaining := max(1, maxBytes)
	we.lastReadError = nil
	for count := 0; count < 64; count++ {
		we.updateRowOffsets()
		current := min(we.validUntil+1, line)
		through := -1
		if current == line {
			through = offset
		}
		used, progress := we.advanceLine(current, int(^uint(0)>>1), remaining, through, -1)
		remaining -= used
		result.Progress = result.Progress || progress
		result.Err = we.lastReadError
		we.updateRowOffsets()
		if we.validUntil >= line-1 {
			layout := &we.fragmentCache[line]
			index := sort.Search(len(layout.fragments), func(i int) bool {
				return layout.fragments[i].ByteOffsetEnd > offset
			})
			if index < len(layout.fragments) {
				result.Row, result.Ready = we.rowOffsets[line]+index, true
				return result
			}
			final := layout.pending == nil || (layout.pending.stopped && offset < layout.pending.offset)
			if final && len(layout.fragments) > 0 {
				result.Row, result.Ready = we.rowOffsets[line]+len(layout.fragments)-1, true
				return result
			}
		}
		if !progress || remaining <= 0 || result.Err != nil {
			return result
		}
	}
	return result
}
