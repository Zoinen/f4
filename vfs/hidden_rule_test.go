package vfs

import "testing"

func TestHiddenByRule(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		attrs uint32
		have  bool
		posix bool
		want  bool
	}{
		// posix personality: the dot prefix and nothing else.
		{"posix dotfile", ".bashrc", 0, false, true, true},
		{"posix plain file", "notes.txt", 0, false, true, false},
		{"posix ignores a hidden attribute that should not exist", "notes.txt", win32HiddenAttr, true, true, false},
		// windows personality: the attribute wins, the dot prefix still counts.
		{"windows hidden attribute", "notes.txt", win32HiddenAttr, true, false, true},
		{"windows other attributes stay visible", "notes.txt", 0x20, true, false, false},
		{"windows dot prefix without attributes", ".config", 0, false, false, true},
		{"windows dot prefix with clear attributes", ".config", 0x20, true, false, true},
		{"windows plain file without attributes", "notes.txt", 0, false, false, false},
		{"windows hidden bit is ignored when attributes are absent", "notes.txt", win32HiddenAttr, false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hiddenByRule(tc.entry, tc.attrs, tc.have, tc.posix); got != tc.want {
				t.Errorf("hiddenByRule(%q, %#x, have=%v, posix=%v) = %v, want %v",
					tc.entry, tc.attrs, tc.have, tc.posix, got, tc.want)
			}
		})
	}
}
