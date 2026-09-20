package terminal

import "github.com/unxed/vtui"

// Reflow: a width change re-wraps the primary screen and GridHistory.
//
// The only thing ever joined is what the view split itself. WrapFlags[y] is
// set in exactly one place, PutChar, when the view ran out of columns with no
// line feed in the stream, so a row with the flag continues into the next one
// by the view's own record, and a row without it ended where the application
// ended it. Nothing is inferred from how full a row is, what it ends with, or
// how the rows look: a row that is exactly as wide as the window and ends in
// a line feed stays a line of its own at every width.
//
// That record is only worth anything when the stream delivers long lines
// whole, so the owner of the view turns reflow on per session (SetReflow): a
// Unix pty always, a Windows ConPTY that passes VT writes through (see
// conpty_package.go), and nothing else. With reflow off, Resize keeps the
// old behaviour of preserving rows as they are.
//
// The cursor is carried as an offset into its logical line, not as (x, y),
// so it stays on the character it was on (far2l's SetSizeRecomposing does
// the same). The number of rows below the cursor is kept, which is the
// bottom anchoring a height change already has: narrowing pushes the top of
// the screen into GridHistory, widening pulls it back.

// wrapPadCell fills the columns a wide character could not use when it moved
// to the next row. It is not text: GetAllLogBytes, extrusion and selection
// skip it, so a line that was re-wrapped any number of times reads back
// unchanged. Char 0 is never written by PutChar, which ignores control
// characters.
var wrapPadCell = vtui.CharInfo{Char: 0, Attributes: DefaultTermAttr}

func isWrapPad(c vtui.CharInfo) bool { return c.Char == 0 }

// isTrailingBlank reports whether a cell at the end of a row carries nothing:
// a default space or a wrap pad.
func isTrailingBlank(c vtui.CharInfo) bool {
	return isWrapPad(c) || (c.Char == ' ' && c.Attributes == DefaultTermAttr)
}

// SetReflow turns reflow on or off for the session currently feeding the
// view. The owner calls it whenever the source of the output may have
// changed; it is cheap when nothing changes.
func (tv *TerminalView) SetReflow(on bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.reflow != on {
		tv.reflow = on
		vtui.DebugLog("TERM_VIEW: reflow %v", on)
	}
}

// ReflowEnabled reports whether a width change re-wraps the grid.
func (tv *TerminalView) ReflowEnabled() bool {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.reflow
}

// reflowLine is what is known about the logical line being collected; its
// cells accumulate in a local slice next to it.
type reflowLine struct {
	cursor int // offset of the cursor in the line's cells, or -1
	// rowStarts maps viewport rows of the old grid that began inside this
	// line to the offset of their first cell, so pictures can follow them.
	rowStarts map[int]int
}

// wrappedRowCells returns the cells of a row that continues into the next
// one: the whole row, except the pads a wide character left at its end. A
// row kept wider than the viewport by a resize without reflow keeps the
// cells beyond the edge too; they are text the application wrote.
func wrappedRowCells(row []vtui.CharInfo, _ int) []vtui.CharInfo {
	n := len(row)
	for n > 0 && isWrapPad(row[n-1]) {
		n--
	}
	return row[:n]
}

// finalRowCells returns the cells of a row that ends its line: up to the last
// cell that carries anything, and at least up to minLen (the cursor's column).
func finalRowCells(row []vtui.CharInfo, minLen int) []vtui.CharInfo {
	n := len(row)
	for n > 0 && isTrailingBlank(row[n-1]) {
		n--
	}
	if minLen > len(row) {
		minLen = len(row)
	}
	if n < minLen {
		n = minLen
	}
	return row[:n]
}

func rowIsBlank(row []vtui.CharInfo) bool {
	for _, c := range row {
		if !isTrailingBlank(c) {
			return false
		}
	}
	return true
}

func blankRow(width int) []vtui.CharInfo {
	row := make([]vtui.CharInfo, width)
	for i := range row {
		row[i] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
	}
	return row
}

// laidOutLine is a logical line cut into rows of one width.
type laidOutLine struct {
	rows       [][]vtui.CharInfo
	rowStart   []int // offset of each row's first cell
	cursorRow  int   // -1 when the line does not hold the cursor
	cursorCol  int
	cellsCount int
}

// layoutLine cuts cells into rows of width columns. A wide character never
// straddles two rows: if it does not fit, the rest of the row is padded and
// the character starts the next one. A cursor just past the last cell of a
// full row stays on that row at column width, the pending-wrap position
// PutChar itself leaves it in.
func layoutLine(cells []vtui.CharInfo, cursor, width int) laidOutLine {
	out := laidOutLine{cursorRow: -1, cellsCount: len(cells)}
	row := blankRow(width)
	start, col := 0, 0
	flush := func(next int) {
		out.rows = append(out.rows, row)
		out.rowStart = append(out.rowStart, start)
		row = blankRow(width)
		start, col = next, 0
	}
	for i := 0; i < len(cells); {
		n := 1
		if cells[i].Char != vtui.WideCharFiller && i+1 < len(cells) && cells[i+1].Char == vtui.WideCharFiller {
			n = 2
		}
		if col > 0 && col+n > width {
			for c := col; c < width; c++ {
				row[c] = wrapPadCell
			}
			flush(i)
		}
		for k := 0; k < n; k++ {
			if i+k == cursor {
				out.cursorRow, out.cursorCol = len(out.rows), col+k
			}
			if col+k < width {
				row[col+k] = cells[i+k]
			}
		}
		i += n
		col += n
		if col >= width && i < len(cells) {
			flush(i)
		}
	}
	if cursor >= len(cells) && cursor >= 0 {
		out.cursorRow, out.cursorCol = len(out.rows), col
	}
	out.rows = append(out.rows, row)
	out.rowStart = append(out.rowStart, start)
	return out
}

// reflowResizeLocked re-wraps the primary screen and GridHistory to w x h.
// The caller holds the mutex, has checked that the primary screen is active
// and that both dimensions are positive.
func (tv *TerminalView) reflowResizeLocked(w, h int) {
	oldW, oldH := tv.Width, tv.Height
	cursorY := tv.CursorY
	if cursorY < 0 {
		cursorY = 0
	}
	if cursorY >= oldH {
		cursorY = oldH - 1
	}
	below := oldH - 1 - cursorY

	last := cursorY
	for y := oldH - 1; y > last; y-- {
		if tv.rowHasText(y) {
			last = y
			break
		}
	}

	// GridHistory up to the run of wrapped rows that continues into the
	// viewport: those lines are complete.
	seam := len(tv.GridHistory)
	for seam > 0 && tv.GridHistoryWrap[seam-1] {
		seam--
	}
	var rows [][]vtui.CharInfo
	var wraps []bool
	appendLine := func(line laidOutLine) int {
		base := len(rows)
		for i, r := range line.rows {
			rows = append(rows, r)
			wraps = append(wraps, i < len(line.rows)-1)
		}
		return base
	}
	var cells []vtui.CharInfo
	for i := 0; i < seam; i++ {
		if tv.GridHistoryWrap[i] {
			cells = append(cells, wrappedRowCells(tv.GridHistory[i], oldW)...)
			continue
		}
		cells = append(cells, finalRowCells(tv.GridHistory[i], 0)...)
		appendLine(layoutLine(cells, -1, w))
		cells = cells[:0:0]
	}
	historyRows := len(rows)

	// The viewport, starting with the head of a line that began in history.
	cells = nil
	for i := seam; i < len(tv.GridHistory); i++ {
		cells = append(cells, wrappedRowCells(tv.GridHistory[i], oldW)...)
	}
	line := reflowLine{cursor: -1, rowStarts: map[int]int{}}
	newRowOf := map[int]int{}
	cursorAbs, cursorCol := -1, 0
	for y := 0; y <= last && y < len(tv.Lines); y++ {
		row := tv.Lines[y]
		continues := y < last && y < len(tv.WrapFlags) && tv.WrapFlags[y]
		line.rowStarts[y] = len(cells)
		if y == cursorY {
			x := tv.CursorX
			if x < 0 {
				x = 0
			}
			rowCells := wrappedRowCells(row, oldW)
			if !continues {
				rowCells = finalRowCells(row, x)
			}
			if x > len(rowCells) {
				x = len(rowCells)
			}
			line.cursor = len(cells) + x
			cells = append(cells, rowCells...)
		} else if continues {
			cells = append(cells, wrappedRowCells(row, oldW)...)
		} else {
			cells = append(cells, finalRowCells(row, 0)...)
		}
		if continues {
			continue
		}
		laid := layoutLine(cells, line.cursor, w)
		base := appendLine(laid)
		if laid.cursorRow >= 0 {
			cursorAbs, cursorCol = base+laid.cursorRow, laid.cursorCol
		}
		for oldRow, off := range line.rowStarts {
			r := 0
			for r+1 < len(laid.rowStart) && laid.rowStart[r+1] <= off {
				r++
			}
			newRowOf[oldRow] = base + r
		}
		cells = nil
		line = reflowLine{cursor: -1, rowStarts: map[int]int{}}
	}
	if cursorAbs < 0 {
		// The cursor sits below every row that holds anything: keep it on a
		// blank row of its own.
		rows = append(rows, blankRow(w))
		wraps = append(wraps, false)
		cursorAbs, cursorCol = len(rows)-1, 0
	}

	// Keep the rows below the cursor. When there is not enough above it to
	// fill the screen, the screen starts with blank rows, as a fresh terminal
	// does (TERMINAL.md, rule 2), rather than the cursor moving up.
	target := h - 1 - below
	if target < 0 {
		target = 0
	}
	top := cursorAbs - target // row shown at the top; negative means blank rows
	if len(rows)-top > h {
		// Rows below the cursor do not fit: move the window down, as far as
		// keeping the cursor on screen allows, and let the rest go.
		top = len(rows) - h
		if top > cursorAbs {
			top = cursorAbs
		}
		vtui.DebugLog("REFLOW_RESIZE: %d rows below the cursor did not fit in %dx%d", len(rows)-top-h, w, h)
	}

	newLines := make([][]vtui.CharInfo, h)
	newWrap := make([]bool, h)
	for y := 0; y < h; y++ {
		if idx := top + y; idx >= 0 && idx < len(rows) {
			newLines[y] = rows[idx]
			newWrap[y] = wraps[idx]
		} else {
			newLines[y] = blankRow(w)
		}
	}

	newAlt := make([][]vtui.CharInfo, h)
	for y := range newAlt {
		newAlt[y] = blankRow(w)
		if y < len(tv.AltLines) {
			copy(newAlt[y], tv.AltLines[y])
		}
	}

	start := top
	if start < 0 {
		start = 0
	}
	history, historyWrap := rows[:start:start], wraps[:start:start]
	if len(tv.GridHistory) == 0 && tv.Pt.Size() == 0 {
		// The extrusion guard of scrollUp (TERMINAL.md, rule 3): the blank
		// rows a terminal starts with never become history, and a reflow
		// that pushes them off the top must not make them history either.
		for len(history) > 0 && !historyWrap[0] && rowIsBlank(history[0]) {
			history, historyWrap = history[1:], historyWrap[1:]
		}
	}

	historyBefore := len(tv.GridHistory)
	tv.GridHistory = history
	tv.GridHistoryWrap = historyWrap
	tv.Lines = newLines
	tv.WrapFlags = newWrap
	tv.AltLines = newAlt
	tv.Width, tv.Height = w, h
	tv.ScrollTop, tv.ScrollBottom = 0, h-1
	tv.CursorY = cursorAbs - top
	tv.CursorX = cursorCol
	tv.lastCharWasCR = tv.CursorX == 0

	for i := range tv.Images {
		if tv.Images[i].Alt {
			continue
		}
		if r, ok := newRowOf[tv.Images[i].Row]; ok {
			tv.Images[i].Row = r - top
		} else {
			tv.Images[i].Row += tv.CursorY - cursorY
		}
	}
	tv.kittyResizePlacements(0, h)
	tv.kittyRecomputeSpans()

	tv.trimGridHistoryLocked()
	vtui.DebugLog("REFLOW_RESIZE: %dx%d -> %dx%d; history %d -> %d rows (%d laid out), cursor (%d,%d)",
		oldW, oldH, w, h, historyBefore, len(tv.GridHistory), historyRows, tv.CursorX, tv.CursorY)
}
