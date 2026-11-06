package filegroups

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func oldCurrentFolderTheme() ColorTheme {
	return ColorTheme{ID: "current", Fields: map[string]string{
		"Name": "My Current", "Vendor.Keep": "personal",
		"Color.folders.NormalColor": "foreground:#FFFFFF", "Color.folders.CursorColor": "foreground:#FFFFFF",
		"Color.folders.SelectedColor": "foreground:#000000", "Color.folders.SelectedCursorColor": "foreground:#000000",
		"Color.parent-folder.NormalColor": "foreground:#B4B4B4", "Color.parent-folder.CursorColor": "foreground:#B4B4B4",
		"Color.parent-folder.SelectedColor": "foreground:#000000", "Color.parent-folder.SelectedCursorColor": "foreground:#000000",
		"Color.files.NormalColor": "foreground:#123456", "Color.folders.Vendor.Custom": "preserved",
	}}
}

func currentFolderMigrationStore(t *testing.T, current ColorTheme) (Store, []byte) {
	t.Helper()
	doc, err := Parse([]byte("# personal comment\n[Vendor.Section]\nKeep=personal\n"))
	if err != nil {
		t.Fatal(err)
	}
	groups := []Group{{ID: "personal", Fields: map[string]string{"Name": "Personal", "IconTheme": "lucide", "Mask": "*.mine", "Vendor.Keep": "group"}, Commands: []CommandSet{
		{ID: "open", Name: "Application", Commands: [6]string{"one", "two", "three", "four", "five\nsix", `C:\seven.exe`}, Enabled: [6]bool{true, false, true, false, true, true}},
	}}}
	colors := []ColorTheme{current, {ID: "zoin", Fields: map[string]string{"Name": "Custom Zoin", "Color.folders.NormalColor": "foreground:#F79F57", "Vendor.Keep": "zoin"}}, {ID: "personal", Fields: map[string]string{"Name": "Personal", "Color.files.NormalColor": "foreground:#abcdef"}}}
	data, err := doc.encodeSnapshotColors(groups, nil, colors, "zoin")
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), ColorDefaults: FactoryColorThemes()}
	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return store, data
}

func TestCurrentFolderColorsMigrateOnlyOnApply(t *testing.T) {
	store, original := currentFolderMigrationStore(t, oldCurrentFolderTheme())
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if !tx.Migrated() || tx.ColorThemeID() != "zoin" {
		t.Fatal("exact old entries were not migrated or selected theme changed")
	}
	want := oldCurrentFolderTheme()
	want.Fields["Color.folders.NormalColor"] = "foreground:#5AB2F1"
	want.Fields["Color.parent-folder.NormalColor"] = "foreground:#5AB2F1"
	if !reflect.DeepEqual(tx.ColorThemes()[0], want) {
		t.Fatalf("migration changed more than the two normal states: %+v", tx.ColorThemes()[0])
	}
	parsed, err := Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	beforeColors, err := parsed.ColorThemes()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tx.ColorThemes()[1:], beforeColors[1:]) {
		t.Fatal("migration changed Zoin or an unrelated colour theme")
	}
	groups := tx.Groups()
	assertPaletteFile(t, store.Path, original)
	backup := store.Path + ".pre-filegroups-current-folders"
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Begin wrote a backup", err)
	}
	if err := tx.Commit(tx.Groups()); err != nil {
		t.Fatal(err)
	}
	assertPaletteFile(t, backup, original)
	reloaded, err := store.Begin()
	if err != nil || reloaded.Migrated() {
		t.Fatal("migration repeated after Apply", err)
	}
	if !reflect.DeepEqual(reloaded.Groups(), groups) || !reflect.DeepEqual(reloaded.ColorThemes(), tx.ColorThemes()) || reloaded.ColorThemeID() != "zoin" {
		t.Fatal("migration changed groups/commands/colours during Apply/reload")
	}
}

func TestCurrentFolderMigrationPreservesEveryCustomState(t *testing.T) {
	for _, name := range []string{"folders", "parent-folder"} {
		for _, state := range []string{"NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor"} {
			t.Run(name+"/"+state, func(t *testing.T) {
				current := oldCurrentFolderTheme()
				current.Fields["Color."+name+"."+state] = ""
				store, _ := currentFolderMigrationStore(t, current)
				tx, err := store.Begin()
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(tx.ColorThemes()[0].Colors(name), current.Colors(name)) {
					t.Fatal("an explicitly edited state was treated as untouched")
				}
				other := "folders"
				if name == other {
					other = "parent-folder"
				}
				if tx.ColorThemes()[0].Colors(other)["NormalColor"] != "foreground:#5AB2F1" {
					t.Fatal("a customization prevented the other untouched entry from migrating")
				}
			})
		}
	}
	t.Run("missing state", func(t *testing.T) {
		current := oldCurrentFolderTheme()
		delete(current.Fields, "Color.folders.SelectedColor")
		store, _ := currentFolderMigrationStore(t, current)
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(tx.ColorThemes()[0].Colors("Folders"), current.Colors("Folders")) {
			t.Fatal("missing state was treated as an exact four-state match")
		}
	})
	t.Run("not opted in", func(t *testing.T) {
		current := oldCurrentFolderTheme()
		store, _ := currentFolderMigrationStore(t, current)
		store.ColorDefaults = nil
		tx, err := store.Begin()
		if err != nil || tx.Migrated() || !reflect.DeepEqual(tx.ColorThemes()[0], current) {
			t.Fatal("zero-default store changed legacy behaviour", err)
		}
	})
}

func TestCurrentFolderMigrationFailedApplyIsRetryable(t *testing.T) {
	for _, mode := range []string{"write", "external", "backup"} {
		t.Run(mode, func(t *testing.T) {
			store, original := currentFolderMigrationStore(t, oldCurrentFolderTheme())
			failure := errors.New("write failure")
			store.Write = func(string, []byte, os.FileMode) error { return failure }
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			backup := store.Path + ".pre-filegroups-current-folders"
			if mode == "external" {
				if err := os.WriteFile(store.Path, append(original, []byte("# external\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "backup" {
				if err := os.WriteFile(backup, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = tx.CommitColors(tx.Groups(), tx.Themes(), tx.ColorThemes(), tx.ColorThemeID())
			if err == nil || !tx.Migrated() {
				t.Fatal("failed Apply lost pending migration", err)
			}
			if mode == "write" && !errors.Is(err, failure) || mode == "external" && !errors.Is(err, ErrConflict) {
				t.Fatal("wrong failure", err)
			}
			if mode == "backup" {
				assertPaletteFile(t, backup, []byte("existing"))
			} else if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed Apply created a persistent backup", err)
			}
			if mode != "external" {
				assertPaletteFile(t, store.Path, original)
			}
			if mode == "write" {
				tx.store.Write = nil
				if err := tx.Commit(tx.Groups()); err != nil {
					t.Fatal("failed Apply could not be retried", err)
				}
			}
		})
	}
}
