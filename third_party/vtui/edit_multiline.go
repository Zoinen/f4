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

func (e *Edit) showMultiline(scr *ScreenBuf) {
	lines := e.multilineLines(e.X2 - e.X1 + 1)
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
		x := e.X1
		forEachTerminalCluster(string(e.text[line.start:line.end]), func(s string, w, _, pos int) {
			color := attr
			if e.WordWrap && pos == 0 && s == "-" && y+e.multilineTop > 0 && lines[y+e.multilineTop-1].soft {
				color = Palette[e.WrapMarkerColorIdx]
			}
			if e.selStart >= 0 && pos+line.start >= e.selStart && pos+line.start < e.selEnd {
				color = Palette[e.ColorSelectedIdx]
			}
			if x+w <= e.X2+1 {
				scr.Write(x, e.Y1+y, StringToCharInfo(s, color))
			}
			x += w
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
