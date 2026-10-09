package filemask

import "testing"

// The syntax this package promises is far2l's CFileMask, so the table is the
// four things that syntax says: "," and ";" are alternatives, "|" starts an
// exclude side that vetoes, "/.../" is a regular expression, and "*.*" means
// everything rather than only names with a dot.
func TestMatch(t *testing.T) {
	cases := []struct {
		name, mask string
		ignoreCase bool
		want       bool
	}{
		{"report.docx", "*.docx", false, true},
		{"report.docx", "*.odt", false, false},
		{"report.docx", "*.odt,*.docx", false, true},
		{"report.docx", "*.odt;*.docx", false, true},
		{"report.DOCX", "*.docx", false, false},
		{"report.DOCX", "*.docx", true, true},

		// The exclude side vetoes a name the include side matched.
		{"report.docx", "*.docx|report.docx", false, false},
		{"budget.docx", "*.docx|report.docx", false, true},

		// A regex section keeps its commas, and honours ignoreCase.
		{"volume.z01", "/^.+[.]z[0-9]{2}$/", false, true},
		{"volume.zip", "/^.+[.]z[0-9]{2}$/", false, false},
		{"VOLUME.Z01", "/^.+[.]z[0-9]{2}$/", true, true},
		{"a,b.txt", "/^a,b[.]txt$/", false, true},

		// far2l's own quirk: star-dot-star matches extension-less names too.
		{"noextension", "*.*", false, true},
		{"noextension", "*", false, true},

		// Nothing matches nothing, and a mask that cannot be parsed matches
		// nothing rather than everything.
		{"report.docx", "", false, false},
		{"", "*.docx", false, false},
		{"report.docx", "/(unclosed/", false, false},
		{"report.docx", "|*.docx", false, false},
	}

	for _, c := range cases {
		if got := Match(c.name, c.mask, c.ignoreCase); got != c.want {
			t.Errorf("Match(%q, %q, ignoreCase=%v) = %v, want %v", c.name, c.mask, c.ignoreCase, got, c.want)
		}
	}
}
