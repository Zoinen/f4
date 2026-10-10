package hostwidth

import "testing"

func TestTableIsSortedAndDisjoint(t *testing.T) {
	if len(spans) != 295 {
		t.Fatalf("%d ranges, the host's table has 295", len(spans))
	}
	for i, s := range spans {
		if s.lo > s.hi {
			t.Fatalf("range %d is inverted: %#x..%#x", i, s.lo, s.hi)
		}
		if i > 0 && spans[i-1].hi >= s.lo {
			t.Fatalf("range %d overlaps or precedes the previous one", i)
		}
	}
}

func TestLookupKnownCodepoints(t *testing.T) {
	cases := []struct {
		cp   rune
		want Width
	}{
		{'A', Narrow}, {' ', Narrow}, {'~', Narrow},
		{0x00a1, Ambiguous}, {0x0300, Ambiguous}, // even a combining mark takes a column of its own
		{0x4e00, Wide}, {0x20000, Wide}, {0x3fffd, Wide},
		{0xe0100, Ambiguous}, {0xf0000, Ambiguous}, {0x10fffd, Ambiguous},
		{0x00c0, Narrow}, // a plain Latin letter is not in the table
		{0x10fffe, Narrow},
	}
	for _, c := range cases {
		if got := Lookup(c.cp); got != c.want {
			t.Errorf("Lookup(%#x) = %v, want %v", c.cp, got, c.want)
		}
	}
}

// What is not a glyph cannot be measured, and the host then answers wide.
func TestLookupOfWhatIsNotAGlyphIsWide(t *testing.T) {
	for _, cp := range []rune{-1, 0xd800, 0xdfff, 0x110000} {
		if Lookup(cp) != Wide {
			t.Errorf("Lookup(%#x) is not wide", cp)
		}
	}
}

// The binary search agrees with a plain scan of the table over every codepoint.
func TestLookupMatchesALinearScan(t *testing.T) {
	scan := func(cp rune) Width {
		if cp >= 0x20 && cp <= 0x7e {
			return Narrow
		}
		for _, s := range spans {
			if cp >= s.lo && cp <= s.hi {
				return s.w
			}
		}
		return Narrow
	}
	for cp := rune(0); cp <= 0x10ffff; cp++ {
		if cp >= 0xd800 && cp <= 0xdfff {
			continue
		}
		if got, want := Lookup(cp), scan(cp); got != want {
			t.Fatalf("Lookup(%#x) = %v, scan says %v", cp, got, want)
		}
	}
}

func TestColumns(t *testing.T) {
	for cp, want := range map[rune]int{'A': 1, 0x4e00: 2, 0x00a1: 1, 0x0300: 1, 0x20000: 2} {
		if got := Columns(cp); got != want {
			t.Errorf("Columns(%#x) = %d, want %d", cp, got, want)
		}
	}
}
