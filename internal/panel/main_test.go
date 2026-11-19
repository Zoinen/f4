package panel

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

// TestMain sets up what every panel test needs before any of them draws.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, installTestSeams, unmountTestFilesystems))
}

func TestPanelTestsUseIsolatedProfile(t *testing.T) {
	want := filepath.Join(testutil.ConfigDir(), "f4", "settings.ini")
	if got := config.GetUserConfigIniPath(); got != want {
		t.Fatalf("settings path = %q, want isolated path %q", got, want)
	}
}

func installTestSeams() {
	// macOS ignores XDG_CONFIG_HOME when resolving the application profile.
	// Some panel tests save settings, so redirect the resolver before any run.
	config.UserConfigDir = func() (string, error) { return testutil.ConfigDir(), nil }
	config.ResetConfigDirForTest()

	// f4 extends vtui's palette past its last index, and any widget drawn with
	// one of the extra colours indexes past the end of the default one. Which
	// test draws first depends on the shuffle seed, so the palette is sized
	// here rather than left to whichever test happens to grow it.
	theme.SetDefaultF4Palette()
	semantic.SetPanelCatalogMetadataEnabled(true)
	semantic.SetPanelGroupingEnabled(true)
	fileops.StartQueueWorker()

	// Unit tests must never hand control to the user's desktop, and no frame
	// may fork the user's shell.
	DefaultExternalUICommandRunner = func(string, []string, string) error { return nil }
	SpawnLocalShellPTY = false

	// Copying files to the clipboard must not write the runner's real one.
	terminal.DisableSystemFileClipboard()
}

func unmountTestFilesystems() error {
	return errors.Join(fusefs.UnmountAll()...)
}
