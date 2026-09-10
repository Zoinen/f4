package dialog

import (
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"os"
	"testing"
)

// f4 extends vtui's palette past its last index, and a dialog drawn with one of
// the extra colours indexes past the end of the default one. Which test draws
// first depends on the shuffle seed, so the palette is sized here.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
