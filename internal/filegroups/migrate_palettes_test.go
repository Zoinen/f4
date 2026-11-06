package filegroups

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func paletteMigrationFixture() ([]Group, string, string) {
	style := "Тёмная"
	prefix := fmt.Sprintf("Palette.%x.", []byte(style))
	legacy := fmt.Sprintf("style.%x.Highlight_8", sha256.Sum256([]byte(style)))
	return []Group{{ID: "type.executables", Fields: map[string]string{
		"Name": "Executables", "Mask": "*.exe,*.sh", "Icons.Enabled": "true", "Factory": "true",
		prefix + "LegacyID": legacy, prefix + "Name": "Executables", prefix + "Mask": "*",
		prefix + "MaskSyntax": "legacy-glob", prefix + "Case": "insensitive",
		prefix + "IncludeAttributes": "Executable", prefix + "ExcludeAttributes": "Directory",
		prefix + "Highlight.Enabled": "true", prefix + "NormalColor": "foreground:#123456",
		prefix + "Order": "8",
	}}}, prefix, legacy
}

func TestMigratePaletteGroupsEmptyAliasesAndOrder(t *testing.T) {
	defaults, prefix, legacy := paletteMigrationFixture()
	otherPrefix := fmt.Sprintf("Palette.%x.", []byte("Other"))
	otherLegacy := "style.other.Highlight_8"
	defaults[0].Fields[otherPrefix+"LegacyID"] = otherLegacy
	overrides := []Group{
		{ID: "before", Fields: map[string]string{"Name": "Before"}},
		{ID: legacy, Fields: map[string]string{}},
		{ID: "between", Fields: map[string]string{"Name": "Between"}},
		{ID: otherLegacy, Fields: map[string]string{}},
		{ID: "type.executables", Fields: map[string]string{"Icon.future": "none"}},
		{ID: "legacy.highlight_8", Fields: map[string]string{"NormalColor": "custom"}},
	}
	before := overrides[1].Clone()
	got, changed, err := migratePaletteGroups(defaults, overrides)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if ids := []string{got[0].ID, got[1].ID, got[2].ID, got[3].ID}; !reflect.DeepEqual(ids, []string{"before", "type.executables", "between", "legacy.highlight_8"}) {
		t.Fatalf("order=%v", ids)
	}
	if len(got) != 4 || !maps.Equal(got[1].Fields, map[string]string{"Icon.future": "none"}) {
		t.Fatalf("aliases became synthetic user records: %+v", got)
	}
	if !reflect.DeepEqual(before, overrides[1]) || defaults[0].Fields[prefix+"LegacyID"] != legacy {
		t.Fatal("migration modified its inputs")
	}
	again, changed, err := migratePaletteGroups(defaults, got)
	if err != nil || changed || !reflect.DeepEqual(again, got) {
		t.Fatalf("migration repeated: changed=%v err=%v", changed, err)
	}
}

func TestMigratePaletteGroupsVariantFields(t *testing.T) {
	defaults, prefix, legacy := paletteMigrationFixture()
	fields := map[string]string{
		"Name": "My executables", "Enabled": "false", "Highlight.Enabled": "false",
		"NormalColor": "", "SelectedColor": "foreground:#abcdef", "CursorColor": "", "SelectedCursorColor": "",
		"UseDefaults": "0", "ContinueProcessing": "true", "Mark": "!", "Mask": "*.custom",
		"MaskSyntax": "far", "Case": "sensitive", "IncludeAttributes": "Hidden", "ExcludeAttributes": "Directory",
		"SizeAbove": "12", "DateRelative": "true", "DateAfter": "2d", "Vendor.Future": "preserved",
	}
	patch := Group{ID: legacy, Fields: OverlayFields(fields, map[string]string{"ColorStyle": "Тёмная", "Factory": "true"})}
	got, changed, err := migratePaletteGroups(defaults, []Group{patch})
	if err != nil || !changed || len(got) != 1 || got[0].ID != defaults[0].ID {
		t.Fatalf("groups=%+v changed=%v err=%v", got, changed, err)
	}
	if len(got[0].Fields) != len(fields) {
		t.Fatalf("unexpected fields: %v", got[0].Fields)
	}
	for key, value := range fields {
		if actual, present := got[0].Fields[prefix+key]; !present || actual != value {
			t.Fatalf("lost %s=%q", key, value)
		}
	}
	if defaults[0].Fields[prefix+"NormalColor"] != "foreground:#123456" {
		t.Fatal("mutated defaults")
	}
}

func TestMigratePaletteGroupsIndependentFeatures(t *testing.T) {
	defaults, prefix, legacy := paletteMigrationFixture()
	for _, fields := range []map[string]string{
		{"Icon.*": "file:/custom.svg"}, {"Icons.Enabled": "false"}, {"IconTheme": "future"},
		{"Sort.Enabled": "true", "Sort.Number": "42"}, {"Associations.Enabled": "true"},
		{"ColorStyle": "Different"},
	} {
		t.Run(fmt.Sprint(fields), func(t *testing.T) {
			got, changed, err := migratePaletteGroups(defaults, []Group{{ID: legacy, Fields: fields}})
			if err != nil || !changed || len(got) != 2 || got[0].ID != legacy {
				t.Fatalf("groups=%+v changed=%v err=%v", got, changed, err)
			}
			for key, value := range fields {
				if actual, present := got[0].Fields[key]; !present || actual != value {
					t.Fatalf("lost %s", key)
				}
			}
			wantStyle := "Тёмная"
			if value, exists := fields["ColorStyle"]; exists {
				wantStyle = value
			}
			if got[0].Fields["ColorStyle"] != wantStyle || got[0].Fields["Mask"] != "*" || got[0].Fields["Factory"] != "false" || got[1].Fields[prefix+"Highlight.Enabled"] != "false" {
				t.Fatalf("lost inherited conditions/context or duplicated variant: %+v", got)
			}
			if got[0].Fields["Order"] != "8" {
				t.Fatal("independent group lost historical palette order")
			}
			again, changed, err := migratePaletteGroups(defaults, got)
			if err != nil || changed || !reflect.DeepEqual(again, got) {
				t.Fatalf("independent migration repeated: changed=%v err=%v", changed, err)
			}
		})
	}
}

func TestMigratePaletteGroupsConflicts(t *testing.T) {
	defaults, prefix, legacy := paletteMigrationFixture()
	for _, overrides := range [][]Group{
		{{ID: legacy, Fields: map[string]string{"NormalColor": "first"}}, {ID: defaults[0].ID, Fields: map[string]string{prefix + "NormalColor": "second"}}},
		{{ID: defaults[0].ID, Fields: map[string]string{prefix + "NormalColor": "second"}}, {ID: legacy, Fields: map[string]string{"NormalColor": "first"}}},
	} {
		if _, _, err := migratePaletteGroups(defaults, overrides); err == nil {
			t.Fatal("conflicting edits silently overwritten")
		}
	}
	other := defaults[0].Clone()
	other.ID = "ambiguous"
	if _, _, err := migratePaletteGroups(append(defaults, other), nil); err == nil {
		t.Fatal("ambiguous ancestry accepted")
	}
}

func TestPaletteMigrationStoreCommandsBackupAndIdempotence(t *testing.T) {
	defaults, _, legacy := paletteMigrationFixture()
	command := CommandSet{ID: "editor", Name: "Editor", Commands: [6]string{"execute", "alternate", "view", "alt-view", "edit\nnext line", `C:\Tools\edit.exe !.!`}, Enabled: [6]bool{true, false, true, false, true, true}, Extra: map[string]string{"Vendor.Command": "kept"}}
	doc, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.Encode([]Group{{ID: legacy, Fields: map[string]string{"Associations.Enabled": "true", "Vendor.Future": "kept"}, Commands: []CommandSet{command}}})
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), Defaults: defaults}
	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	backup := store.Path + ".pre-filegroups-palettes"
	assertPaletteFile(t, store.Path, data)
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Begin created a backup", err)
	}
	groups := tx.Groups()
	if groups[0].ID != legacy || groups[0].Fields["ColorStyle"] != "Тёмная" || groups[0].Fields["Vendor.Future"] != "kept" || len(groups[0].Commands) != 1 {
		t.Fatalf("lost independent group: %+v", groups)
	}
	if got := groups[0].Commands[0]; got.Commands != command.Commands || got.Enabled != command.Enabled || got.Extra["Vendor.Command"] != "kept" {
		t.Fatalf("six commands changed: %+v", got)
	}
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	projection := snapshot.HighlightGroups("Тёмная")
	active := 0
	for _, group := range projection {
		if !group.Enabled("Highlight") {
			continue
		}
		active++
		if group.ID != legacy || group.Fields["IncludeAttributes"] != "Executable" || group.Fields["Mask"] != "*" {
			t.Fatalf("independent projection lost its predicate: %+v", group)
		}
		if snapshot.HighlightPredicate(group.ID, "Тёмная").Match(&vfs.VFSItem{Name: "plain.txt"}, true, time.Now()) {
			t.Fatal("materialized rule lost executable condition")
		}
	}
	if active != 1 {
		t.Fatalf("expected one active highlighting rule, got %d", active)
	}
	if err := tx.Commit(groups); err != nil {
		t.Fatal(err)
	}
	assertPaletteFile(t, backup, data)
	reloaded, err := store.Begin()
	if err != nil || reloaded.Migrated() {
		t.Fatalf("reload: migrated=%v err=%v", reloaded != nil && reloaded.Migrated(), err)
	}
	if !reflect.DeepEqual(groups, reloaded.Groups()) {
		t.Fatalf("round trip changed groups\nbefore=%+v\nafter=%+v", groups, reloaded.Groups())
	}
	if err := reloaded.Commit(reloaded.Groups()); err != nil {
		t.Fatal(err)
	}
	assertPaletteFile(t, backup, data)
}

func TestPaletteMigrationStoreFailedApply(t *testing.T) {
	for _, mode := range []string{"write", "conflict", "backup conflict", "empty backup", "matching backup"} {
		t.Run(mode, func(t *testing.T) {
			defaults, _, legacy := paletteMigrationFixture()
			data := []byte("[FileGroups]\nVersion=3\n[Group." + legacy + "]\n")
			store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), Defaults: defaults}
			if err := os.WriteFile(store.Path, data, 0600); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("write failed")
			store.Write = func(string, []byte, os.FileMode) error { return failure }
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			backup := store.Path + ".pre-filegroups-palettes"
			if mode == "conflict" {
				if err := os.WriteFile(store.Path, append(data, []byte("# external\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "backup conflict" {
				if err := os.WriteFile(backup, []byte("existing backup"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "empty backup" || mode == "matching backup" {
				var existing []byte
				if mode == "matching backup" {
					existing = data
				}
				if err := os.WriteFile(backup, existing, 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = tx.Commit(tx.Groups())
			if err == nil || !tx.Migrated() {
				t.Fatal("failed Apply lost pending migration", err)
			}
			if mode == "conflict" && !errors.Is(err, ErrConflict) {
				t.Fatal("external conflict ignored", err)
			}
			if mode == "write" && !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if mode == "backup conflict" {
				assertPaletteFile(t, backup, []byte("existing backup"))
			} else if mode == "empty backup" {
				assertPaletteFile(t, backup, nil)
			} else if mode == "matching backup" {
				assertPaletteFile(t, backup, data)
			} else if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed Apply left a backup", err)
			}
			if mode != "conflict" {
				assertPaletteFile(t, store.Path, data)
			}
		})
	}
}

func TestMigratePaletteGroupsRequiresDeclaredAncestry(t *testing.T) {
	defaults, _, _ := paletteMigrationFixture()
	groups := []Group{
		{ID: "style.unlisted.Highlight_8", Fields: map[string]string{"Name": "Executables", "NormalColor": "custom"}},
		{ID: "legacy.highlight_8", Fields: map[string]string{"Name": "Executables", "NormalColor": "custom"}},
	}
	got, changed, err := migratePaletteGroups(defaults, groups)
	if err != nil || changed || !reflect.DeepEqual(got, groups) {
		t.Fatalf("inferred factory ancestry: groups=%+v changed=%v err=%v", got, changed, err)
	}
}

func assertPaletteFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(want) {
		t.Fatalf("%s changed: err=%v", path, err)
	}
}

func TestPaletteMigrationStoreEmptyAliases(t *testing.T) {
	defaults, _, legacy := paletteMigrationFixture()
	store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), Defaults: defaults}
	data := []byte("[FileGroups]\nVersion=3\n[Group." + legacy + "]\n")
	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if groups := tx.Groups(); len(groups) != 1 || !reflect.DeepEqual(groups[0], defaults[0]) {
		t.Fatalf("empty alias became synthetic user/modified group: %+v", groups)
	}
	if err := tx.Commit(tx.Groups()); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(store.Path)
	if err != nil || strings.Contains(string(saved), "[Group."+legacy+"]") {
		t.Fatal("old alias survived Apply", err)
	}
	reloaded, err := store.Begin()
	if err != nil || reloaded.Migrated() {
		t.Fatal("empty alias migration repeated", err)
	}
}

func TestPaletteBooleanDifferences(t *testing.T) {
	prefix := fmt.Sprintf("Palette.%x.", []byte("Modern"))
	for _, field := range []string{"Enabled", "Highlight.Enabled", "Icons.Enabled", "UseDefaults", "ContinueProcessing", "DateRelative"} {
		t.Run(field, func(t *testing.T) {
			key := prefix + field
			if !isBooleanField(key) {
				t.Fatal("palette boolean not recognized")
			}
			for _, values := range [][2]string{{"1", "true"}, {"true", "1"}, {"0", "false"}, {"false", "0"}} {
				defaults := []Group{{ID: "canonical", Fields: map[string]string{key: values[0]}}}
				groups := []Group{{ID: "canonical", Fields: map[string]string{key: values[1]}}}
				if patch := Differences(defaults, groups); len(patch[0].Fields) != 0 {
					t.Fatalf("normalization froze inheritance: %v", patch)
				}
			}
			value := "true"
			if field == "Enabled" {
				value = "false"
			}
			defaults := []Group{{ID: "canonical", Fields: map[string]string{}}}
			groups := []Group{{ID: "canonical", Fields: map[string]string{key: value}}}
			if patch := Differences(defaults, groups); patch[0].Fields[key] != value {
				t.Fatalf("effective boolean change lost: %v", patch)
			}
		})
	}
}

func TestPaletteMigrationStoreVariantRoundTrip(t *testing.T) {
	defaults, prefix, legacy := paletteMigrationFixture()
	defaults[0].Fields["Icon.*"] = "asset:executables"
	data := []byte("[FileGroups]\nVersion=3\n[Group." + legacy + "]\nEnabled=false\nNormalColor=\"\"\nVendor.Future=kept\nMask=*.custom\n")
	store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), Defaults: defaults}
	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(tx.Groups()); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Begin()
	if err != nil || reloaded.Migrated() {
		t.Fatal("variant migration repeated", err)
	}
	fields := reloaded.Overrides()[0].Fields
	for key, want := range map[string]string{"Enabled": "false", "NormalColor": "", "Vendor.Future": "kept", "Mask": "*.custom"} {
		if got, present := fields[prefix+key]; !present || got != want {
			t.Fatalf("lost palette field %s=%q", key, want)
		}
	}
	group := reloaded.Groups()[0]
	if !group.Enabled("Icons") || group.Highlighting("Тёмная").Enabled("Highlight") {
		t.Fatal("disabled palette changed group icons or failed to disable highlighting")
	}
	snapshot, err := Compile(reloaded.Groups())
	if err != nil {
		t.Fatal(err)
	}
	choices := snapshot.IconChoices(&vfs.VFSItem{Name: "run.exe"}, true, time.Now())
	if choices["*"].Reference.Value != "executables" {
		t.Fatalf("disabled palette removed group artwork: %v", choices)
	}
}

func TestMigratedAliasesRestoreFactoryOrder(t *testing.T) {
	for _, kind := range []string{"catalog", "palette", "both", "canonical"} {
		t.Run(kind, func(t *testing.T) {
			prefix := PalettePrefix("Modern")
			defaults := []Group{
				{ID: "type.yaml", Fields: map[string]string{"Name": "Yaml", "Mask": "*.yaml", "LegacyIDs": "qt.zoin.yml.extensions", prefix + "LegacyID": "style.modern.Highlight_1"}},
				{ID: "type.text-files", Fields: map[string]string{"Name": "Text files", "Mask": "*.yaml,*.txt", "LegacyIDs": "qt.zoin.text.extensions", prefix + "LegacyID": "style.modern.Highlight_0"}},
			}
			groups := []Group{
				{ID: "legacy.highlight_0", Fields: map[string]string{"Name": "Personal", "Mask": "*.mine"}},
			}
			if kind == "catalog" || kind == "both" {
				groups = append(groups, Group{ID: "qt.zoin.text.extensions", Fields: map[string]string{}}, Group{ID: "qt.zoin.yml.extensions", Fields: map[string]string{}})
			}
			if kind == "palette" || kind == "both" {
				groups = append(groups, Group{ID: "style.modern.Highlight_0", Fields: map[string]string{}}, Group{ID: "style.modern.Highlight_1", Fields: map[string]string{}})
			}
			// Include the canonical empty patches too: a former Apply can
			// have frozen them alongside the old aliases.
			groups = append(groups, Group{ID: "type.text-files", Fields: map[string]string{}}, Group{ID: "type.yaml", Fields: map[string]string{}}, Group{ID: "user.empty", Fields: map[string]string{}})
			doc, err := Parse(nil)
			if err != nil {
				t.Fatal(err)
			}
			data, err := doc.Encode(groups)
			if err != nil {
				t.Fatal(err)
			}
			store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), Defaults: defaults}
			if err := os.WriteFile(store.Path, data, 0600); err != nil {
				t.Fatal(err)
			}
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"legacy.highlight_0", "user.empty", "type.yaml", "type.text-files"}
			if kind == "canonical" {
				want = []string{"legacy.highlight_0", "type.text-files", "type.yaml", "user.empty"}
				if tx.Migrated() {
					t.Fatal("canonical-only order incorrectly repaired")
				}
			}
			assertPaletteGroupOrder(t, tx.Groups(), want)
			assertPaletteFile(t, store.Path, data)
			if err := tx.Commit(tx.Groups()); err != nil {
				t.Fatal(err)
			}
			reloaded, err := store.Begin()
			if err != nil || reloaded.Migrated() {
				t.Fatal("order repair repeated", err)
			}
			assertPaletteGroupOrder(t, reloaded.Groups(), want)
		})
	}
}

func TestDiscardMigratedFactoryOrderPreservesOverrides(t *testing.T) {
	defaults := []Group{{ID: "empty"}, {ID: "edited"}, {ID: "commands"}, {ID: "clear-commands"}}
	groups := []Group{
		{ID: "empty", Fields: map[string]string{}},
		{ID: "edited", Fields: map[string]string{"Vendor.Keep": ""}},
		{ID: "commands", Commands: []CommandSet{{ID: "edit", Commands: [6]string{"one", "two", "three", "four", "five", "six"}}}},
		{ID: "clear-commands", Commands: []CommandSet{}},
		{ID: "user.empty", Fields: map[string]string{}},
		{ID: "legacy.highlight_0", Fields: map[string]string{"Name": "Imported"}},
	}
	got := discardMigratedFactoryOrder(defaults, groups)
	if !reflect.DeepEqual(got, groups[1:]) {
		t.Fatalf("repair removed or changed customized records: %+v", got)
	}
}

func assertPaletteGroupOrder(t *testing.T, groups []Group, want []string) {
	t.Helper()
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("group order=%v, want %v", ids, want)
	}
}
