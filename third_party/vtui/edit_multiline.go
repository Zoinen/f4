package vtui

import (
	"strings"
	"unicode"
)

// editLine offsets are rune positions in the unchanged input buffer.
type editLine struct {
	start, end int
	soft       bool
}

// MultilineBlockSelectionGeometry contains visual cell coordinates supplied
// by a native text layout and the number of cells available for wrapping.
type MultilineBlockSelectionGeometry struct {
	AnchorRow, AnchorColumn int
	FocusRow, FocusColumn   int
	WrapWidth               int
}

func (e *Edit) multilineLines(width int) []editLine {
	text := string(e.text)
	if e.multilineCache != nil && e.multilineCacheText == text && e.multilineCacheWidth == width && e.multilineCacheWordWrap == e.WordWrap {
		return e.multilineCache
	}
	e.multilineCacheText, e.multilineCacheWidth, e.multilineCacheWordWrap = text, width, e.WordWrap
	width = max(1, width-1) // Reserve a cell for the caret or visual wrap marker.
	type cluster struct {
		start, end, width int
		space, newline    bool
		argument          bool
	}
	var clusters []cluster
	argumentStarts := make(map[int]bool)
	var quote rune
	for pos, r := range e.text {
		if (r == '\'' || r == '"') && (pos == 0 || e.text[pos-1] != '\\') {
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
		}
		if e.WordWrap && quote == 0 && r == '-' && pos > 0 && unicode.IsSpace(e.text[pos-1]) && e.text[pos-1] != '\n' {
			argumentStarts[pos] = true
		}
	}
	offset := 0
	paragraphs := strings.Split(text, "\n")
	for index, paragraph := range paragraphs {
		forEachTerminalCluster(paragraph, func(s string, w, _, pos int) {
			runes := []rune(s)
			clusters = append(clusters, cluster{offset + pos, offset + pos + len(runes), w, unicode.IsSpace(runes[0]), false, argumentStarts[offset+pos]})
		})
		offset += len([]rune(paragraph))
		if index < len(paragraphs)-1 {
			clusters = append(clusters, cluster{offset, offset + 1, 0, false, true, false})
			offset++
		}
	}
	var lines []editLine
	start, i, cells, boundary := 0, 0, 0, -1
	for i < len(clusters) {
		c := clusters[i]
		if c.argument && i > start {
			lines = append(lines, editLine{clusters[start].start, c.start, true})
			start, cells, boundary = i, 0, -1
		}
		if c.newline {
			lines = append(lines, editLine{clusters[start].start, c.start, false})
			i++
			start, cells, boundary = i, 0, -1
			continue
		}
		if cells+c.width > width && i > start {
			end := i
			if e.WordWrap && boundary > start {
				end = boundary
			}
			lines = append(lines, editLine{clusters[start].start, clusters[end].start, true})
			start, i, cells, boundary = end, end, 0, -1
			continue
		}
		cells += c.width
		i++
		if c.space {
			boundary = i
		}
	}
	pos := len(e.text)
	if start < len(clusters) {
		pos = clusters[start].start
	}
	e.multilineCache = append(lines, editLine{pos, len(e.text), false})
	return e.multilineCache
}

// MultilineRows measures the input without changing its text or caret.
func (e *Edit) MultilineRows(width int) int {
	if !e.Multiline {
		return 1
	}
	return len(e.multilineLines(width))
}

// MoveCursorVertical moves within displayed lines, retaining the preferred
// column across shorter rows. False means the caret is already at the edge.
func (e *Edit) MoveCursorVertical(direction int) bool {
	return e.MoveCursorVerticalSelection(direction, false)
}

func (e *Edit) MoveCursorVerticalSelection(direction int, extend bool) bool {
	if !e.Multiline || e.IsDisabled() || direction == 0 {
		return false
	}
	if e.blockSelection {
		// Once the caret is moved with the keyboard, subsequent Shift
		// navigation is linear rather than silently retaining an old block.
		e.ClearSelection()
	}
	lines := e.multilineLines(e.X2 - e.X1 + 1)
	row, column := e.multilineCaret(lines)
	next := max(0, min(len(lines)-1, row+direction))
	if next == row {
		e.multilineMoveActive = false
		return false
	}
	text := e.multilineCacheText
	if e.multilineMoveActive && e.multilineMoveCursor == e.curPos && e.multilineMoveText == text {
		column = e.multilineMoveColumn
	}
	line := lines[next]
	position, cells, found := line.end, 0, false
	forEachTerminalCluster(string(e.text[line.start:line.end]), func(s string, width, _, offset int) {
		if !found && cells+width > column {
			position, found = line.start+offset, true
		}
		cells += width
	})
	if line.soft && position == line.end && position > line.start {
		position = e.prevClusterBoundary(position)
	}
	if extend {
		e.beginSelection()
	} else {
		e.ClearSelection()
	}
	e.curPos = position
	if extend {
		e.endSelection()
	}
	e.multilineMoveText, e.multilineMoveCursor = text, position
	e.multilineMoveColumn, e.multilineMoveActive = column, true
	e.ScreenObject.NotifyChange()
	DebugLog("[FIX:command-line-navigation] row=%d target=%d column=%d", row, next, column)
	return true
}

func (e *Edit) multilineCaret(lines []editLine) (int, int) {
	for row, line := range lines {
		if e.curPos <= line.end && (!line.soft || e.curPos < line.end) {
			return row, StringWidth(string(e.text[line.start:max(line.start, e.curPos)]))
		}
	}
	return len(lines) - 1, 0
}

func (e *Edit) cursorPositionAtPoint(x, y int) int {
	if !e.Multiline {
		return e.cursorPositionAtX(x)
	}
	lines := e.multilineLines(e.X2 - e.X1 + 1)
	line := lines[max(0, min(len(lines)-1, y-e.Y1+e.multilineTop))]
	column, result, found := 0, line.end, false
	forEachTerminalCluster(string(e.text[line.start:line.end]), func(s string, w, _, pos int) {
		if !found && column+w > x-e.X1 {
			result, found = line.start+pos, true
		}
		column += w
	})
	return result
}

// SetMultilineBlockSelection selects the visual rectangle between two rune
// offsets. The offsets stay in the shared edit buffer; row and column are
// derived from the same wrapped-line cache used for painting and navigation.
func (e *Edit) SetMultilineBlockSelection(anchor, cursor int) {
	if !e.Multiline {
		e.HandleSemanticAction(map[string]any{"action": "select", "anchor": anchor, "cursor": cursor})
		return
	}
	wasBlockSelection := e.blockSelection
	anchor = e.semanticCursor(anchor)
	cursor = e.semanticCursor(cursor)
	lines := e.multilineLines(e.X2 - e.X1 + 1)
	anchorRow, anchorColumn := e.multilinePosition(lines, anchor)
	focusRow, focusColumn := e.multilinePosition(lines, cursor)
	e.ClearSelection()
	e.setMultilineBlockSelection(anchor, cursor, anchorRow, anchorColumn, focusRow, focusColumn, 0)
	if !wasBlockSelection {
		DebugLog("[FIX:text-block-selection] multiline start rows=%d:%d columns=%d:%d",
			anchorRow, focusRow, anchorColumn, focusColumn)
	}
	e.ScreenObject.NotifyChange()
}

// SetMultilineBlockSelectionAt uses visual coordinates from the native layout
// while retaining source offsets in the shared edit buffer.
func (e *Edit) SetMultilineBlockSelectionAt(anchor, cursor int, geometry MultilineBlockSelectionGeometry) {
	if !e.Multiline {
		e.HandleSemanticAction(map[string]any{"action": "select", "anchor": anchor, "cursor": cursor})
		return
	}
	wasBlockSelection := e.blockSelection
	anchor = e.semanticCursor(anchor)
	cursor = e.semanticCursor(cursor)
	e.ClearSelection()
	e.setMultilineBlockSelection(anchor, cursor,
		max(0, geometry.AnchorRow), max(0, geometry.AnchorColumn),
		max(0, geometry.FocusRow), max(0, geometry.FocusColumn),
		max(0, geometry.WrapWidth))
	if !wasBlockSelection {
		DebugLog("[FIX:text-block-selection] native rows=%d:%d columns=%d:%d width=%d",
			geometry.AnchorRow, geometry.FocusRow, geometry.AnchorColumn,
			geometry.FocusColumn, geometry.WrapWidth)
	}
	e.ScreenObject.NotifyChange()
}

// MultilineBlockSelection returns the block's visual row and cell endpoints.
func (e *Edit) MultilineBlockSelection() (active bool, anchorRow, anchorColumn, focusRow, focusColumn int) {
	return e.blockSelection, e.blockAnchorRow, e.blockAnchorColumn,
		e.blockFocusRow, e.blockFocusColumn
}

func (e *Edit) setMultilineBlockSelection(anchor, cursor, anchorRow, anchorColumn, focusRow, focusColumn, wrapWidth int) {
	e.blockAnchorRow, e.blockAnchorColumn = anchorRow, anchorColumn
	e.blockFocusRow, e.blockFocusColumn = focusRow, focusColumn
	e.blockWrapWidth = wrapWidth
	e.blockSelection = true
	e.selAnchor, e.curPos = anchor, cursor
	e.selStart, e.selEnd = min(anchor, cursor), max(anchor, cursor)
	e.clearFlag = false
	e.multilineMoveActive = false
}

func (e *Edit) multilineBlockLines() []editLine {
	if e.blockWrapWidth > 0 {
		return e.multilineLines(e.blockWrapWidth + 1)
	}
	return e.multilineLines(e.X2 - e.X1 + 1)
}

func (e *Edit) multilinePosition(lines []editLine, offset int) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	offset = max(0, min(offset, len(e.text)))
	for row, line := range lines {
		if offset < line.end || (offset == line.end && (!line.soft || row == len(lines)-1)) {
			end := max(line.start, min(offset, line.end))
			return row, StringWidth(string(e.text[line.start:end]))
		}
	}
	last := lines[len(lines)-1]
	return len(lines) - 1, StringWidth(string(e.text[last.start:last.end]))
}

func (e *Edit) multilineBlockBounds() (top, bottom, left, right int) {
	top, bottom = e.blockAnchorRow, e.blockFocusRow
	left, right = e.blockAnchorColumn, e.blockFocusColumn
	if top > bottom {
		top, bottom = bottom, top
	}
	if left > right {
		left, right = right, left
	}
	return top, bottom, left, right
}

func (e *Edit) multilineBlockText() string {
	if !e.blockSelection || !e.Multiline {
		return ""
	}
	top, bottom, left, right := e.multilineBlockBounds()
	if right <= left {
		return ""
	}
	lines := e.multilineBlockLines()
	top = max(0, min(top, len(lines)-1))
	bottom = max(top, min(bottom, len(lines)-1))
	var out strings.Builder
	for row := top; row <= bottom; row++ {
		if row > top {
			out.WriteByte('\n')
		}
		line := lines[row]
		column := 0
		written := 0
		forEachTerminalCluster(string(e.text[line.start:line.end]), func(cluster string, width, _, _ int) {
			start, end := column, column+width
			if end > left && start < right {
				if start < left {
					out.WriteString(strings.Repeat(" ", min(end, right)-left))
					written += min(end, right) - left
				} else if end > right {
					out.WriteString(strings.Repeat(" ", right-start))
					written += right - start
				} else {
					out.WriteString(cluster)
					written += width
				}
			}
			column = end
		})
		if written < right-left {
			out.WriteString(strings.Repeat(" ", right-left-written))
		}
	}
	return out.String()
}

func (e *Edit) deleteMultilineBlock() {
	top, bottom, left, right := e.multilineBlockBounds()
	if right <= left {
		e.ClearSelection()
		return
	}
	lines := e.multilineBlockLines()
	top = max(0, min(top, len(lines)-1))
	bottom = max(top, min(bottom, len(lines)-1))
	type runeRange struct{ start, end int }
	ranges := make([]runeRange, 0, bottom-top+1)
	for row := top; row <= bottom; row++ {
		line := lines[row]
		column := 0
		start, end := -1, -1
		forEachTerminalCluster(string(e.text[line.start:line.end]), func(cluster string, width, _, offset int) {
			clusterStart, clusterEnd := column, column+width
			if clusterEnd > left && clusterStart < right {
				if start < 0 {
					start = line.start + offset
				}
				end = line.start + offset + len([]rune(cluster))
			}
			column = clusterEnd
		})
		if start >= 0 && end > start {
			ranges = append(ranges, runeRange{start, end})
		}
	}
	if len(ranges) == 0 {
		e.ClearSelection()
		return
	}
	caret := ranges[0].start
	for i := len(ranges) - 1; i >= 0; i-- {
		r := ranges[i]
		e.text = append(e.text[:r.start], e.text[r.end:]...)
	}
	e.curPos = min(caret, len(e.text))
	e.ClearSelection()
	if e.OnTextChange != nil {
		e.OnTextChange(string(e.text))
	}
	e.NotifyChange()
}

func (e *Edit) showMultiline(scr *ScreenBuf) {
	lines := e.multilineBlockLines()
	row, column := e.multilineCaret(lines)
	height := max(1, e.Y2-e.Y1+1)
	e.multilineTop = max(0, min(e.multilineTop, len(lines)-height))
	if row < e.multilineTop {
		e.multilineTop = row
	}
	if row >= e.multilineTop+height {
		e.multilineTop = row - height + 1
	}
	attr := e.GetStateAttr(e.ColorTextIdx, e.ColorTextIdx)
	scr.FillRect(e.X1, e.Y1, e.X2, e.Y2, ' ', attr)
	for y := 0; y < height && y+e.multilineTop < len(lines); y++ {
		line := lines[y+e.multilineTop]
		visualRow := y + e.multilineTop
		blockTop, blockBottom, blockLeft, blockRight := e.multilineBlockBounds()
		if e.blockSelection && visualRow >= blockTop && visualRow <= blockBottom && blockRight > blockLeft {
			left := min(e.X2+1, e.X1+blockLeft)
			right := min(e.X2+1, e.X1+blockRight)
			if right > left {
				scr.FillRect(left, e.Y1+y, right-1, e.Y1+y, ' ', Palette[e.ColorSelectedIdx])
			}
		}
		x := e.X1
		column := 0
		forEachTerminalCluster(string(e.text[line.start:line.end]), func(s string, w, _, pos int) {
			color := attr
			if e.WordWrap && pos == 0 && s == "-" && y+e.multilineTop > 0 && lines[y+e.multilineTop-1].soft {
				color = Palette[e.WrapMarkerColorIdx]
			}
			selected := e.selStart >= 0 && pos+line.start >= e.selStart && pos+line.start < e.selEnd
			if e.blockSelection {
				selected = visualRow >= blockTop && visualRow <= blockBottom && column+w > blockLeft && column < blockRight
			}
			if selected {
				color = Palette[e.ColorSelectedIdx]
			}
			if x+w <= e.X2+1 {
				scr.Write(x, e.Y1+y, StringToCharInfo(s, color))
			}
			x += w
			column += w
		})
		if line.soft {
			scr.Write(e.X2, e.Y1+y, StringToCharInfo("-", Palette[e.WrapMarkerColorIdx]))
		}
	}
	if e.IsFocused() && !e.HideCursor {
		scr.SetCursorVisible(true)
		shape := CursorShapeUnderline
		if e.overtype {
			shape = CursorShapeBlock
		}
		scr.SetCursorShape(shape)
		scr.SetCursorPos(min(e.X2, e.X1+column), e.Y1+row-e.multilineTop)
	}
}
