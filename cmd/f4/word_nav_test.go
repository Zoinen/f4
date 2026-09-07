package main

import "testing"

// nextWordStop replays the pure part of the editor jump loops: step once, then
// keep going while no stop condition holds. Positions are rune indices.
func nextWordStop(s string, pos int, right, selecting bool) int {
	r := []rune(s)
	if right {
		if pos >= len(r) {
			return pos
		}
		pos++
		for pos < len(r) && !stopBeforeRuneRight(r[pos-1], r[pos], selecting) {
			pos++
		}
		return pos
	}
	if pos <= 0 {
		return 0
	}
	pos--
	for pos > 0 && !stopBeforeRuneLeft(r[pos-1], r[pos], selecting) {
		pos--
	}
	return pos
}

func TestWordNav_CharCategories(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{' ', catSpace},
		{'\t', catSpace},
		{'.', catDivider},
		{'/', catDivider},
		{'|', catDivider},
		{'a', catWord},
		{'0', catWord},
		{'_', catWord},
		{'$', catWord},
		{'#', catWord},
		{'@', catWord},
		{'я', catWord},
	}
	for _, c := range cases {
		if got := getCharCategory(c.r); got != c.want {
			t.Errorf("getCharCategory(%q) = %d, want %d", c.r, got, c.want)
		}
	}
}

// Expectations below were produced by replaying the far2l algorithms
// (edit.cpp KEY_CTRLLEFT/KEY_CTRLRIGHT, editor.cpp KEY_CTRLSHIFTLEFT/RIGHT)
// on the same inputs.
func TestWordNav_Far2lJumps(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		pos       int
		right     bool
		selecting bool
		want      int
	}{
		{"space right first", "foo bar", 0, true, false, 4},
		{"space right second", "foo bar", 4, true, false, 7},
		{"space left first", "foo bar", 7, false, false, 4},
		{"space left second", "foo bar", 4, false, false, 0},

		{"divider right first", "foo.bar", 0, true, false, 3},
		{"divider right second", "foo.bar", 3, true, false, 7},
		{"divider left first", "foo.bar", 7, false, false, 4},
		{"divider left second", "foo.bar", 4, false, false, 0},

		{"divider run right", "...///", 0, true, false, 6},
		{"divider run left", "...///", 6, false, false, 0},

		{"select space right first", "foo bar", 0, true, true, 3},
		{"select space right second", "foo bar", 3, true, true, 7},
		{"select space left first", "foo bar", 7, false, true, 4},
		{"select space left second", "foo bar", 4, false, true, 0},

		{"select divider right first", "foo.bar", 0, true, true, 3},
		{"select divider right second", "foo.bar", 3, true, true, 7},
		{"select divider left first", "foo.bar", 7, false, true, 4},
		{"select divider left second", "foo.bar", 4, false, true, 0},

		{"select divider run right", "...///", 0, true, true, 6},
		{"select divider run left", "...///", 6, false, true, 0},

		{"leading spaces right", "  ab", 0, true, false, 2},
		{"tabs are spaces", "a\t\tb", 1, true, false, 3},
	}

	for _, c := range cases {
		got := nextWordStop(c.text, c.pos, c.right, c.selecting)
		if got != c.want {
			t.Errorf("%s: nextWordStop(%q, %d, right=%v, sel=%v) = %d, want %d",
				c.name, c.text, c.pos, c.right, c.selecting, got, c.want)
		}
	}
}

// A divider run must never be split, no matter how many different divider
// characters it contains. This is the regression guard for issue #280.
func TestWordNav_DividerRunIsNotSplit(t *testing.T) {
	if got := nextWordStop("a.-/b", 0, true, false); got != 1 {
		t.Errorf("stop at end of word: got %d, want 1", got)
	}
	if got := nextWordStop("a.-/b", 1, true, false); got != 5 {
		t.Errorf("divider run must be crossed in one jump: got %d, want 5", got)
	}
}
