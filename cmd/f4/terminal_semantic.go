package main

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// terminalSemanticLayout is an allocation-free index over the three canonical
// terminal stores.  It does not retain rows: an absolute visual row is mapped
// on demand to PieceTable, GridHistory, or the active grid while tv.mu is held.
type terminalSemanticLayout struct {
	pieceRows        int
	historyRows      int
	activeOffset     int
	activeRows       int
	sourceActiveRows int
	totalRows        int
	buffer           [][]vtui.CharInfo
	wraps            []bool
	altScreen        bool
}

func (tv *TerminalView) semanticPieceRowsUnsafe() int {
	if tv.pt == nil || tv.li == nil || tv.engine == nil || tv.pt.Size() == 0 {
		return 0
	}
	tv.engine.SetWidth(max(1, tv.Width))
	rows := tv.engine.GetTotalVisualRows()

	// LineIndex intentionally exposes the empty logical line after a trailing
	// newline.  GridHistory begins at that exact byte boundary, so counting the
	// synthetic empty fragment would insert a blank row between the two stores.
	tv.semanticScratch = tv.semanticScratch[:0]
	last, err := tv.pt.AppendRange(tv.semanticScratch, tv.pt.Size()-1, 1)
	if err == nil && len(last) == 1 && last[0] == '\n' && rows > 0 {
		rows--
	}
	return max(0, rows)
}

func (tv *TerminalView) semanticActiveOffsetUnsafe(buffer [][]vtui.CharInfo,
	altScreen bool,
) int {
	height := min(tv.Height, len(buffer))
	if height <= 0 || altScreen {
		return 0
	}

	lowest := 0
	for row := height - 1; row >= 0; row-- {
		if tv.rowHasText(row) {
			lowest = row
			break
		}
	}
	if tv.CursorY > lowest {
		lowest = min(tv.CursorY, height-1)
	}
	return max(0, height-1-lowest)
}

func (tv *TerminalView) semanticLayoutUnsafe() terminalSemanticLayout {
	buffer := tv.Lines
	wraps := tv.WrapFlags
	if tv.UseAltScreen {
		buffer = tv.AltLines
		wraps = nil
	}
	sourceActiveRows := min(tv.Height, len(buffer))
	bottomOverlayRows := max(0, min(tv.semanticBottomOverlayRows,
		sourceActiveRows))
	layout := terminalSemanticLayout{
		activeOffset:     tv.semanticActiveOffsetUnsafe(buffer, tv.UseAltScreen),
		activeRows:       sourceActiveRows - bottomOverlayRows,
		sourceActiveRows: sourceActiveRows,
		buffer:           buffer,
		wraps:            wraps,
		altScreen:        tv.UseAltScreen,
	}
	if !tv.UseAltScreen {
		layout.pieceRows = tv.semanticPieceRowsUnsafe()
		layout.historyRows = len(tv.GridHistory)
	}
	layout.totalRows = layout.pieceRows + layout.historyRows + layout.activeRows
	return layout
}

func terminalTrimmedCells(cells []vtui.CharInfo, width int) []vtui.CharInfo {
	end := min(len(cells), max(0, width))
	for end > 0 {
		cell := cells[end-1]
		if (cell.Char == 0 || cell.Char == ' ') && cell.Attributes == DefaultTermAttr {
			end--
			continue
		}
		break
	}
	return cells[:end]
}

func (tv *TerminalView) semanticPieceRunsUnsafe(start, end int) []extui.RunModel {
	if tv.pt == nil || start < 0 || end <= start || start >= tv.pt.Size() {
		return nil
	}
	end = min(end, tv.pt.Size())
	styleIndex := sort.Search(len(tv.styles), func(index int) bool {
		return tv.styles[index].Offset > start
	})
	attr := DefaultTermAttr
	if styleIndex > 0 {
		attr = tv.styles[styleIndex-1].Attr
	}

	runs := make([]extui.RunModel, 0, 2)
	for position := start; position < end; {
		next := end
		if styleIndex < len(tv.styles) && tv.styles[styleIndex].Offset < next {
			next = tv.styles[styleIndex].Offset
		}
		if next > position {
			tv.semanticScratch = tv.semanticScratch[:0]
			data, err := tv.pt.AppendRange(tv.semanticScratch, position, next-position)
			if err == nil && len(data) > 0 {
				runs = append(runs, semanticRunModel(string(data), attr))
			}
			position = next
		}
		for styleIndex < len(tv.styles) && tv.styles[styleIndex].Offset <= position {
			attr = tv.styles[styleIndex].Attr
			styleIndex++
		}
	}
	return runs
}

func (tv *TerminalView) semanticPieceFragmentUnsafe(visualRow int) (int, int, int, bool) {
	if visualRow < 0 || tv.engine == nil || tv.li == nil {
		return 0, 0, 0, false
	}
	logicalLine, fragmentIndex := tv.engine.GetLogLineAtVisualRow(visualRow)
	fragments := tv.engine.GetFragments(logicalLine)
	if fragmentIndex < 0 || fragmentIndex >= len(fragments) {
		return 0, 0, 0, false
	}
	fragment := fragments[fragmentIndex]
	return logicalLine, fragmentIndex, fragment.ByteOffsetStart,
		fragment.ByteOffsetEnd >= fragment.ByteOffsetStart
}

func semanticWrappedRowSpan(wraps []bool, index, count int) (int, int) {
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

// semanticLogicalRowSpanUnsafe returns the complete hard-line/paragraph span
// containing one visual terminal row. End is exclusive. This lets the native
// frontend implement a true triple-click even when the paragraph is soft-
// wrapped beyond the currently materialised QML window.
func (tv *TerminalView) semanticLogicalRowSpanUnsafe(
	layout terminalSemanticLayout, absoluteRow int,
) (start, end, logicalLine int) {
	switch {
	case absoluteRow < 0 || absoluteRow >= layout.totalRows:
		return absoluteRow, absoluteRow + 1, absoluteRow
	case absoluteRow < layout.pieceRows:
		line, _ := tv.engine.GetLogLineAtVisualRow(absoluteRow)
		fragments := tv.engine.GetFragments(line)
		start = tv.engine.GetRowOffset(line)
		return start, start + max(1, len(fragments)), line
	case absoluteRow < layout.pieceRows+layout.historyRows:
		index := absoluteRow - layout.pieceRows
		first, after := semanticWrappedRowSpan(
			tv.GridHistoryWrap, index, layout.historyRows)
		start = layout.pieceRows + first
		end = layout.pieceRows + after
		return start, end, start
	default:
		base := layout.pieceRows + layout.historyRows
		screenRow := absoluteRow - base
		index := screenRow - layout.activeOffset
		includedSourceRows := max(0, layout.activeRows-layout.activeOffset)
		if index < 0 || index >= includedSourceRows {
			return absoluteRow, absoluteRow + 1, absoluteRow
		}
		first, after := semanticWrappedRowSpan(
			layout.wraps, index, includedSourceRows)
		start = base + layout.activeOffset + first
		end = base + layout.activeOffset + after
		return start, end, start
	}
}

func (tv *TerminalView) semanticRowUnsafe(layout terminalSemanticLayout,
	absoluteRow, localIndex int,
) (extui.TextRowModel, bool) {
	row := extui.TextRowModel{
		Index:       localIndex,
		VisualRow:   absoluteRow,
		LogicalLine: absoluteRow,
		Offset:      int64(absoluteRow),
		EndOffset:   int64(absoluteRow + 1),
	}
	switch {
	case absoluteRow < 0 || absoluteRow >= layout.totalRows:
		return extui.TextRowModel{}, false
	case absoluteRow < layout.pieceRows:
		logicalLine, fragmentIndex := tv.engine.GetLogLineAtVisualRow(absoluteRow)
		row.LogicalLine = logicalLine
		fragments := tv.engine.GetFragments(logicalLine)
		if fragmentIndex < 0 || fragmentIndex >= len(fragments) {
			return extui.TextRowModel{}, false
		}
		fragment := fragments[fragmentIndex]
		row.Runs = tv.semanticPieceRunsUnsafe(fragment.ByteOffsetStart,
			fragment.ByteOffsetEnd)
	case absoluteRow < layout.pieceRows+layout.historyRows:
		index := absoluteRow - layout.pieceRows
		row.Runs = semanticRunsFromCells(terminalTrimmedCells(
			tv.GridHistory[index], tv.Width))
	default:
		screenRow := absoluteRow - layout.pieceRows - layout.historyRows
		index := screenRow - layout.activeOffset
		if index < 0 || index >= len(layout.buffer) {
			break
		}
		row.Runs = semanticRunsFromCells(terminalTrimmedCells(
			layout.buffer[index], tv.Width))
	}
	row.LogicalRowStart, row.LogicalRowEnd, row.LogicalLine =
		tv.semanticLogicalRowSpanUnsafe(layout, absoluteRow)
	row.HasLogicalRowSpan = true
	row.ContentKey = extui.TextRowContentKey(row)
	return row, true
}

func (tv *TerminalView) semanticWindowRowsUnsafe(layout terminalSemanticLayout,
	start, end int,
) []extui.TextRowModel {
	start = max(0, min(start, layout.totalRows))
	end = max(start, min(end, layout.totalRows))
	rows := make([]extui.TextRowModel, 0, end-start)
	for absoluteRow := start; absoluteRow < end; absoluteRow++ {
		if row, ok := tv.semanticRowUnsafe(layout, absoluteRow, len(rows)); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func (tv *TerminalView) semanticCursorAbsoluteRowUnsafe(layout terminalSemanticLayout) int {
	activeRow := tv.CursorY + layout.activeOffset
	activeRow = max(0, min(activeRow, max(0, layout.sourceActiveRows-1)))
	return layout.pieceRows + layout.historyRows + activeRow
}

func (tv *TerminalView) semanticModel(ctx *vtui.SemanticContext) *extui.TerminalModel {
	return tv.semanticModelWithBottomOverlay(ctx, 0)
}

// semanticModelWithBottomOverlay projects the terminal rows which remain
// visible after the console UI has painted its own bottom chrome. The classic
// renderer draws CommandLine over the terminal's final row; native frontends
// reserve a separate rectangle for it, so omitting that covered row preserves
// the same output without duplicating the idle shell prompt and cursor.
func (tv *TerminalView) semanticModelWithBottomOverlay(
	_ *vtui.SemanticContext, bottomOverlayRows int,
) *extui.TerminalModel {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	if tv.UseAltScreen {
		bottomOverlayRows = 0
	}
	tv.semanticBottomOverlayRows = max(0, bottomOverlayRows)
	layout := tv.semanticLayoutUnsafe()
	viewportRows := tv.semanticViewportRows
	if viewportRows <= 0 {
		viewportRows = max(1, tv.Height)
	}
	maxTop := max(0, layout.totalRows-viewportRows)
	if layout.altScreen {
		tv.semanticFollowTail = true
	}
	if tv.semanticFollowTail {
		tv.semanticScrollTop = maxTop
	} else {
		tv.semanticScrollTop = max(0, min(tv.semanticScrollTop, maxTop))
	}

	bufferRows := semanticWindowBufferRows(viewportRows)
	windowStart := max(0, tv.semanticScrollTop-bufferRows)
	windowEnd := min(layout.totalRows,
		tv.semanticScrollTop+viewportRows+bufferRows)
	windowRows := tv.semanticWindowRowsUnsafe(layout, windowStart, windowEnd)
	viewportRow := tv.semanticScrollTop - windowStart
	viewportSpan := min(viewportRows,
		max(0, layout.totalRows-tv.semanticScrollTop))
	visibleRows := windowRows
	visibleEnd := min(viewportRow+viewportRows, len(windowRows))
	if viewportRow >= 0 && viewportRow <= visibleEnd {
		// This is a slice view used by in-process/legacy callers only. ToMap emits
		// WindowRows exclusively, so the wire payload still contains one copy.
		visibleRows = windowRows[viewportRow:visibleEnd]
	}
	cursorAbsoluteRow := tv.semanticCursorAbsoluteRowUnsafe(layout)
	cursorVisible := tv.IsVisible() && tv.IsFocused() &&
		cursorAbsoluteRow < layout.totalRows &&
		cursorAbsoluteRow >= tv.semanticScrollTop &&
		cursorAbsoluteRow < tv.semanticScrollTop+viewportRows
	id := vtui.SemanticID(tv)

	return &extui.TerminalModel{
		ID:                 id,
		Title:              tv.Title,
		Columns:            tv.Width,
		DefaultBackground:  semanticAttrColor(DefaultTermAttr, false),
		Visible:            tv.IsVisible(),
		Focused:            tv.IsFocused(),
		AltScreen:          tv.UseAltScreen,
		Busy:               tv.Muted,
		FollowTail:         tv.semanticFollowTail,
		CursorX:            tv.CursorX,
		CursorY:            cursorAbsoluteRow - tv.semanticScrollTop,
		CursorAbsoluteRow:  int64(cursorAbsoluteRow),
		CursorVisible:      cursorVisible,
		CursorShape:        "block",
		SelectionEnabled:   !tv.UseAltScreen && tv.MouseTrackingMode == 0 && !tv.MouseSGRMode,
		DocumentKey:        id,
		ScrollAction:       "terminal.scroll",
		ScrollUnit:         "rows",
		WindowStart:        int64(windowStart),
		WindowEnd:          int64(windowEnd),
		ViewportStart:      int64(tv.semanticScrollTop),
		ViewportSpan:       int64(viewportSpan),
		ContentExtent:      int64(layout.totalRows),
		ContentExtentKnown: true,
		ViewportRow:        viewportRow,
		ViewportRows:       viewportRows,
		WindowGeneration:   tv.semanticWindowGeneration,
		Rows:               visibleRows,
		WindowRows:         windowRows,
	}
}

func (tv *TerminalView) handleSemanticAction(action map[string]any) bool {
	if tv == nil || semanticString(action["target"]) != vtui.SemanticID(tv) {
		return false
	}
	switch semanticString(action["action"]) {
	case "terminal.viewport":
		tv.mu.Lock()
		tv.semanticViewportRows = max(0, semanticInt(action["rows"]))
		tv.mu.Unlock()
		return true
	case "terminal.followTail":
		tv.mu.Lock()
		tv.semanticFollowTail = semanticBool(action["followTail"])
		if tv.semanticFollowTail {
			layout := tv.semanticLayoutUnsafe()
			viewportRows := tv.semanticViewportRows
			if viewportRows <= 0 {
				viewportRows = max(1, tv.Height)
			}
			tv.semanticScrollTop = max(0, layout.totalRows-viewportRows)
		}
		tv.mu.Unlock()
		return true
	case "terminal.scroll":
		tv.mu.Lock()
		generation, accepted := semanticAcceptWindowGeneration(action,
			tv.semanticWindowGeneration, &tv.semanticWindowRequestGeneration)
		if accepted {
			layout := tv.semanticLayoutUnsafe()
			viewportRows := tv.semanticViewportRows
			if viewportRows <= 0 {
				viewportRows = max(1, tv.Height)
			}
			maxTop := max(0, layout.totalRows-viewportRows)
			target := max(0, min(semanticInt(action["visualRow"]), maxTop))
			followTail := target >= maxTop
			if requested, present := action["followTail"]; present &&
				semanticBool(requested) {
				followTail = true
				target = maxTop
			}
			tv.semanticScrollTop = target
			tv.semanticFollowTail = followTail
			tv.semanticWindowGeneration = generation
		}
		tv.mu.Unlock()
		return true
	case "terminal.copySelection":
		var text string
		if semanticBool(action["endExclusive"]) {
			text = tv.semanticBoundarySelectionText(
				semanticInt(action["startRow"]), semanticInt(action["startColumn"]),
				semanticInt(action["endRow"]), semanticInt(action["endColumn"]))
		} else {
			text = tv.semanticSelectionText(
				semanticInt(action["startRow"]), semanticInt(action["startColumn"]),
				semanticInt(action["endRow"]), semanticInt(action["endColumn"]))
		}
		if text != "" {
			go tv.writeClipboard(text)
		}
		return true
	}
	return false
}

func terminalSelectionOrder(startRow, startColumn, endRow, endColumn int) (int, int, int, int) {
	if startRow > endRow || (startRow == endRow && startColumn > endColumn) {
		return endRow, endColumn, startRow, startColumn
	}
	return startRow, startColumn, endRow, endColumn
}

func terminalCellsSelectionRange(cells []vtui.CharInfo, width, left,
	rightExclusive int,
) string {
	left = max(0, left)
	rightExclusive = min(max(0, width), rightExclusive)
	if rightExclusive <= left {
		return ""
	}
	var out strings.Builder
	for column := left; column < rightExclusive && column < len(cells); column++ {
		cell := cells[column]
		if cell.Char == vtui.WideCharFiller {
			continue
		}
		out.WriteRune(cellRune(cell.Char))
	}
	return strings.TrimRight(out.String(), " ")
}

func terminalCellsSelection(cells []vtui.CharInfo, width, left, right int) string {
	return terminalCellsSelectionRange(cells, width, left, right+1)
}

func terminalTextSelectionRange(text string, left, rightExclusive int) string {
	left = max(0, left)
	if rightExclusive <= left {
		return ""
	}
	column := 0
	var out strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		width := runewidth.RuneWidth(r)
		if width <= 0 {
			width = 1
		}
		if column+width > left && column < rightExclusive {
			out.WriteRune(r)
		}
		column += width
		if column >= rightExclusive {
			break
		}
		text = text[size:]
	}
	return strings.TrimRight(out.String(), " ")
}

func terminalTextSelection(text string, left, right int) string {
	return terminalTextSelectionRange(text, left, right+1)
}

func (tv *TerminalView) semanticPieceLineBreakUnsafe(logicalLine int) bool {
	if tv.pt == nil || tv.li == nil || logicalLine < 0 {
		return false
	}
	end := tv.pt.Size()
	if logicalLine+1 < tv.li.LineCount() {
		end = tv.li.GetLineOffset(logicalLine + 1)
	}
	if end <= 0 {
		return false
	}
	tv.semanticScratch = tv.semanticScratch[:0]
	last, err := tv.pt.AppendRange(tv.semanticScratch, end-1, 1)
	return err == nil && len(last) == 1 && last[0] == '\n'
}

func (tv *TerminalView) semanticSelectionRowRangeUnsafe(layout terminalSemanticLayout,
	absoluteRow, left, rightExclusive int,
) (string, bool) {
	switch {
	case absoluteRow < 0 || absoluteRow >= layout.totalRows:
		return "", true
	case absoluteRow < layout.pieceRows:
		logicalLine, fragmentIndex := tv.engine.GetLogLineAtVisualRow(absoluteRow)
		fragments := tv.engine.GetFragments(logicalLine)
		if fragmentIndex < 0 || fragmentIndex >= len(fragments) {
			return "", true
		}
		fragment := fragments[fragmentIndex]
		tv.semanticScratch = tv.semanticScratch[:0]
		data, err := tv.pt.AppendRange(tv.semanticScratch,
			fragment.ByteOffsetStart,
			fragment.ByteOffsetEnd-fragment.ByteOffsetStart)
		if err != nil {
			return "", true
		}
		text := string(data)
		hardBreak := fragmentIndex == len(fragments)-1 &&
			tv.semanticPieceLineBreakUnsafe(logicalLine)
		return terminalTextSelectionRange(text, left, rightExclusive), hardBreak
	case absoluteRow < layout.pieceRows+layout.historyRows:
		index := absoluteRow - layout.pieceRows
		return terminalCellsSelectionRange(tv.GridHistory[index], tv.Width,
				left, rightExclusive),
			!tv.GridHistoryWrap[index]
	default:
		screenRow := absoluteRow - layout.pieceRows - layout.historyRows
		index := screenRow - layout.activeOffset
		if index < 0 || index >= len(layout.buffer) {
			return "", true
		}
		hardBreak := true
		if !layout.altScreen && index < len(layout.wraps) {
			hardBreak = !layout.wraps[index]
		}
		return terminalCellsSelectionRange(layout.buffer[index], tv.Width,
				left, rightExclusive),
			hardBreak
	}
}

// semanticSelectionText extracts only the selected composite rows.  Its memory
// use is proportional to clipboard text, not terminal history size.
func (tv *TerminalView) semanticSelectionText(startRow, startColumn,
	endRow, endColumn int,
) string {
	return tv.semanticSelectionTextWithEndpoints(startRow, startColumn,
		endRow, endColumn, false)
}

func (tv *TerminalView) semanticBoundarySelectionText(startRow, startColumn,
	endRow, endColumn int,
) string {
	return tv.semanticSelectionTextWithEndpoints(startRow, startColumn,
		endRow, endColumn, true)
}

func (tv *TerminalView) semanticSelectionTextWithEndpoints(
	startRow, startColumn, endRow, endColumn int, endExclusive bool,
) string {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	layout := tv.semanticLayoutUnsafe()
	if layout.totalRows <= 0 {
		return ""
	}
	startRow, startColumn, endRow, endColumn = terminalSelectionOrder(
		startRow, startColumn, endRow, endColumn)
	if !endExclusive {
		endColumn++
	}
	startRow = max(0, min(startRow, layout.totalRows-1))
	endRow = max(startRow, min(endRow, layout.totalRows-1))

	var out strings.Builder
	for row := startRow; row <= endRow; row++ {
		left, right := 0, max(0, tv.Width)
		if row == startRow {
			left = startColumn
		}
		if row == endRow {
			right = endColumn
		}
		text, hardBreak := tv.semanticSelectionRowRangeUnsafe(
			layout, row, left, right)
		out.WriteString(text)
		if row < endRow && hardBreak {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
