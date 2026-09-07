package main

import "testing"

// Ported straight from far2l's ConvertWildcards, so the cases are the ones
// that function's rules produce — including the odd ones.
func TestApplyFileMask(t *testing.T) {
	cases := []struct{ name, mask, want string }{
		{"file.txt", "*.bak", "file.bak"},
		{"file.txt", "*.*", "file.txt"},
		{"file.txt", "*", "file.txt"},
		{"file.txt", "*.1", "file.1"},
		{"file.txt", "?.txt", "f.txt"},
		{"file.txt", "z*.*", "zile.txt"},
		{"file.tar.gz", "*.bak", "file.tar.bak"},
		{"noext", "*.bak", "noext.bak"},
		{"noext", "*.*", "noext"},
		{"file.txt", "plain.txt", "plain.txt"},
	}
	for _, c := range cases {
		if got := applyFileMask(c.name, c.mask); got != c.want {
			t.Errorf("applyFileMask(%q, %q) = %q, want %q", c.name, c.mask, got, c.want)
		}
	}
}

func TestDestMask(t *testing.T) {
	cases := []struct{ in, mask, dir string }{
		{"*.1", "*.1", "."},
		{"/tmp/dest/*.bak", "*.bak", "/tmp/dest/"},
		{"/tmp/dest", "", ""},
		{"/tmp/dest/", "", ""},
		{"", "", ""},
		// A wildcard in a directory component is not a rename mask.
		{"/tmp/a*/dest", "", ""},
	}
	for _, c := range cases {
		if got := destMask(c.in); got != c.mask {
			t.Errorf("destMask(%q) = %q, want %q", c.in, got, c.mask)
		}
		if c.mask != "" {
			if got := destWithoutMask(c.in); got != c.dir {
				t.Errorf("destWithoutMask(%q) = %q, want %q", c.in, got, c.dir)
			}
		}
	}
}
