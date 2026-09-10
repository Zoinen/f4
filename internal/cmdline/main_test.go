package cmdline

import (
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"os"
	"testing"
)

// Components use f4's extended palette, installed by the composition root at runtime.
func TestMain(m *testing.M) { os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil)) }
