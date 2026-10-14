package sheet

import "testing"

// f4#1892 (nz256): TSV and semicolon files open in columns, not as one.
func TestDetectCSVSeparator(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       rune
	}{
		{"comma", "a,b,c\n1,2,3\n", ','},
		{"tab", "a\tb\tc\n1\t2\t3\n", '\t'},
		{"semicolon with decimal commas", "name;price\napple;1,5\npear;2,25\n", ';'},
		{"comma inside quotes", "\"a;b\",c\n\"x;y\",z\n", ','},
		{"one column", "alpha\nbeta\n", ','},
		{"inconsistent semicolons", "a;b,c\nd,e\n", ','},
		{"cut last line", "a\tb\tc\n1\t2\t3\n4\t5", '\t'},
		{"empty", "", ','},
	} {
		if got := DetectCSVSeparator([]byte(tc.text)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
