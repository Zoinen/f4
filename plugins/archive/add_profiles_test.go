package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func useTempProfiles(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive_profiles.json")
	old := addArchiveProfilesPath
	t.Cleanup(func() { addArchiveProfilesPath = old })
	addArchiveProfilesPath = func() string { return path }
	return path
}

// A profile keeps the options of the window under a name and puts them back
// when chosen; the password is never written to the disk (f4#2002, part 5).
func TestAddArchiveProfileRoundTrip(t *testing.T) {
	path := useTempProfiles(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	seven := addArchiveFormatIndex(t, ".7z")
	d.format.Menu.SetSelectPos(seven)
	d.format.Menu.OnAction(seven)
	d.solid.State = 0
	d.split.State = 1
	d.split.OnChange(1)
	d.volumeSize.Edit.SetText("100M")
	d.deleteAfter.State = 1
	d.filter.SetText("*.txt")
	d.password.SetText("secret-password")
	d.profile.Edit.SetText("backup")
	d.storeProfile()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-password") {
		t.Fatal("the password was written to the profiles file")
	}

	e := newAddArchiveDialog("photos.zip", func(addArchiveSettings) {})
	idx := -1
	for i, it := range e.profile.Menu.Items {
		if it.Text == "backup" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("saved profile not offered: %v", e.profile.Menu.Items)
	}
	e.profile.Menu.SetSelectPos(idx)
	e.profile.Menu.OnAction(idx)

	if e.name.GetText() != "photos.7z" || e.format.Menu.SelectPos != seven {
		t.Errorf("format not applied: %q", e.name.GetText())
	}
	if e.solid.State != 0 || e.split.State != 1 || e.volumeSize.Edit.GetText() != "100M" || e.volumeSize.IsDisabled() {
		t.Error("solid or volumes not applied")
	}
	if e.deleteAfter.State != 1 || e.filter.GetText() != "*.txt" {
		t.Error("delete after or filter not applied")
	}
	if e.password.GetText() != "" {
		t.Error("a profile filled in a password")
	}

	e.profile.Edit.SetText("backup")
	e.dropProfile()
	if _, ok := loadAddArchiveProfiles()["backup"]; ok || len(e.profile.Menu.Items) != 0 {
		t.Error("the profile was not deleted")
	}
}

// A profiles file that cannot be read leaves no profiles, not a broken window.
func TestAddArchiveProfilesFileUnreadable(t *testing.T) {
	path := useTempProfiles(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadAddArchiveProfiles(); len(got) != 0 {
		t.Fatalf("profiles %v", got)
	}
}
