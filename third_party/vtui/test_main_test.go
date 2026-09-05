package vtui

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// mkGrid builds a buf/shadow pair of w*h cells filled with ch and attributes.
func mkGrid(w, h int, ch uint64, attr uint64) (buf, shadow []CharInfo) {
	buf = make([]CharInfo, w*h)
	shadow = make([]CharInfo, w*h)
	for i := range buf {
		buf[i] = CharInfo{Char: ch, Attributes: attr}
	}
	return buf, shadow
}
