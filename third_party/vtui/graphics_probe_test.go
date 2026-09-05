package vtui

import "testing"

func TestParseDA1Sixel(t *testing.T) {
	cases := []struct {
		resp string
		want bool
	}{
		{"\x1b[?62;4;6c", true},     // VT340
		{"\x1b[?64;4;6;7;8c", true}, // Windows Terminal style
		{"\x1b[?4c", true},          // bare sixel parameter
		{"garbage\x1b[?4c", true},   // stray bytes before the answer
		{"\x1b[?64;1;2c", false},    // no sixel extension
		{"\x1b[?1;2c", false},       //
		{"\x1b[?40c", false},        // 40 is not the 4 extension
		{"", false},                 // no answer at all
		{"\x1b[?62;44c", false},     // 44 is a level, not the 4 extension
	}
	for _, tc := range cases {
		if got := parseDA1Sixel(tc.resp); got != tc.want {
			t.Errorf("parseDA1Sixel(%q) = %v, want %v", tc.resp, got, tc.want)
		}
	}
}

func TestDA1ResponseComplete(t *testing.T) {
	if !da1ResponseComplete("\x1b[?62;4c") {
		t.Error("a terminated DA1 answer is complete")
	}
	if da1ResponseComplete("\x1b[?62;4") {
		t.Error("a DA1 answer without its terminating 'c' is not complete")
	}
	if da1ResponseComplete("") {
		t.Error("an empty buffer is not complete")
	}
}
