package panel

import (
	"errors"
	"os"
	"testing"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

// TestMain sets up what every panel test needs before any of them draws.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, installTestSeams, unmountTestFilesystems))
}

func installTestSeams() {
	// f4 extends vtui's palette past its last index, and any widget drawn with
	// one of the extra colours indexes past the end of the default one. Which
	// test draws first depends on the shuffle seed, so the palette is sized
	// here rather than left to whichever test happens to grow it.
	theme.SetDefaultF4Palette()
	semantic.SetPanelCatalogMetadataEnabled(true)
	fileops.StartQueueWorker()

	// Unit tests must never hand control to the user's desktop, and no frame
	// may fork the user's shell.
	DefaultExternalUICommandRunner = func(string, []string, string) error { return nil }
	SpawnLocalShellPTY = false
}

func unmountTestFilesystems() error {
	return errors.Join(fusefs.UnmountAll()...)
}
