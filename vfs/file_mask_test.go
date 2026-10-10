package vfs

import "testing"

// TestMatchFileMask exercises MatchFileMask's far2l-style mask syntax end to
// end: comma/semicolon separated glob includes, a pipe-separated exclude
// section, and /.../ wrapped regular expressions, with and without
// ignoreCase.
func TestMatchFileMask(t *testing.T) {
	cases := []struct {
		name       string
		fileName   string
		mask       string
		ignoreCase bool
		want       bool
	}{
		{"empty mask", "foo.txt", "", false, false},
		{"whitespace-only mask", "foo.txt", "   ", false, false},
		{"empty name", "", "*.txt", false, false},

		{"simple glob match", "foo.txt", "*.txt", false, true},
		{"simple glob no match", "foo.txt", "*.go", false, false},

		{"comma separated includes, first matches", "foo.go", "*.go,*.txt", false, true},
		{"comma separated includes, second matches", "foo.txt", "*.go,*.txt", false, true},
		{"comma separated includes, none match", "foo.md", "*.go,*.txt", false, false},
		{"semicolon separated includes", "foo.txt", "*.go;*.txt", false, true},

		{"exclude removes matching name", "foo.txt", "*.txt|foo.txt", false, false},
		{"exclude leaves other names", "bar.txt", "*.txt|foo.txt", false, true},
		{"exclude list with multiple entries", "bar.txt", "*.txt|foo.txt,bar.txt", false, false},

		{"star-dot-star treated as star, matches extensionless name", "foo", "*.*", false, true},
		{"star-dot-star treated as star, matches dotted name", "foo.txt", "*.*", false, true},

		{"case sensitive glob rejects mismatched case", "FOO.TXT", "*.txt", false, false},
		{"ignoreCase glob accepts mismatched case", "FOO.TXT", "*.txt", true, true},

		{"regex mask matches prefix", "foobar", "/^foo/", false, true},
		{"regex mask rejects non-matching name", "barfoo", "/^foo/", false, false},
		{"regex mask honours ignoreCase", "FOOBAR", "/^foo/", true, true},
		{"regex mask case sensitive by default", "FOOBAR", "/^foo/", false, false},
		{"regex alternation across an internal pipe stays one pattern", "b", "/^(a|b)$/", false, true},
		{"regex alternation across an internal pipe rejects other names", "c", "/^(a|b)$/", false, false},
		{"invalid regex fails closed instead of panicking", "foo", "/(/", false, false},

		{"leading/trailing whitespace around items is trimmed", "foo.txt", " *.txt , *.go ", false, true},
		{"empty include before pipe yields no includes", "foo.txt", "|*.txt", false, false},
		{"blank items between separators are ignored", "foo.txt", "*.txt,,", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchFileMask(tc.fileName, tc.mask, tc.ignoreCase); got != tc.want {
				t.Errorf("MatchFileMask(%q, %q, %v) = %v, want %v",
					tc.fileName, tc.mask, tc.ignoreCase, got, tc.want)
			}
		})
	}
}

// TestSplitFileMaskOnPipe pins the '/' depth tracking that lets a regex
// section carry its own '|' (regex alternation) without being mistaken for
// the include/exclude separator.
func TestSplitFileMaskOnPipe(t *testing.T) {
	cases := []struct {
		name    string
		mask    string
		wantInc string
		wantExc string
		wantHas bool
	}{
		{"no pipe at all", "*.txt", "*.txt", "", false},
		{"plain pipe splits include and exclude", "*.txt|foo.txt", "*.txt", "foo.txt", true},
		{"pipe inside a regex section is not a separator", "/a|b/", "/a|b/", "", false},
		{"pipe outside a regex section still splits", "/a|b/|c", "/a|b/", "c", true},
		{"unterminated slash still toggles depth for the rest of the mask", "/a|b", "/a|b", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inc, exc, has := splitFileMaskOnPipe(tc.mask)
			if inc != tc.wantInc || exc != tc.wantExc || has != tc.wantHas {
				t.Errorf("splitFileMaskOnPipe(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.mask, inc, exc, has, tc.wantInc, tc.wantExc, tc.wantHas)
			}
		})
	}
}

// TestSplitFileMaskCommaSemi pins the same depth tracking for the
// comma/semicolon separator used within one side (include or exclude) of a
// mask, including a comma embedded in a regex section.
func TestSplitFileMaskCommaSemi(t *testing.T) {
	cases := []struct {
		name string
		side string
		want []string
	}{
		{"empty side yields nil", "", nil},
		{"single item", "*.txt", []string{"*.txt"}},
		{"comma separated", "*.txt,*.go", []string{"*.txt", "*.go"}},
		{"semicolon separated", "*.txt;*.go", []string{"*.txt", "*.go"}},
		{"mixed separators with whitespace", " *.txt , *.go ; *.md ", []string{"*.txt", "*.go", "*.md"}},
		{"blank items are dropped", "*.txt,,;*.go", []string{"*.txt", "*.go"}},
		{"comma embedded in a regex section is preserved", "/a,b/,c", []string{"/a,b/", "c"}},
		{"all blank items yield nil", " , ; ", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitFileMaskCommaSemi(tc.side)
			if len(got) != len(tc.want) {
				t.Fatalf("splitFileMaskCommaSemi(%q) = %#v, want %#v", tc.side, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitFileMaskCommaSemi(%q)[%d] = %q, want %q", tc.side, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestMatchOneFileMask pins the single-item matcher directly, including the
// invalid-regex-fails-closed path that MatchFileMask relies on to avoid
// panicking on a malformed /.../  section.
func TestMatchOneFileMask(t *testing.T) {
	if !matchOneFileMask("foo.txt", "*.txt", false) {
		t.Error("expected glob match")
	}
	if matchOneFileMask("foo.txt", "*.go", false) {
		t.Error("expected glob mismatch")
	}
	if !matchOneFileMask("FOO.TXT", "*.txt", true) {
		t.Error("expected case-insensitive glob match")
	}
	if !matchOneFileMask("foo", "*.*", false) {
		t.Error("expected *.* to behave like *")
	}
	if !matchOneFileMask("foobar", "/^foo/", false) {
		t.Error("expected regex match")
	}
	if matchOneFileMask("foobar", "/(/", false) {
		t.Error("expected invalid regex to fail closed, not match")
	}
	// A malformed glob pattern (unterminated character class) must also fail
	// closed rather than propagate filepath.Match's error.
	if matchOneFileMask("foo.txt", "[", false) {
		t.Error("expected malformed glob to fail closed")
	}
}
