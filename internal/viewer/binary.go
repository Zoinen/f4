package viewer

import (
	"unicode/utf8"
)

// LooksBinary decides whether a byte run should be shown as hex rather than as
// text. It is the viewer's rule, and the quick-view panel asks for the same
// answer so a file does not change form between the two.
func LooksBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if !utf8.Valid(b) {
		return true
	}
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}
