package archive

import (
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

// "Save settings" keeps what the window shows, and the window opens with it
// next time; creating an archive afterwards keeps them (f4#2002).
func TestAddArchiveDialogSaveSettings(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldPath := addArchivePreferencesPath
	prefs := filepath.Join(t.TempDir(), "archive_preferences.json")
	addArchivePreferencesPath = func() string { return prefs }
	t.Cleanup(func() { addArchivePreferencesPath = oldPath })

	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	sevenIdx := addArchiveFormatIndex(t, ".7z")
	d.format.Menu.SetSelectPos(sevenIdx)
	d.format.Menu.OnAction(sevenIdx)
	d.solid.State = 0
	d.deleteAfter.State = 1
	d.keepExtension.State = 0
	d.saveDefaults.OnClick()

	again := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	if again.format.Menu.SelectPos != sevenIdx || again.name.GetText() != "docs.7z" {
		t.Fatalf("reopened with format %d and name %q, want 7z and docs.7z", again.format.Menu.SelectPos, again.name.GetText())
	}
	if again.solid.State != 0 || again.deleteAfter.State != 1 || again.keepExtension.State != 0 {
		t.Fatalf("reopened with solid %d, delete after %d, keep extension %d; want 0, 1, 0",
			again.solid.State, again.deleteAfter.State, again.keepExtension.State)
	}

	again.create.OnClick()
	if loadAddArchivePreferences().Defaults == nil {
		t.Fatal("creating an archive dropped the saved settings")
	}
}

// "Save settings" stands to the right of "Cancel", not between "Create" and
// "Cancel" (f4#2002, mitek1977-coder 18-10-2026), and Tab goes in that order.
func TestAddArchiveDialogButtonOrder(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	if d.create.X1 >= d.cancel.X1 || d.cancel.X1 >= d.saveDefaults.X1 {
		t.Fatalf("button columns: create %d, cancel %d, save settings %d; want create < cancel < save settings",
			d.create.X1, d.cancel.X1, d.saveDefaults.X1)
	}
}
