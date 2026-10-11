package git

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
// the default one -- exactly what panicked statusPanel.Show (panel.go) with
// "index out of range [59] with length 59" (vtui.LastPaletteColor is 59):
// nothing in this package had grown vtui.Palette to fit internal/theme's own
// ColPanelText and friends before a test drew a panel with them. The same
// fix plugins/proclist/main_test.go already applies for its own panel.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
