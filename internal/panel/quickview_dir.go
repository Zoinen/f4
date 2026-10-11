package panel

import (
	"strings"

	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// quickViewDirRow is one line of the folder summary. Rows with a label are
// the ones the cursor stops on and C copies; the rest are spacers, the
// "Contains:" heading and the scan status.
type quickViewDirRow struct {
	label string
	value string
	text  string
}

func (r quickViewDirRow) copyable() bool { return r.label != "" }

// settleDirCursor keeps the cursor on a copyable row as rows come and go
// while the scan runs; a fresh folder starts on its first row.
func (q *QuickViewPanel) settleDirCursor() {
	if q.dirCursor >= 0 && q.dirCursor < len(q.dirRows) && q.dirRows[q.dirCursor].copyable() {
		return
	}
	q.dirCursor = -1
	for i, row := range q.dirRows {
		if row.copyable() {
			q.dirCursor = i
			return
		}
	}
}

// moveDirCursor steps over |delta| copyable rows, stopping at either end, so
// PgUp/PgDn and Home/End land on the first or last row as in the info panel.
func (q *QuickViewPanel) moveDirCursor(delta int) {
	q.settleDirCursor()
	if q.dirCursor < 0 {
		return
	}
	step := 1
	if delta < 0 {
		step, delta = -1, -delta
	}
	for ; delta > 0; delta-- {
		next := q.dirCursor + step
		for next >= 0 && next < len(q.dirRows) && !q.dirRows[next].copyable() {
			next += step
		}
		if next < 0 || next >= len(q.dirRows) {
			return
		}
		q.dirCursor = next
	}
}

func (q *QuickViewPanel) toggleDirMark() {
	q.settleDirCursor()
	if q.dirCursor < 0 {
		return
	}
	label := q.dirRows[q.dirCursor].label
	if q.dirMarks[label] {
		delete(q.dirMarks, label)
		return
	}
	if q.dirMarks == nil {
		q.dirMarks = make(map[string]bool)
	}
	q.dirMarks[label] = true
}

// dirRowAttr colours row i as the info panel colours its rows.
func (q *QuickViewPanel) dirRowAttr(i int, base uint64) uint64 {
	row := q.dirRows[i]
	if !row.copyable() {
		return base
	}
	marked := q.dirMarks[row.label]
	cursor := q.Focused && i == q.dirCursor
	switch {
	case cursor && marked:
		return vtui.Palette[theme.ColPanelSelectedCursor]
	case cursor:
		return vtui.Palette[theme.ColPanelCursor]
	case marked:
		return vtui.Palette[theme.ColPanelSelectedText]
	}
	return base
}

// copyDirRows is C over the folder summary: the marked rows as
// "label: value" lines in screen order, or else the value under the cursor.
func (q *QuickViewPanel) copyDirRows() {
	var lines []string
	for _, row := range q.dirRows {
		if row.copyable() && q.dirMarks[row.label] {
			lines = append(lines, row.label+": "+row.value)
		}
	}
	if len(lines) > 0 {
		q.copyText(strings.Join(lines, "\n"), len(lines))
		return
	}
	q.settleDirCursor()
	if q.dirCursor >= 0 {
		q.copyText(q.dirRows[q.dirCursor].value, 0)
	}
}
