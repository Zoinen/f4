package editor

import "strings"

// shiftSelectedLines changes the indentation of every non-empty line touched
// by a multi-line selection. A single-line selection keeps the ordinary Tab
// behaviour, where typing replaces the selected text.
func (ev *EditorView) shiftSelectedLines(left bool) bool {
	if !ev.SelActive && !ev.RectSelActive {
		return false
	}

	first, last := ev.selectedLineSpan()
	if last <= first {
		return false
	}
	if first < 0 {
		first = 0
	}
	ev.EnsureIndexedToLine(last + 1)
	if last >= ev.Li.LineCount() {
		last = ev.Li.LineCount() - 1
	}
	if last < first {
		return false
	}

	tabSize := ev.TabSize
	if tabSize <= 0 {
		tabSize = 8
	}

	type edit struct {
		off int
		del int
		ins []byte
	}
	edits := make([]edit, 0, last-first+1)
	for line := last; line >= first; line-- {
		lineStart := ev.Li.GetLineOffset(line)
		lineLen := ev.GetLineLength(line)
		if lineLen == 0 {
			continue
		}

		if left {
			prefixLen := 0
			prefix, _ := ev.Pt.GetRange(lineStart, min(lineLen, tabSize))
			if len(prefix) > 0 && prefix[0] == '\t' {
				prefixLen = 1
			} else {
				for prefixLen < len(prefix) && prefixLen < tabSize && prefix[prefixLen] == ' ' {
					prefixLen++
				}
			}
			if prefixLen > 0 {
				edits = append(edits, edit{off: lineStart, del: prefixLen})
			}
			continue
		}

		indent := []byte("\t")
		if ev.ExpandTabs > 0 {
			indent = []byte(strings.Repeat(" ", tabSize))
		}
		edits = append(edits, edit{off: lineStart, ins: indent})
	}

	// A block operation must keep the cursor and selection attached to the
	// same text. Positions exactly at a line start stay there so the new
	// indentation remains part of the selected block; positions inside the
	// line move with its text.
	shiftOffset := func(offset, off, del, ins int) int {
		if del > 0 {
			if offset <= off {
				return offset
			}
			if offset >= off+del {
				return offset - del
			}
			return off
		}
		if offset <= off {
			return offset
		}
		return offset + ins
	}

	cursorOffset := ev.caretOffset()
	anchorOffset := ev.SelAnchorOffset
	if len(edits) > 0 {
		ev.noteBufferEdit()
		ev.saveUndo(opOther)
		ev.Modified = true
		for _, item := range edits {
			if item.del > 0 {
				ev.Pt.Delete(item.off, item.del)
				ev.Li.UpdateAfterDelete(item.off, item.del)
			} else {
				ev.Pt.Insert(item.off, item.ins)
				ev.Li.UpdateAfterInsert(item.off, item.ins)
			}
			cursorOffset = shiftOffset(cursorOffset, item.off, item.del, len(item.ins))
			if ev.SelActive {
				anchorOffset = shiftOffset(anchorOffset, item.off, item.del, len(item.ins))
			}
		}

		if cursorOffset < 0 {
			cursorOffset = 0
		}
		if cursorOffset > ev.Pt.Size() {
			cursorOffset = ev.Pt.Size()
		}
		ev.CursorLine = ev.Li.GetLineAtOffset(cursorOffset)
		ev.CursorPos = cursorOffset - ev.Li.GetLineOffset(ev.CursorLine)
		if ev.SelActive {
			if anchorOffset < 0 {
				anchorOffset = 0
			}
			if anchorOffset > ev.Pt.Size() {
				anchorOffset = ev.Pt.Size()
			}
			ev.SelAnchorOffset = anchorOffset
		}
		ev.invalidateStates(first)
		ev.Engine.InvalidateFrom(first)
		ev.updateDesiredVisualCol()
		ev.EnsureCursorVisible()
	}

	// Consume Tab even when all selected lines are empty or already have no
	// indentation to remove: the selection must never fall through to the
	// single-caret replacement path.
	return true
}
