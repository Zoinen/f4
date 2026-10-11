package proclist

import (
	"os"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

// TestMain sets up what every test in this package needs before any of them
// draws: f4 extends vtui's palette past its last index (internal/theme's
// ColPanel* constants among them), and a widget drawn with one of those
// extra colors before the palette is sized for them indexes past the end of
// the default one. testutil.Main also gives every test a silent screen and
// checks that no frame manager or task pump -- notably procListPanel's own
// refresh goroutine -- outlives the test that started it.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
