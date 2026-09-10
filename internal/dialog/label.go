package dialog

import "github.com/mattn/go-runewidth"

// PadLabel pads a dialog label to twelve columns so the fields beside a column
// of labels line up. Width is counted in display cells, not bytes: a label
// translated into a wide script pads to the same visual column.
func PadLabel(s string) string {
	for runewidth.StringWidth(s) < 12 {
		s += " "
	}
	return s
}
