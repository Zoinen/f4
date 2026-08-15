package vtui

import "testing"

// The reported symptom was syntax colouring sliding out of place after an
// emoji. A cluster spending several runes on one cell is exactly where an
// attribute slice and a cell slice come apart, so this is the case to pin.
func TestStringToCharInfoWithAttrs_ColourChangeLandsAfterAnEmoji(t *testing.T) {
	const s = "ab\U0001F600cd"
	const hot = uint64(0x40)

	// Runes: 'a', 'b', the emoji, 'c', 'd'. The colour changes right after it.
	cells := StringToCharInfoWithAttrs(s, []uint64{0, 0, 0, hot, hot}, 0)

	// Cells: 'a', 'b', the emoji, its filler, 'c', 'd'.
	if len(cells) != 6 {
		t.Fatalf("Expected 6 cells for %q, got %d", s, len(cells))
	}
	if cells[3].Char != WideCharFiller {
		t.Error("Expected the emoji to claim two columns")
	}
	for i, want := range []uint64{0, 0, 0, 0, hot, hot} {
		if cells[i].Attributes != want {
			t.Errorf("Cell %d carries attribute %#x, expected %#x", i, cells[i].Attributes, want)
		}
	}
}

func TestStringToCharInfoWithAttrs_TheBaseOfAClusterWins(t *testing.T) {
	const s = "e\u0301x"
	const markAttr = uint64(0x20)
	const tailAttr = uint64(0x30)

	// Runes: 'e', the combining acute, 'x'. The mark carries a colour of its
	// own, and has no cell to put it in.
	cells := StringToCharInfoWithAttrs(s, []uint64{0, markAttr, tailAttr}, 0)

	if len(cells) != 2 {
		t.Fatalf("Expected 2 cells for %q, got %d", s, len(cells))
	}
	if cells[0].Attributes != 0 {
		t.Errorf("Expected the base of the cluster to keep its attribute, got %#x", cells[0].Attributes)
	}
	if cells[1].Attributes != tailAttr {
		t.Errorf("Expected the rune after the cluster to keep its attribute, got %#x", cells[1].Attributes)
	}
}

func TestStringToCharInfoWithAttrs_ShortAndMissingSlices(t *testing.T) {
	const s = "abc"
	const base = uint64(0x11)
	const hot = uint64(0x22)

	cells := StringToCharInfoWithAttrs(s, []uint64{hot}, base)
	if len(cells) != 3 {
		t.Fatalf("Expected 3 cells for %q, got %d", s, len(cells))
	}
	if cells[0].Attributes != hot || cells[1].Attributes != base || cells[2].Attributes != base {
		t.Errorf("Expected the base attribute past the end of the slice, got %#x, %#x, %#x",
			cells[0].Attributes, cells[1].Attributes, cells[2].Attributes)
	}

	for i, c := range StringToCharInfoWithAttrs(s, nil, base) {
		if c.Attributes != base {
			t.Errorf("Cell %d carries attribute %#x, expected the base %#x", i, c.Attributes, base)
		}
	}
}

// Whatever the attributes say, the layout must not move. If this ever fails,
// every widget drawing highlighted text is wrong.
func TestStringToCharInfoWithAttrs_WidthIsUnchanged(t *testing.T) {
	for _, s := range []string{
		"\u0928\u092e\u0938\u094d\u0924\u0947",
		"\U0001F468\u200D\U0001F469\u200D\U0001F466",
		"\u2764\uFE0F",
		"A\u4E16B",
		"e\u0301",
		"abc",
	} {
		attrs := make([]uint64, len([]rune(s)))
		for i := range attrs {
			attrs[i] = uint64(i)
		}
		got := len(StringToCharInfoWithAttrs(s, attrs, 0))
		if want := StringWidth(s); got != want {
			t.Errorf("%q produced %d cells, expected %d", s, got, want)
		}
	}
}
