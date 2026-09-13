package dialog

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

func TestFar3HistoryPathUTF16Profile(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		root := t.TempDir()
		profile := filepath.Join(root, "Профиль")
		if err := os.MkdirAll(profile, 0700); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(profile, "history.db")
		if err := os.WriteFile(want, nil, 0600); err != nil {
			t.Fatal(err)
		}
		units := utf16.Encode([]rune("\ufeff;Far Manager configuration\r\n[General]\r\nUseSystemProfiles=0\r\nUserLocalProfileDir=%FARHOME%/Профиль\r\n"))
		data := make([]byte, 2*len(units))
		for i, unit := range units {
			order.PutUint16(data[2*i:], unit)
		}
		if err := os.WriteFile(filepath.Join(root, "Far.exe.ini"), data, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Far3HistoryPath(root)
		if err != nil || got != want {
			t.Fatalf("UTF-16 profile: got %q, want %q: %v", got, want, err)
		}
	}
}

func TestFar3HistoryPathLocalInstallation(t *testing.T) {
	root := os.Getenv("F4_TEST_FAR3_INSTALLATION")
	if root == "" {
		t.Skip("set F4_TEST_FAR3_INSTALLATION for local path discovery")
	}
	got, err := Far3HistoryPath(root)
	want := filepath.Join(root, "Profile", "history.db")
	if err != nil || got != want {
		t.Fatalf("got %q, want %q: %v", got, want, err)
	}
}

func TestFar3HistoryPath(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "Profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(profile, "history.db")
	if err := os.WriteFile(db, nil, 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "Far.exe.ini")
	if err := os.WriteFile(config, []byte("[General]\nUseSystemProfiles=0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{root, profile, db, `"` + root + `"`} {
		got, err := Far3HistoryPath(source)
		if err != nil || got != db {
			t.Fatalf("%q => %q, %v", source, got, err)
		}
	}
	for _, mode := range []string{"1", "2"} {
		system := filepath.Join(t.TempDir(), "Far Manager")
		if err := os.MkdirAll(system, 0700); err != nil {
			t.Fatal(err)
		}
		systemDB := filepath.Join(system, "history.db")
		if err := os.WriteFile(systemDB, nil, 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("APPDATA", filepath.Dir(system))
		t.Setenv("LOCALAPPDATA", filepath.Dir(system))
		if err := os.WriteFile(config, []byte("[General]\nUseSystemProfiles="+mode+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := Far3HistoryPath(root); err != nil || got != systemDB {
			t.Fatalf("system profile %s: %q %v", mode, got, err)
		}
	}
	if err := os.WriteFile(config, []byte("[General]\nUseSystemProfiles=0\nUserProfileDir=absent\nUserLocalProfileDir=%FARHOME%/Profile\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := Far3HistoryPath(root); err != nil || got != db {
		t.Fatalf("local override: %q %v", got, err)
	}
	if _, err := Far3HistoryPath(""); err == nil {
		t.Fatal("empty accepted")
	}
	if _, err := Far3HistoryPath(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing accepted")
	}
}

func TestFar3ImportDialogPathAndCancel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	hp := history.NewProviderAtPath(filepath.Join(t.TempDir(), "history.json"))
	defer hp.Close()
	hp.SaveHistory(far3LocationHistoryID, []string{`C:\Programs\Far3`})
	called := false
	dlg := ShowFar3HistoryImport(hp, func() { called = true })
	defer dlg.Close()
	edit := firstDialogEdit(dlg)
	if edit == nil || edit.GetText() != `C:\Programs\Far3` {
		t.Fatal("saved path not displayed")
	}
	for _, width := range []int{80, 160, 80} {
		dlg.ResizeConsole(width, 30)
		assertFileDialogGeometry(t, dlg, edit, width)
		for _, item := range dlg.GetChildren() {
			x1, y1, x2, y2 := item.GetPosition()
			if x1 <= dlg.X1 || x2 >= dlg.X2 || y1 <= dlg.Y1 || y2 >= dlg.Y2 {
				t.Fatalf("control outside dialog at %d columns: %T (%d,%d)-(%d,%d)", width, item, x1, y1, x2, y2)
			}
		}
	}
	for _, item := range dlg.GetChildren() {
		if b, ok := item.(*vtui.Button); ok {
			caption, _, _ := vtui.ParseAmpersandString(i18n.Msg("vtui.Cancel"))
			if b.GetCaption() == caption {
				b.OnClick()
			}
		}
	}
	if called || len(hp.LoadHistory("cmdline")) > 0 {
		t.Fatal("cancel imported history")
	}
}
