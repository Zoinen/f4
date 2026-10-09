package svcmgr

import (
	"os"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

// TestMain grows vtui's palette to fit internal/theme's ColPanel* colors
// before any test draws a panel, as plugins/git and plugins/proclist do.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
