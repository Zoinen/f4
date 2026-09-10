package editor

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/textlayout"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
	"unicode/utf8"
)

type editorTextProjectionStyle struct {
	background, selected           uint64
	paintStreamSelection           bool
	cursorRow, cursorColumn        int
	crossRow, crossColumn          int
	horizontalCross, verticalCross uint64
}

type editorProjectedTextRow struct {
	logicalLine, visualRow int
	fragment               textlayout.LineFragment
	cells                  []vtui.CharInfo
	err                    error
}

func (ev *EditorView) textProjectionStyle() editorTextProjectionStyle {
	cursorOffset := ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	cursorRow, cursorColumn := ev.Engine.LogicalToVisual(cursorOffset)
	style := editorTextProjectionStyle{
		background: ColorerEditorBaseAttr(vtui.Palette[theme.ColEditorText]),
		selected:   vtui.Palette[vtui.ColDialogEditSelected],
		cursorRow:  cursorRow, cursorColumn: cursorColumn,
		crossRow: -1, crossColumn: -1,
	}
	if showHorz, showVert, hAttr, vAttr := EditorCrossAttrs(); ev.IsFocused() {
		if showHorz {
			style.crossRow, style.horizontalCross = cursorRow, hAttr
		}
		if showVert {
			style.crossColumn, style.verticalCross = cursorColumn+ev.CursorVirtualSpaces, vAttr
		}
	}
	return style
}

// projectTextRows is the shared console/native text projection. Source spans,
// cell attributes and navigation coordinates are emitted together. The callback
// consumes cells immediately; the existing visible-span scratch storage is reused.
func (ev *EditorView) projectTextRows(startVisualRow, width, height int,
	style editorTextProjectionStyle, emit func(editorProjectedTextRow)) {
	if width <= 0 || height <= 0 || emit == nil {
		return
	}
	occNeedle := ev.occurrenceNeedle()
	ev.occSpans = ev.occSpans[:0]
	ev.extraSelSpans = ev.extraSelSpans[:0]
	type projectedCaret struct{ row, column int }
	carets := make([]projectedCaret, 0, len(ev.extraCursors))
	for _, caret := range ev.extraCursors {
		if !style.paintStreamSelection {
			break
		}
		if start, end := caret.selRange(); end > start {
			ev.extraSelSpans = append(ev.extraSelSpans, matchSpan{Off: start, Len: end - start})
		}
		if caret.off >= 0 && caret.off <= ev.Pt.Size() {
			row, column := ev.Engine.LogicalToVisual(caret.off)
			carets = append(carets, projectedCaret{row, column - ev.ScrollLeft})
		}
	}
	bgAttr, selAttr := style.background, style.selected
	crossVRow, crossVCol := style.crossRow, style.crossColumn
	horzCrossAttr, vertCrossAttr := style.horizontalCross, style.verticalCross
	autocompleteRow, autocompleteColumn := -1, 0
	var autocompleteCells []vtui.CharInfo
	if ev.acEnabled && len(ev.acMatches) > 0 && ev.IsFocused() && !ev.pasting {
		match := ev.acMatches[ev.acCurrentIdx]
		if len(match) > len(ev.acPrefix) {
			tail := match[len(ev.acPrefix):]
			column := style.cursorColumn - ev.ScrollLeft
			if maxLen := width - column - 1; column >= 0 && maxLen > 0 {
				if len([]rune(tail)) > maxLen {
					tail = string([]rune(tail)[:maxLen])
				}
				autocompleteRow, autocompleteColumn = style.cursorRow, column
				autocompleteCells = vtui.StringToCharInfo(tail, vtui.DimColor(vtui.Palette[theme.ColCommandLineUserScreen]))
			}
		}
	}
	startLogLine, startFragIdx := ev.Engine.GetLogLineAtVisualRow(startVisualRow)
	rowsRendered := 0

	for logIdx := startLogLine; logIdx < ev.Li.LineCount(); logIdx++ {
		lineStart := ev.Li.GetLineOffset(logIdx)
		lineLen := 0
		if logIdx+1 < ev.Li.LineCount() {
			lineLen = ev.Li.GetLineOffset(logIdx+1) - lineStart
		} else {
			lineLen = ev.Pt.Size() - lineStart
		}

		// Stateful Highlighting
		var lineSyntax []uint64
		if ch, isColorer := ev.Highlighter.(*ColorerHighlighter); isColorer {
			// Colorer is addressed by line number: its parser state cannot
			// be carried in ev.lineStates, so it keeps its own anchor near
			// the viewport instead. See HIGHLIGHT.md, phase 5.
			if text, ok := ev.lineTextForHighlight(logIdx); ok {
				lineSyntax = ch.HighlightLine(logIdx, text, bgAttr)
			}
		} else if ev.Highlighter != nil {
			// Catch up synchronously only if the uncomputed gap is small (<= 50 lines).
			// For large jumps, render unhighlighted immediately and compute in background.
			const syncHighlightGapLimit = 50
			if logIdx >= len(ev.lineStates)+syncHighlightGapLimit {
				ev.startHighlighting()

				// Allow stateless highlighters (like Chroma) to provide instant colors
				if _, isColorer := ev.Highlighter.(*ColorerHighlighter); !isColorer {
					lStart := ev.Li.GetLineOffset(logIdx)
					highlightLen := lineLen
					if highlightLen > 64*1024 {
						highlightLen = 64 * 1024
					}
					lineData, _ := ev.Pt.GetRange(lStart, highlightLen)
					lineSyntax, _ = ev.Highlighter.Highlight(string(lineData), nil, bgAttr)
				}
			} else {
				for len(ev.lineStates) <= logIdx {
					currIdx := len(ev.lineStates)
					lStart := ev.Li.GetLineOffset(currIdx)
					lEnd := ev.Pt.Size()
					if currIdx+1 < ev.Li.LineCount() {
						lEnd = ev.Li.GetLineOffset(currIdx + 1)
					}
					// Prevent highlighter from crashing on huge binary lines
					if lEnd-lStart > 64*1024 {
						lEnd = lStart + 64*1024
					}

					var prevState any
					if currIdx > 0 {
						prevState = ev.lineStates[currIdx-1]
					}

					lineData, err := ev.Pt.GetRange(lStart, lEnd-lStart)
					if err == piecetable.ErrLoading {
						break // Wait for data
					}

					attrs, nextState := ev.Highlighter.Highlight(string(lineData), prevState, bgAttr)
					ev.lineStates = append(ev.lineStates, nextState)
					if currIdx == logIdx {
						lineSyntax = attrs
					}
				}
				if logIdx < len(ev.lineStates) && lineSyntax == nil {
					// State was already cached, but we need the actual attributes for the current visible line
					lStart := ev.Li.GetLineOffset(logIdx)
					// Re-apply highlighter OOM protection for the rendering path
					highlightLen := lineLen
					if highlightLen > 64*1024 {
						highlightLen = 64 * 1024
					}
					lineData, _ := ev.Pt.GetRange(lStart, highlightLen)
					var prevState any
					if logIdx > 0 {
						prevState = ev.lineStates[logIdx-1]
					}
					lineSyntax, _ = ev.Highlighter.Highlight(string(lineData), prevState, bgAttr)
				}
			}
		}

		baseVRow := ev.Engine.GetRowOffset(logIdx)
		frags := ev.Engine.GetProjectionFragments(logIdx, startVisualRow+height-baseVRow, ev.ScrollLeft+width)
		// vtui.DebugLog("EDITOR_RENDER: Line %d, Frags: %d, BaseVRow: %d", logIdx, len(frags), baseVRow)

		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				continue
			}

			absVRow := baseVRow + fIdx

			ev.renderBytes = ev.renderBytes[:0]
			var err error
			readLength := frag.ByteOffsetEnd - frag.ByteOffsetStart
			// Every displayed rune consumes at least one cell and at most four
			// UTF-8 bytes. Bytes past this bound cannot affect the visible span.
			if columns := ev.ScrollLeft + width; columns > 0 && columns < readLength/utf8.UTFMax {
				readLength = columns * utf8.UTFMax
			}
			ev.renderBytes, err = ev.Pt.AppendRange(ev.renderBytes, frag.ByteOffsetStart, readLength)

			if err == piecetable.ErrLoading {
				ev.renderCells = ev.renderCells[:0]
				message := vtui.StringToCharInfo(" [ Loading... ] ", bgAttr)
				if ev.ScrollLeft < len(message) {
					ev.renderCells = append(ev.renderCells, message[ev.ScrollLeft:min(len(message), ev.ScrollLeft+width)]...)
				}
				for len(ev.renderCells) < width {
					ev.renderCells = append(ev.renderCells, vtui.CharInfo{Char: ' ', Attributes: bgAttr})
				}
				emit(editorProjectedTextRow{logicalLine: logIdx, visualRow: absVRow, fragment: frag, cells: ev.renderCells, err: err})
				rowsRendered++
				if rowsRendered >= height {
					return
				}
				continue
			}

			selMin, selMax := ev.GetSelectionRange()
			ev.occSpans = ev.appendOccurrenceSpans(ev.occSpans, occNeedle, frag.ByteOffsetStart, frag.ByteOffsetStart+readLength, lineStart, lineStart+lineLen)

			// Вырезаем кусок атрибутов именно для этого фрагмента
			var fragSyntax []uint64
			if frag.RuneStart < len(lineSyntax) {
				end := frag.RuneStart + utf8.RuneCount(ev.renderBytes)
				if end > len(lineSyntax) {
					end = len(lineSyntax)
				}
				fragSyntax = lineSyntax[frag.RuneStart:end]
			}

			isCrossRow := (absVRow == crossVRow)
			ev.renderCells = ev.fillCellsSpan(ev.renderCells, ev.renderBytes, bgAttr, selAttr, frag.ByteOffsetStart, style.paintStreamSelection && ev.SelActive, selMin, selMax, ev.fadeSyntax(fragSyntax, bgAttr), 0, isCrossRow, crossVCol, horzCrossAttr, vertCrossAttr, absVRow, frag.VisualColumnStart, ev.ScrollLeft, ev.ScrollLeft+width)

			lineBg := bgAttr
			if logIdx < len(ev.lineStates) {
				if ch, ok := ev.Highlighter.(*ColorerHighlighter); ok {
					lineBg = ch.GetLineBackground(logIdx, bgAttr)
				}
			}
			fillBg := lineBg
			if isCrossRow && horzCrossAttr != 0 {
				if horzCrossAttr&vtui.IsBgRGB != 0 {
					fillBg = vtui.SetRGBBack(fillBg, vtui.GetRGBBack(horzCrossAttr))
				} else {
					fillBg = vtui.SetIndexBack(fillBg, vtui.GetIndexBack(horzCrossAttr))
				}
			}
			for len(ev.renderCells) < width {
				ev.renderCells = append(ev.renderCells, vtui.CharInfo{Char: ' ', Attributes: fillBg})
			}
			if absVRow == autocompleteRow {
				copy(ev.renderCells[autocompleteColumn:], autocompleteCells)
			}
			for _, caret := range carets {
				if caret.row == absVRow && caret.column >= 0 && caret.column < len(ev.renderCells) {
					ev.renderCells[caret.column].Attributes = selAttr
				}
			}
			emit(editorProjectedTextRow{logicalLine: logIdx, visualRow: absVRow, fragment: frag, cells: ev.renderCells, err: err})

			rowsRendered++
			if rowsRendered >= height {
				return
			}
		}
	}

}
