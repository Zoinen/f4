package vtui

import (
	"github.com/unxed/vtinput"
	"sort"
	"strings"
)

// TableColumn defines the properties of a single table column.
type TableColumn struct {
	Title string
	// Width in characters. Width <= 0 makes the column flexible: all flexible
	// columns evenly share the space left after fixed-width columns and
	// separators, and are recomputed whenever the table is resized.
	Width int
	// MinWidth is the minimum width of a flexible column (Width <= 0), in
	// characters. If MinWidth <= 0, the title width is used as the minimum.
	// Ignored for fixed-width columns.
	MinWidth  int
	Alignment Alignment
}

// TableRow is an interface for data providers.
type TableRow interface {
	GetCellText(col int) string
}

// Table is a generic control for displaying tabular data.
// SelectableRow is an optional interface for rows that can be selected.
type SelectableRow interface {
	IsSelected() bool
}

// MultiColSelectableRow is an interface for multi-column rows where selection is cell-specific.
type MultiColSelectableRow interface {
	IsColSelected(col int) bool
}

// CellColorableRow is an optional interface allowing rows to define custom colors per cell.
type CellColorableRow interface {
	GetCellAttr(col int, defaultAttr uint64) uint64
}

// Table is a generic control for displaying tabular data.
type Table struct {
	ScrollView
	Columns []TableColumn
	Rows    []TableRow
	// rowProvider supplies rows lazily for large externally sorted models.
	// It is mutually exclusive with Rows and lets a hidden/native-backed table
	// keep exact scroll/cursor metrics without allocating one wrapper per item.
	rowProvider func(index int) TableRow
	rowCount    int

	SelectCol        int
	CellSelection    bool
	ShowHeader       bool
	ShowSeparators   bool
	AlwaysShowCursor bool

	// Sortable enables click-on-header sorting. Default is false: no sorting
	// and header clicks are ignored, so applications doing their own sorting
	// (e.g. by rewriting column titles) are unaffected.
	Sortable bool
	// SortColumn is the column rows are sorted by; -1 (default) means no
	// sorting. SortAscending controls the direction. SortCompare is an
	// optional comparator; when nil, rows are compared by cell text.
	SortColumn    int
	SortAscending bool
	SortCompare   func(a, b TableRow, col int) int

	// QuickSearch enables type-to-filter: while the table is focused,
	// printable characters go into a search string shown in a line below the
	// table, and rows are filtered by fuzzy match (Myers' bit-vector
	// algorithm) against all columns, best match wins. The filtered list is
	// ranked by (edit distance, match position). Default is false.
	QuickSearch bool
	// SearchCaseSensitive makes QuickSearch case-sensitive (default false).
	SearchCaseSensitive bool
	// OnSearchChange is called whenever the search string changes.
	OnSearchChange func(text string)

	ColorTextIdx             int
	ColorSelectedTextIdx     int
	ColorItemSelectTextIdx   int
	ColorItemSelectCursorIdx int
	ColorTitleIdx            int
	ColorBoxIdx              int

	// colWidths caches the resolved column widths (flexible columns expanded);
	// reused across frames to avoid allocations in the render hot path.
	colWidths []int
	// order maps display position to Rows index when sorting is active.
	// The Rows slice itself is never reordered.
	order []int
	// searchRunes is the QuickSearch string; searchCursor/searchLeft are the
	// cursor and horizontal scroll positions (in runes) within it.
	searchRunes  []rune
	searchCursor int
	searchLeft   int
	// matchBuf is reused by the search filter to avoid allocations.
	matchBuf []searchMatch
	// matchSpans holds the matched cell span per Rows index for needle
	// highlighting; col == -1 means the row is not matched.
	matchSpans []cellHighlight
}

// searchMatch is one row passing the QuickSearch filter.
type searchMatch struct {
	idx, score, pos int
}

// cellHighlight locates the matched substring inside one cell: column index
// and the [start, end] span in runes of that cell's text.
type cellHighlight struct {
	col, start, end int
}

func NewTable(x, y, w, h int, columns []TableColumn) *Table {
	t := &Table{
		Columns:                  columns,
		Rows:                     []TableRow{},
		ShowHeader:               true,
		ShowSeparators:           true,
		ColorTextIdx:             ColTableText,
		ColorSelectedTextIdx:     ColTableSelectedText,
		ColorItemSelectTextIdx:   ColTableText,
		ColorItemSelectCursorIdx: ColTableSelectedText,
		ColorTitleIdx:            ColTableColumnTitle,
		ColorBoxIdx:              ColTableBox,
		SortColumn:               -1,
	}
	t.canFocus = true
	t.InitScrollBar(t)
	t.SetPosition(x, y, x+w-1, y+h-1)
	return t
}

// resolvedWidths returns the effective width of every column. Columns with
// Width <= 0 are flexible: each gets its minimum width (MinWidth, or the
// title width if unset), then the remaining content width (after fixed-width
// columns and the 1-cell gaps between columns) is distributed between them
// evenly. The result is recomputed on every call, so it follows widget
// resizes automatically.
func (t *Table) resolvedWidths() []int {
	n := len(t.Columns)
	t.colWidths = t.colWidths[:0]
	if n == 0 {
		return t.colWidths
	}

	flexCount := 0
	fixed := 0
	minSum := 0
	for i := range t.Columns {
		if t.Columns[i].Width <= 0 {
			flexCount++
			minSum += t.Columns[i].minWidth()
		} else {
			fixed += t.Columns[i].Width
		}
	}

	extra := 0
	if flexCount > 0 {
		avail := t.GetContentWidth() - fixed - (n - 1)
		if avail < minSum {
			avail = minSum // each flexible column gets at least its minimum
		}
		extra = avail - minSum
	}
	per := 0
	rem := 0
	if flexCount > 0 {
		per = extra / flexCount
		rem = extra % flexCount
	}

	for i := range t.Columns {
		w := t.Columns[i].Width
		if w <= 0 {
			w = t.Columns[i].minWidth() + per
			if rem > 0 {
				w++
				rem--
			}
		}
		t.colWidths = append(t.colWidths, w)
	}
	return t.colWidths
}

// minWidth returns the minimum width of a flexible column: MinWidth if set,
// otherwise the width of the column title.
func (c *TableColumn) minWidth() int {
	if c.MinWidth > 0 {
		return c.MinWidth
	}
	return StringWidth(c.Title)
}

func (t *Table) SetRows(rows []TableRow) {
	t.Rows = rows
	t.rowProvider = nil
	t.rowCount = len(rows)
	t.ItemCount = len(rows)
	t.resort()
	t.clampSelectionAfterRowsChanged()
	t.EnsureVisible()
}

// SetRowProvider installs an externally ordered virtual row source. Only rows
// touched by rendering, sorting, or quick search are materialized.
func (t *Table) SetRowProvider(count int, provider func(index int) TableRow) {
	if count < 0 {
		count = 0
	}
	t.Rows = nil
	t.rowProvider = provider
	t.rowCount = count
	t.ItemCount = count
	t.resort()
	t.clampSelectionAfterRowsChanged()
	t.EnsureVisible()
}

func (t *Table) clampSelectionAfterRowsChanged() {
	if t.ItemCount == 0 {
		t.SelectPos = 0
	} else if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	} else if t.SelectPos < 0 {
		t.SelectPos = 0
	}
}

func (t *Table) sourceRowCount() int {
	if t.rowProvider != nil {
		return t.rowCount
	}
	return len(t.Rows)
}

func (t *Table) sourceRow(index int) TableRow {
	if index < 0 || index >= t.sourceRowCount() {
		return nil
	}
	if t.rowProvider != nil {
		return t.rowProvider(index)
	}
	return t.Rows[index]
}

// SetSort sorts rows by the given column. A negative col disables sorting.
// The header of the sorted column shows a direction arrow (↑/↓).
func (t *Table) SetSort(col int, ascending bool) {
	t.SortColumn = col
	t.SortAscending = ascending
	t.resort()
}

// ClearSort disables sorting and restores the original row order.
func (t *Table) ClearSort() {
	t.SortColumn = -1
	t.resort()
}

// resort rebuilds the display-to-row index mapping. With no active sort it
// is the identity mapping; the Rows slice itself is never reordered.
func (t *Table) resort() {
	n := t.sourceRowCount()
	if len(t.searchRunes) == 0 &&
		(t.SortColumn < 0 || t.SortColumn >= len(t.Columns) || n < 2) {
		t.order = t.order[:0]
		t.ItemCount = n
		if t.SelectPos >= t.ItemCount {
			t.SelectPos = t.ItemCount - 1
		}
		if t.SelectPos < 0 {
			t.SelectPos = 0
		}
		return
	}
	if cap(t.order) < n {
		t.order = make([]int, n)
	} else {
		t.order = t.order[:n]
	}
	for i := range t.order {
		t.order[i] = i
	}

	if len(t.searchRunes) > 0 {
		// While searching, the column sort gives way to match ranking.
		t.applySearchFilter()
	} else if col := t.SortColumn; col >= 0 && col < len(t.Columns) && n >= 2 {
		ascending := t.SortAscending
		cmp := t.SortCompare
		sort.SliceStable(t.order, func(i, j int) bool {
			a, b := t.sourceRow(t.order[i]), t.sourceRow(t.order[j])
			c := 0
			if cmp != nil {
				c = cmp(a, b, col)
			} else {
				c = strings.Compare(a.GetCellText(col), b.GetCellText(col))
			}
			if !ascending {
				c = -c
			}
			return c < 0
		})
	}

	t.ItemCount = len(t.order)
	if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	}
	if t.SelectPos < 0 {
		t.SelectPos = 0
	}
}

// applySearchFilter keeps only rows fuzzy-matching the search string (best
// match across all columns wins) and ranks them by (distance, position).
// The matched cell span is remembered per row for needle highlighting.
func (t *Table) applySearchFilter() {
	matcher := newFuzzyMatcher(string(t.searchRunes), t.SearchCaseSensitive)
	t.matchBuf = t.matchBuf[:0]
	rowCount := t.sourceRowCount()
	if cap(t.matchSpans) < rowCount {
		t.matchSpans = make([]cellHighlight, rowCount)
	} else {
		t.matchSpans = t.matchSpans[:rowCount]
	}
	for i := range t.matchSpans {
		t.matchSpans[i].col = -1
	}
	for i := 0; i < rowCount; i++ {
		row := t.sourceRow(i)
		if row == nil {
			continue
		}
		bestScore := -1
		bestStart, bestEnd, bestCol := 0, 0, 0
		for col := range t.Columns {
			score, start, end, ok := matcher.match(row.GetCellText(col))
			if ok && (bestScore < 0 || score < bestScore || (score == bestScore && start < bestStart)) {
				bestScore, bestStart, bestEnd, bestCol = score, start, end, col
			}
		}
		if bestScore >= 0 {
			t.matchBuf = append(t.matchBuf, searchMatch{i, bestScore, bestStart})
			t.matchSpans[i] = cellHighlight{bestCol, bestStart, bestEnd}
		}
	}
	sort.Slice(t.matchBuf, func(a, b int) bool {
		ma, mb := t.matchBuf[a], t.matchBuf[b]
		if ma.score != mb.score {
			return ma.score < mb.score
		}
		if ma.pos != mb.pos {
			return ma.pos < mb.pos
		}
		return ma.idx < mb.idx
	})
	t.order = t.order[:len(t.matchBuf)]
	for i, m := range t.matchBuf {
		t.order[i] = m.idx
	}
}

// SearchText returns the current QuickSearch string.
func (t *Table) SearchText() string {
	return string(t.searchRunes)
}

// SetSearchText replaces the QuickSearch string and refilters the rows.
func (t *Table) SetSearchText(text string) {
	t.searchRunes = []rune(text)
	t.searchCursor = len(t.searchRunes)
	t.searchLeft = 0
	t.resort()
	t.EnsureVisible()
	t.fireSearchChange()
}

// ClearSearch empties the QuickSearch string and restores the full row list.
func (t *Table) ClearSearch() {
	if len(t.searchRunes) == 0 {
		return
	}
	t.searchRunes = t.searchRunes[:0]
	t.searchCursor = 0
	t.searchLeft = 0
	t.resort()
	t.fireSearchChange()
}

func (t *Table) fireSearchChange() {
	if t.OnSearchChange != nil {
		t.OnSearchChange(string(t.searchRunes))
	}
}

// rowAt maps a display position to the index in Rows, accounting for the
// active sorting. Out-of-range positions are returned unchanged.
func (t *Table) rowAt(pos int) int {
	if pos >= 0 && pos < len(t.order) {
		return t.order[pos]
	}
	return pos
}

// RowAt maps a display position (e.g. SelectPos) to the index in Rows,
// accounting for the active sorting. With no sorting it returns pos.
func (t *Table) RowAt(pos int) int {
	return t.rowAt(pos)
}

func (t *Table) Show(scr *ScreenBuf) {
	t.ScreenObject.Show(scr)
	t.DisplayObject(scr)
}

func (t *Table) DisplayObject(scr *ScreenBuf) {
	if !t.IsVisible() {
		return
	}

	// Ensure margins are in sync with ShowHeader/ShowScrollBar before rendering
	t.SetPosition(t.X1, t.Y1, t.X2, t.Y2)

	yOffset := 0

	// 1. Draw Header
	if t.ShowHeader {
		t.drawRow(scr, t.Y1, -1, Palette[t.ColorTitleIdx])
		yOffset++
	}

	// 2. Draw Data Rows (ViewHeight already excludes header and search line).
	// While a search is active, results are shown bottom-up (best match right
	// above the search line), telescope-style.
	dataHeight := t.ViewHeight
	if dataHeight < 0 {
		dataHeight = 0
	}
	bottomUp := len(t.searchRunes) > 0
	dataBottom := t.Y1 + yOffset + dataHeight - 1
	for i := 0; i < dataHeight; i++ {
		displayPos := t.TopPos + i
		currY := t.Y1 + yOffset + i
		if bottomUp {
			currY = dataBottom - i
		}

		if displayPos < t.ItemCount {
			rowIdx := t.rowAt(displayPos)
			//isSelected := false
			// Calculate standard attribute as a fallback (passed into drawRow)
			attr := Palette[t.ColorTextIdx]
			t.drawRow(scr, currY, rowIdx, attr)
		} else {
			// Fill empty space with background color
			scr.FillRect(t.X1, currY, t.X2, currY, ' ', Palette[t.ColorTextIdx])
		}
	}

	// 3. Draw Vertical Separators if needed
	if t.ShowSeparators {
		p := NewPainter(scr)
		widths := t.resolvedWidths()
		currX := t.X1
		sepChar := boxSymbols[bsV]     // │
		sepY2 := t.Y2 - t.MarginBottom // do not cross the search line
		for i := 0; i < len(t.Columns)-1; i++ {
			currX += widths[i]
			p.Fill(currX, t.Y1, currX, sepY2, sepChar, Palette[t.ColorBoxIdx])
			currX++
		}
	}

	// 4. Draw Scrollbar
	t.DrawScrollBar(scr)

	// 5. Draw the QuickSearch line
	if t.QuickSearch {
		t.drawSearchLine(scr)
	}
}

func (t *Table) drawRow(scr *ScreenBuf, y int, rowIdx int, attr uint64) {
	endX := t.X1 + t.GetContentWidth() - 1
	widths := t.resolvedWidths()

	currX := t.X1
	var row TableRow
	if rowIdx >= 0 {
		row = t.sourceRow(rowIdx)
	}
	for colIdx, col := range t.Columns {
		text := ""
		if rowIdx == -1 {
			text = col.Title
		} else if row != nil {
			text = row.GetCellText(colIdx)
		}

		isSelected := false
		if row != nil {
			if mcsr, ok := row.(MultiColSelectableRow); ok {
				isSelected = mcsr.IsColSelected(colIdx)
			} else if selRow, ok := row.(SelectableRow); ok {
				isSelected = selRow.IsSelected()
			}
		}

		isCursorHere := rowIdx == t.SelectPos && (!t.CellSelection || colIdx == t.SelectCol)

		stateAttr := attr
		if rowIdx != -1 {
			if isCursorHere {
				if t.IsFocused() {
					if isSelected {
						stateAttr = Palette[t.ColorItemSelectCursorIdx]
					} else {
						stateAttr = Palette[t.ColorSelectedTextIdx]
					}
				} else {
					if isSelected {
						stateAttr = Palette[t.ColorItemSelectTextIdx]
					} else if t.AlwaysShowCursor {
						stateAttr = Palette[t.ColorSelectedTextIdx]
					}
				}
			} else if isSelected {
				stateAttr = Palette[t.ColorItemSelectTextIdx]
			}

			if cr, ok := row.(CellColorableRow); ok {
				stateAttr = cr.GetCellAttr(colIdx, stateAttr)
			}
		}

		cellAttr := stateAttr

		// Prepare cell text with alignment. The sorted column's header gets a
		// direction arrow appended at the right edge of the cell.
		cellText := t.formatCell(text, widths[colIdx], col.Alignment)
		if rowIdx == -1 && colIdx == t.SortColumn {
			cellText = t.formatSortedHeader(text, widths[colIdx], col.Alignment)
		}
		cis := StringToCharInfo(cellText, cellAttr)
		// Invert the colors of the matched substring (QuickSearch highlight).
		if rowIdx >= 0 && len(t.searchRunes) > 0 && rowIdx < len(t.matchSpans) {
			if span := t.matchSpans[rowIdx]; span.col == colIdx {
				t.applyCellHighlight(cis, text, widths[colIdx], col.Alignment, span)
			}
		}
		scr.Write(currX, y, cis)
		currX += widths[colIdx]

		// Skip separator space if not the last column
		if colIdx < len(t.Columns)-1 {
			currX++
		}
	}

	// Fill remaining horizontal space if any
	lastX := currX - 1
	if lastX < endX {
		scr.FillRect(lastX+1, y, endX, y, ' ', attr)
	}
}

// applyCellHighlight inverts the colors of the cells covered by the matched
// substring. span.start/span.end are rune indices in the original cell text;
// they are mapped to display cells accounting for truncation, alignment
// padding and wide runes.
func (t *Table) applyCellHighlight(cis []CharInfo, text string, width int, align Alignment, span cellHighlight) {
	truncated := TruncateString(text, width, "")
	space := width - StringWidth(truncated)
	if space < 0 {
		space = 0
	}
	padLeft := 0
	switch align {
	case AlignRight:
		padLeft = space
	case AlignCenter:
		padLeft = space / 2
	}
	cellPos := padLeft
	runeIdx := 0
	for _, r := range truncated {
		w := ClusterWidth(string(r))
		if w < 1 {
			runeIdx++
			continue // zero-width runes occupy no display cell
		}
		if runeIdx >= span.start && runeIdx <= span.end {
			for k := 0; k < w && cellPos+k < len(cis); k++ {
				cis[cellPos+k].Attributes = InvertColors(cis[cellPos+k].Attributes)
			}
		}
		cellPos += w
		runeIdx++
	}
}

// formatSortedHeader renders a column header with a sort direction arrow
// (↑/↓) at the right edge of the cell, reserving space for it so the title
// itself is truncated first.
func (t *Table) formatSortedHeader(title string, width int, align Alignment) string {
	arrow := " ↓"
	if t.SortAscending {
		arrow = " ↑"
	}
	arrowWidth := StringWidth(arrow)
	if width <= arrowWidth {
		return TruncateString(arrow, width, "")
	}
	return t.formatCell(title, width-arrowWidth, align) + arrow
}

func (t *Table) formatCell(text string, width int, align Alignment) string {
	text = TruncateString(text, width, "")
	vLen := StringWidth(text)
	if vLen >= width {
		return text
	}

	space := width - vLen
	switch align {
	case AlignLeft:
		return text + strings.Repeat(" ", space)
	case AlignRight:
		return strings.Repeat(" ", space) + text
	case AlignCenter:
		left := space / 2
		right := space - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	}
	return text
}

func (t *Table) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || t.IsDisabled() {
		return false
	}

	searchActive := len(t.searchRunes) > 0
	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		// With bottom-up search results Up moves towards worse matches.
		if searchActive {
			if t.SelectPos == t.ItemCount-1 {
				return false
			}
		} else if t.SelectPos == 0 {
			return false
		}
	case vtinput.VK_DOWN:
		if searchActive {
			if t.SelectPos == 0 {
				return false
			}
		} else if t.SelectPos == t.ItemCount-1 {
			return false
		}
	}

	if searchActive {
		switch e.VirtualKeyCode {
		case vtinput.VK_UP:
			return t.MoveSelection(1)
		case vtinput.VK_DOWN:
			return t.MoveSelection(-1)
		}
	}

	if t.QuickSearch && t.processSearchKey(e) {
		return true
	}

	if t.CellSelection {
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT:
			if t.SelectCol > 0 {
				t.SelectCol--
				return true
			}
			if t.SelectPos == 0 {
				return false
			}
			if t.MoveSelection(-1) {
				t.SelectCol = len(t.Columns) - 1
				return true
			}
		case vtinput.VK_RIGHT:
			if t.SelectCol < len(t.Columns)-1 {
				t.SelectCol++
				return true
			}
			if t.SelectPos == t.ItemCount-1 {
				return false
			}
			if t.MoveSelection(1) {
				t.SelectCol = 0
				return true
			}
		}
	}

	return t.HandleKey(e)
}

// processSearchKey handles QuickSearch editing keys. It returns true if the
// event was consumed. Up/Down and all other navigation keys fall through to
// the default table handling; with CellSelection enabled it wins plain
// Left/Right, and the search cursor moves with Ctrl+Left/Right instead.
func (t *Table) processSearchKey(e *vtinput.InputEvent) bool {
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE:
		if len(t.searchRunes) > 0 {
			t.ClearSearch()
			return true
		}
		return false
	case vtinput.VK_BACK:
		if t.searchCursor > 0 {
			t.searchRunes = append(t.searchRunes[:t.searchCursor-1], t.searchRunes[t.searchCursor:]...)
			t.searchCursor--
			t.resort()
			t.EnsureVisible()
			t.fireSearchChange()
		}
		return true
	case vtinput.VK_DELETE:
		if t.searchCursor < len(t.searchRunes) {
			t.searchRunes = append(t.searchRunes[:t.searchCursor], t.searchRunes[t.searchCursor+1:]...)
			t.resort()
			t.EnsureVisible()
			t.fireSearchChange()
		}
		return true
	case vtinput.VK_LEFT:
		if ctrl || !t.CellSelection {
			if t.searchCursor > 0 {
				t.searchCursor--
			}
			return true
		}
		return false
	case vtinput.VK_RIGHT:
		if ctrl || !t.CellSelection {
			if t.searchCursor < len(t.searchRunes) {
				t.searchCursor++
			}
			return true
		}
		return false
	case vtinput.VK_HOME:
		if ctrl {
			t.searchCursor = 0
			return true
		}
		return false
	case vtinput.VK_END:
		if ctrl {
			t.searchCursor = len(t.searchRunes)
			return true
		}
		return false
	}

	// Printable characters are appended at the cursor position.
	if e.Char >= ' ' && !ctrl && !alt {
		t.searchRunes = append(t.searchRunes, 0)
		copy(t.searchRunes[t.searchCursor+1:], t.searchRunes[t.searchCursor:])
		t.searchRunes[t.searchCursor] = e.Char
		t.searchCursor++
		t.resort()
		t.EnsureVisible()
		t.fireSearchChange()
		return true
	}
	return false
}

func (t *Table) ProcessMouse(e *vtinput.InputEvent) bool {
	if t.IsDisabled() {
		return false
	}

	// Pre-process for CellSelection before generic HandleMouse
	originalCol := t.SelectCol
	colChanged := false

	if e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		// Click on a column header toggles sorting (only when Sortable).
		// Clicks on separator cells are consumed but do not change the sort.
		if t.Sortable && t.ShowHeader && int(e.MouseY) == t.Y1 &&
			int(e.MouseX) >= t.X1 && int(e.MouseX) <= t.X2 {
			widths := t.resolvedWidths()
			currX := t.X1
			for i := range t.Columns {
				if int(e.MouseX) >= currX && int(e.MouseX) < currX+widths[i] {
					if t.SortColumn == i {
						t.SortAscending = !t.SortAscending
					} else {
						t.SortColumn = i
						t.SortAscending = true
					}
					t.resort()
					break
				}
				currX += widths[i]
				if i < len(t.Columns)-1 {
					currX++
				}
			}
			return true
		}

		if t.CellSelection && t.HitTest(int(e.MouseX), int(e.MouseY)) {
			widths := t.resolvedWidths()
			currX := t.X1
			for i := range t.Columns {
				if int(e.MouseX) >= currX && int(e.MouseX) < currX+widths[i] {
					if t.SelectCol != i {
						t.SelectCol = i
						colChanged = true
					}
					break
				}
				currX += widths[i]
				if i < len(t.Columns)-1 {
					currX++
				}
			}
		}
	}

	// With bottom-up search results, clicks in the data area map to rows
	// counted from the bottom; consume them all so the generic handler does
	// not mis-map empty rows above the results.
	if len(t.searchRunes) > 0 && e.Type == vtinput.MouseEventType && e.ButtonState != 0 && e.KeyDown {
		dataTop := t.Y1 + t.MarginTop
		dataBottom := dataTop + t.ViewHeight - 1
		mx, my := int(e.MouseX), int(e.MouseY)
		if my >= dataTop && my <= dataBottom && mx >= t.X1 && mx < t.X1+t.GetContentWidth() {
			idx := t.TopPos + (dataBottom - my)
			if idx >= 0 && idx < t.ItemCount {
				oldPos := t.SelectPos
				t.SelectPos = idx
				if t.SelectPos != oldPos && t.OnSelect != nil {
					t.OnSelect(t.SelectPos)
				}
				isLeftDoubleClick := e.ButtonState == vtinput.FromLeft1stButtonPressed && (e.MouseEventFlags&vtinput.DoubleClick) != 0
				isMiddleClick := e.ButtonState == vtinput.FromLeft2ndButtonPressed
				if (isLeftDoubleClick || isMiddleClick) && t.OnAction != nil {
					t.OnAction(t.SelectPos)
				}
			}
			return true
		}
	}

	handled := t.HandleMouse(e)
	if !handled && colChanged {
		t.SelectCol = originalCol
	}
	return handled
}

func (t *Table) SetPosition(x1, y1, x2, y2 int) {
	t.MarginTop = map[bool]int{true: 1, false: 0}[t.ShowHeader]
	t.MarginBottom = map[bool]int{true: 1, false: 0}[t.QuickSearch]
	t.ScrollView.SetPosition(x1, y1, x2, y2)
}

// drawSearchLine renders the QuickSearch string in the bottom line of the
// table: "> text" with a hardware cursor while the table is focused.
func (t *Table) drawSearchLine(scr *ScreenBuf) {
	y := t.Y2
	attr := Palette[t.ColorTextIdx]
	scr.FillRect(t.X1, y, t.X2, y, ' ', attr)

	fullWidth := t.X2 - t.X1 + 1
	visibleWidth := fullWidth - 2 // "> " prefix
	if visibleWidth < 0 {
		visibleWidth = 0
	}

	// Horizontal scroll: keep the cursor visible (rune-width aware).
	if t.searchCursor < t.searchLeft {
		t.searchLeft = t.searchCursor
	}
	for t.searchLeft < t.searchCursor && StringWidth(string(t.searchRunes[t.searchLeft:t.searchCursor])) >= visibleWidth {
		t.searchLeft++
	}

	scr.Write(t.X1, y, StringToCharInfo("> ", attr))
	text := string(t.searchRunes[t.searchLeft:])
	text = TruncateString(text, visibleWidth, "")
	scr.Write(t.X1+2, y, StringToCharInfo(text, attr))

	if t.IsFocused() {
		scr.SetCursorVisible(true)
		scr.SetCursorShape(CursorShapeUnderline)
		cursorX := t.X1 + 2 + StringWidth(string(t.searchRunes[t.searchLeft:t.searchCursor]))
		if cursorX > t.X2 {
			cursorX = t.X2
		}
		scr.SetCursorPos(cursorX, y)
	}
}
