package panel

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestQuickViewColoredCells(t *testing.T) {
	const base, hot = uint64(1), uint64(2)
	cells := quickViewColoredCells("ab\u0301c", 1, []uint64{base, hot, hot, base}, base, 6)
	if len(cells) != 6 {
		t.Fatalf("%d cells, want the row padded to 6", len(cells))
	}
	// Runes: a (1: hot), b (2: hot), combining acute (3: base), c (past attrs: base).
	// The acute differs in colour from b, so it starts a run of its own.
	if cells[0].Attributes != hot || cells[len(cells)-1].Attributes != base {
		t.Errorf("attributes %v", attrsOf(cells))
	}
	if cut := quickViewColoredCells("abcdef", 0, nil, base, 4); len(cut) != 4 || rune(cut[3].Char) != '…' {
		t.Errorf("cut row %q", string(runesOf(cut)))
	}
}

func attrsOf(cells []vtui.CharInfo) []uint64 {
	out := make([]uint64, len(cells))
	for i, c := range cells {
		out[i] = c.Attributes
	}
	return out
}

func runesOf(cells []vtui.CharInfo) []rune {
	out := make([]rune, len(cells))
	for i, c := range cells {
		out[i] = rune(c.Char)
	}
	return out
}
