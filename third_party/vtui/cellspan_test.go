package vtui

import "testing"

func cellSpanTestRow(t *testing.T, s string, width int) []CharInfo {
	t.Helper()
	row := StringToCharInfo(s, 0)
	for len(row) < width {
		row = append(row, CharInfo{})
	}
	return row[:width]
}

func TestCellSpanAt_WideCharFromEitherHalf(t *testing.T) {
	const width = 6
	buf := cellSpanTestRow(t, "A世B", width)

	if sx, span := CellSpanAt(buf, width, 0, 0); sx != 0 || span != 1 {
		t.Errorf("narrow cell: got (%d, %d), want (0, 1)", sx, span)
	}
	if sx, span := CellSpanAt(buf, width, 1, 0); sx != 1 || span != 2 {
		t.Errorf("left half of wide char: got (%d, %d), want (1, 2)", sx, span)
	}
	if sx, span := CellSpanAt(buf, width, 2, 0); sx != 1 || span != 2 {
		t.Errorf("right half of wide char: got (%d, %d), want (1, 2)", sx, span)
	}
	if sx, span := CellSpanAt(buf, width, 3, 0); sx != 3 || span != 1 {
		t.Errorf("cell after wide char: got (%d, %d), want (3, 1)", sx, span)
	}
}

func TestCellSpanAt_CompositeCluster(t *testing.T) {
	// The case a renderer cannot get right by measuring the character: the
	// base rune of this cluster is one column wide, the cluster is two.
	const width = 4
	buf := cellSpanTestRow(t, "👨\u200D👩\u200D👦x", width)

	if sx, span := CellSpanAt(buf, width, 1, 0); sx != 0 || span != 2 {
		t.Errorf("composite cluster: got (%d, %d), want (0, 2)", sx, span)
	}
	if sx, span := CellSpanAt(buf, width, 2, 0); sx != 2 || span != 1 {
		t.Errorf("cell after cluster: got (%d, %d), want (2, 1)", sx, span)
	}
}

func TestCellSpanAt_SecondRow(t *testing.T) {
	const width = 4
	buf := append(cellSpanTestRow(t, "ab", width), cellSpanTestRow(t, "世", width)...)

	if sx, span := CellSpanAt(buf, width, 1, 1); sx != 0 || span != 2 {
		t.Errorf("second row: got (%d, %d), want (0, 2)", sx, span)
	}
}

func TestCellSpanAt_OutOfRange(t *testing.T) {
	buf := cellSpanTestRow(t, "ab", 2)

	for _, c := range []struct{ x, y int }{{-1, 0}, {2, 0}, {0, 5}, {0, -1}} {
		if sx, span := CellSpanAt(buf, 2, c.x, c.y); sx != c.x || span != 1 {
			t.Errorf("out of range (%d, %d): got (%d, %d), want (%d, 1)", c.x, c.y, sx, span, c.x)
		}
	}
	if sx, span := CellSpanAt(nil, 0, 3, 0); sx != 3 || span != 1 {
		t.Errorf("empty buffer: got (%d, %d), want (3, 1)", sx, span)
	}
}

func TestCellSpanAt_LeadingFiller(t *testing.T) {
	// An orphan filler in column zero must not send the walk out of the row.
	buf := []CharInfo{{Char: WideCharFiller}, {Char: 'a'}}
	if sx, span := CellSpanAt(buf, 2, 0, 0); sx != 0 || span != 1 {
		t.Errorf("orphan filler at column 0: got (%d, %d), want (0, 1)", sx, span)
	}
}
