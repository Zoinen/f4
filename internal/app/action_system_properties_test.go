package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
)

func TestAppsKeyHasAName(t *testing.T) {
	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_APPS}
	if got := keymap.EventToFarString(e); got != "Apps" {
		t.Fatalf("the Apps key is named %q, want \"Apps\"", got)
	}
}

// System properties remains a direct command after Apps moves to the
// cross-platform file context menu.
func TestAction_FileSystemProperties_OpensTheCursorItem(t *testing.T) {
	a, ok := action.Lookup("File.SystemProperties")
	if !ok {
		t.Fatal("File.SystemProperties is not registered")
	}
	if len(a.DefaultKeys) != 0 {
		t.Fatalf("File.SystemProperties must not compete with File.ContextMenu for Apps: %q", a.DefaultKeys)
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf := seedPanelForCopyName(t, tmp)
	var opened []string
	old := dialog.DefaultNativePropertiesOpener
	dialog.DefaultNativePropertiesOpener = func(path string) error {
		opened = append(opened, path)
		return nil
	}
	t.Cleanup(func() { dialog.DefaultNativePropertiesOpener = old })

	fsp := pf.GetActivePanel()
	fsp.SelectName("a.txt")
	actionFileSystemProperties(pf)
	fsp.SelectName("..")
	actionFileSystemProperties(pf)

	abs, err := fsp.Vfs.Abs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{fsp.Vfs.Join(abs, "a.txt"), abs}
	if len(opened) != 2 || opened[0] != want[0] || opened[1] != want[1] {
		t.Fatalf("opened %q, want %q", opened, want)
	}
}
