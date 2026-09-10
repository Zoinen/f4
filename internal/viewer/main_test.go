package viewer

import (
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"os"
	"testing"
)

// The viewer draws through vtui.Palette, which is shorter than f4's until the
// theme sizes it. In cmd/f4 that happened once in its own TestMain; a package
// with its own test binary has to do it for itself, or every render test
// indexes past the end of the default palette.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
