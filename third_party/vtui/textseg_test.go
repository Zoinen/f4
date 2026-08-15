package vtui

import "testing"

func TestClusterWidth_Combining(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"ascii", "A", 1},
		{"cjk", "世", 2},
		{"latin with acute", "e\u0301", 1},
		{"devanagari base", "क", 1},
		{"devanagari with spacing matra", "का", 2},
		{"devanagari with virama", "स\u094D", 1},
		{"emoji", "😀", 2},
		{"emoji with presentation selector", "❤\uFE0F", 2},
		{"emoji with text selector", "❤\uFE0E", 1},
		{"emoji with skin tone", "👍🏽", 2},
		{"emoji zwj family", "👨\u200D👩\u200D👦", 2},
		{"regional indicator flag", "🇩🇪", 2},
		{"keycap", "1\uFE0F\u20E3", 2},
		{"lone combining mark", "\u0301", 1},
	}
	for _, c := range cases {
		if got := ClusterWidth(c.in); got != c.want {
			t.Errorf("%s: ClusterWidth(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

func TestStringWidth_MatchesWcwidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"नमस्ते", 4},
		{"مرحبا", 5},
		{"A世B", 4},
		{"👨\u200D👩\u200D👦!", 3},
	}
	for _, c := range cases {
		if got := StringWidth(c.in); got != c.want {
			t.Errorf("StringWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestRegisterCluster_RoundTrip(t *testing.T) {
	plain := RegisterCluster("A")
	if IsCompChar(plain) {
		t.Errorf("single rune cluster should stay a plain rune, got %X", plain)
	}
	if plain != uint64('A') {
		t.Errorf("expected plain rune 'A', got %X", plain)
	}

	family := "👨\u200D👩\u200D👦"
	id := RegisterCluster(family)
	if !IsCompChar(id) {
		t.Fatalf("multi rune cluster should be a composite id, got %X", id)
	}
	if id == WideCharFiller {
		t.Fatal("composite id must never collide with WideCharFiller")
	}
	if got := CellString(id); got != family {
		t.Errorf("CellString round trip: got %q, want %q", got, family)
	}
	if got := RegisterCluster(family); got != id {
		t.Errorf("registry should be stable: got %X, want %X", got, id)
	}
	if got := CellBaseRune(id); got != '👨' {
		t.Errorf("CellBaseRune: got %q, want %q", got, '👨')
	}
	if got := len(CellRunes(id)); got != 5 {
		t.Errorf("CellRunes: got %d runes, want 5", got)
	}
}

func TestCellString_SpecialCells(t *testing.T) {
	if got := CellString(WideCharFiller); got != "" {
		t.Errorf("filler should render as nothing, got %q", got)
	}
	if got := CellString(0); got != " " {
		t.Errorf("empty cell should render as a space, got %q", got)
	}
	if got := CellBaseRune(WideCharFiller); got != 0 {
		t.Errorf("filler has no base rune, got %q", got)
	}
}

func TestStringToCharInfo_KeepsMarksWithBase(t *testing.T) {
	ci := StringToCharInfo("e\u0301X", 7)
	if len(ci) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(ci))
	}
	if CellString(ci[0].Char) != "e\u0301" {
		t.Errorf("cell 0: got %q, want %q", CellString(ci[0].Char), "e\u0301")
	}
	if ci[1].Char != 'X' {
		t.Errorf("cell 1: got %X, want 'X'", ci[1].Char)
	}
	for i, c := range ci {
		if c.Attributes != 7 {
			t.Errorf("cell %d lost its attributes: %d", i, c.Attributes)
		}
	}
}

func TestStringToCharInfo_Devanagari(t *testing.T) {
	// The whole point of the exercise: a Hindi word must claim exactly as many
	// cells as a wcwidth terminal gives it, or every dialog around it shifts.
	ci := StringToCharInfo("नमस्ते", 0)
	if len(ci) != 4 {
		t.Fatalf("expected 4 cells for नमस्ते, got %d", len(ci))
	}
	for i, c := range ci {
		if c.Char == WideCharFiller {
			t.Errorf("cell %d: no cell of this word is wide", i)
		}
	}
}

func TestStringToCharInfo_EmojiSequenceIsOneCell(t *testing.T) {
	ci := StringToCharInfo("👨\u200D👩\u200D👦", 0)
	if len(ci) != 2 {
		t.Fatalf("expected 2 cells (one wide char), got %d", len(ci))
	}
	if !IsCompChar(ci[0].Char) {
		t.Errorf("cell 0 should hold a composite cluster, got %X", ci[0].Char)
	}
	if ci[1].Char != WideCharFiller {
		t.Errorf("cell 1 should be a filler, got %X", ci[1].Char)
	}
}

func TestStringToCharInfoHighlighted_HotkeyAfterCluster(t *testing.T) {
	// The hotkey sits after a combining sequence, so the rune index of the
	// hotkey and the cell index no longer agree.
	cells, hk := StringToCharInfoHighlighted("e\u0301&Xy", 1, 2)
	if hk != 'x' {
		t.Fatalf("hotkey: got %q, want 'x'", hk)
	}
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(cells))
	}
	if cells[1].Attributes != 2 {
		t.Errorf("hotkey cell should be highlighted, got attr %d", cells[1].Attributes)
	}
	if cells[0].Attributes != 1 || cells[2].Attributes != 1 {
		t.Error("only the hotkey cell should be highlighted")
	}
}

func TestFillCharInfoWithSelection_ClusterBounds(t *testing.T) {
	// "e" plus acute is 3 bytes; selecting from byte 3 must select only "X".
	data := []byte("e\u0301X")
	cells := FillCharInfoWithSelection(nil, data, 1, 2, 0, 3, 4)
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
	if cells[0].Attributes != 1 {
		t.Errorf("cluster before selection should keep the default attr, got %d", cells[0].Attributes)
	}
	if cells[1].Attributes != 2 {
		t.Errorf("selected cluster should use the selection attr, got %d", cells[1].Attributes)
	}
}

func TestTruncateString_NeverSplitsACluster(t *testing.T) {
	if got := TruncateString("A世B", 2, ""); got != "A" {
		t.Errorf("wide char must not be half cut: got %q, want %q", got, "A")
	}
	if got := TruncateString("A世B", 3, ""); got != "A世" {
		t.Errorf("got %q, want %q", got, "A世")
	}
	if got := TruncateString("e\u0301XY", 2, ""); got != "e\u0301X" {
		t.Errorf("combining mark must travel with its base: got %q", got)
	}
	if got := TruncateString("abcdef", 4, "…"); got != "abc…" {
		t.Errorf("got %q, want %q", got, "abc…")
	}
	if got := TruncateString("abc", 10, ""); got != "abc" {
		t.Errorf("short string must be returned as is, got %q", got)
	}
}

func TestSanitizeCluster_Controls(t *testing.T) {
	if _, w := SanitizeCluster("\n"); w != 0 {
		t.Errorf("newline must not take a cell, got width %d", w)
	}
	if s, w := SanitizeCluster("\x01"); s != "·" || w != 1 {
		t.Errorf("control char: got %q width %d", s, w)
	}
	if s, _ := SanitizeCluster("\uFFFD"); s != "?" {
		t.Errorf("replacement char: got %q", s)
	}
}

func TestEmojiPresentationWideSetting(t *testing.T) {
	old := EmojiPresentationWide
	defer func() { EmojiPresentationWide = old }()

	EmojiPresentationWide = false
	if got := ClusterWidth("❤\uFE0F"); got != 1 {
		t.Errorf("with the setting off the base width wins: got %d, want 1", got)
	}
	EmojiPresentationWide = true
	if got := ClusterWidth("❤\uFE0F"); got != 2 {
		t.Errorf("with the setting on emoji presentation is wide: got %d, want 2", got)
	}
}
