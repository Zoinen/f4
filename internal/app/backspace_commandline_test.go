package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// Backspace belongs to the command line while it holds text: it must erase a
// character and leave the panel in its directory, in every navigation mode
// (f4#1797). Going up on Backspace is for an empty line only.
func TestBackspaceWithTextOnCommandLineStaysInDirectory(t *testing.T) {
	for _, mode := range []config.PanelNavigationMode{config.NavigationClassic, config.NavigationVim} {
		t.Run(mode.String(), func(t *testing.T) {
			old := config.App
			defer func() { config.App = old }()
			config.App.NavigationMode = mode

			pf := seedPanelForRestore(t, []string{"a"})
			sub := filepath.Join(t.TempDir(), "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			fsp := pf.GetActivePanel()
			fsp.Vfs = vfs.NewOSVFS(sub)

			for _, ch := range "abc" {
				pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ch, VirtualKeyCode: vtinput.VK_A})
			}
			for _, ch := range []rune{0, '\b', 0x7f} {
				pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ch, VirtualKeyCode: vtinput.VK_BACK})
			}
			if got := pf.CmdLine.Edit.GetText(); got != "" {
				t.Fatalf("command line after three Backspaces = %q, want it erased", got)
			}
			if got := fsp.Vfs.GetPath(); got != sub {
				t.Fatalf("Backspace on a non-empty command line left %q for %q", sub, got)
			}
		})
	}
}
