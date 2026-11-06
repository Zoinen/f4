package filegroups

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestMigrateCatalogGroupsMergesAliases(t *testing.T) {
	t.Parallel()
	defaults := []Group{{ID: "images", Fields: map[string]string{
		"LegacyIDs": " qt.lucide.image,qt.zoin.image.extensions, qt.system.image,qt.lucide.image ",
		"Name":      "Images", "Mask": "*.png", "ExcludeAttributes": "Directory", "Icons.Enabled": "true",
		"Icon.*": "asset:images", "Factory": "true",
	}}}
	overrides := []Group{
		{ID: "qt.lucide.image", Fields: map[string]string{"IconTheme": "lucide", "Factory": "true", "Icon.*": "none"}},
		{ID: "images", Fields: map[string]string{"Name": "Pictures"}},
		{ID: "qt.zoin.image.extensions", Fields: map[string]string{"IconTheme": "zoin", "Icon.zoin": "file:/my/image.svg"}},
		{ID: "qt.system.image", Fields: map[string]string{}},
	}
	got, changed, err := migrateCatalogGroups(defaults, overrides)
	if err != nil || !changed {
		t.Fatalf("changed=%v, err=%v", changed, err)
	}
	want := []Group{{ID: "images", Fields: map[string]string{
		"Name": "Pictures", "Icon.lucide": "none", "Icon.zoin": "file:/my/image.svg",
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	again, changed, err := migrateCatalogGroups(defaults, got)
	if err != nil || changed || !reflect.DeepEqual(got, again) {
		t.Fatalf("migration repeated: changed=%v, err=%v, groups=%+v", changed, err, again)
	}
	if overrides[0].ID != "qt.lucide.image" || overrides[0].Fields["Icon.*"] != "none" || defaults[0].Fields["Factory"] != "true" {
		t.Fatal("migration modified its inputs")
	}
}

func TestMigrateCatalogGroupsHarmlessMetadata(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		fields map[string]string
	}{
		{name: "empty"},
		{name: "scope", fields: map[string]string{"IconTheme": "zoin"}},
		{name: "factory metadata", fields: map[string]string{"IconTheme": "zoin", "LegacyID": "old", "FactoryFallback": "true", "Factory": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defaults := []Group{{ID: "images", Fields: map[string]string{"LegacyIDs": "qt.zoin.image", "Mask": "*.png"}}}
			got, changed, err := migrateCatalogGroups(defaults, []Group{{ID: "qt.zoin.image", Fields: tc.fields}, {ID: "images", Fields: map[string]string{}}})
			if err != nil || !changed || len(got) != 1 || got[0].ID != "images" || len(got[0].Fields) != 0 {
				t.Fatalf("got %+v, changed=%v, err=%v", got, changed, err)
			}
		})
	}
}

func TestMigrateCatalogGroupsPreservesCustomizedRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		fields map[string]string
	}{
		{name: "custom name", fields: map[string]string{"Name": "My images"}},
		{name: "mask", fields: map[string]string{"Mask": "*.custom"}},
		{name: "attributes", fields: map[string]string{"IncludeAttributes": "Hidden"}},
		{name: "date", fields: map[string]string{"DateAfter": "2d", "DateRelative": "true"}},
		{name: "highlight", fields: map[string]string{"Highlight.Enabled": "false", "NormalColor": ""}},
		{name: "sort", fields: map[string]string{"Sort.Enabled": "true", "Sort.Number": "42"}},
		{name: "association", fields: map[string]string{"Associations.Enabled": "true"}},
		{name: "unknown field", fields: map[string]string{"Vendor.Keep": "unchanged"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defaults := []Group{{ID: "images", Fields: map[string]string{
				"LegacyIDs": "qt.zoin.image", "Name": "Images", "Mask": "*.png", "MaskSyntax": "far", "Case": "insensitive", "ExcludeAttributes": "Directory", "Factory": "true",
			}}}
			patch := Group{ID: "qt.zoin.image", Fields: OverlayFields(tc.fields, map[string]string{"IconTheme": "zoin"})}
			got, changed, err := migrateCatalogGroups(defaults, []Group{patch})
			if err != nil || !changed || len(got) != 1 {
				t.Fatalf("groups=%+v changed=%v err=%v", got, changed, err)
			}
			group := got[0]
			wantName := "Images"
			if name, exists := tc.fields["Name"]; exists {
				wantName = name
			}
			if group.ID != patch.ID || group.IconTheme() != "" || group.Fields["Name"] != wantName || group.Fields["Factory"] != "false" {
				t.Fatalf("custom group lost identity or catalog association: %+v", group)
			}
			for key, value := range tc.fields {
				if actual, present := group.Fields[key]; !present || actual != value {
					t.Fatalf("lost %s=%q: %+v", key, value, group)
				}
			}
			if _, present := tc.fields["Mask"]; !present && group.Fields["Mask"] != "*.png" {
				t.Fatal("partial override lost inherited predicate")
			}
			if _, present := group.Fields["LegacyIDs"]; present {
				t.Fatal("user group retained factory aliases")
			}
			if _, present := group.Fields["Icon.*"]; present {
				t.Fatal("automatic catalog association became a generic override")
			}
			if again, changed, err := migrateCatalogGroups(defaults, got); err != nil || changed || !reflect.DeepEqual(again, got) {
				t.Fatalf("custom migration repeated: groups=%+v changed=%v err=%v", again, changed, err)
			}
		})
	}
}

func TestMigrateCatalogGroupsPreservesCommandsAndIcons(t *testing.T) {
	t.Parallel()
	defaults := []Group{{ID: "images", Fields: map[string]string{
		"LegacyIDs": "qt.zoin.image", "Name": "Images", "Mask": "*.png", "Icons.Enabled": "true", "Icon.*": "asset:images",
	}}}
	command := CommandSet{ID: "open", Name: "Editor", Commands: [6]string{`C:\Tools\open.exe !.!`, "two\nlines", "view", "altview", "edit", "altedit"}, Enabled: [6]bool{true, false, true, false, true, true}, Extra: map[string]string{"Vendor.Command": "kept"}, Overrides: map[string]string{"Edit": "edit", "Edit.Enabled": "true"}}
	command.Overrides = commandValues(command)
	patch := Group{ID: "qt.zoin.image", Fields: map[string]string{
		"IconTheme": "zoin", "Name": "", "Icon.*": "none", "Icon.future": "", "Associations.Enabled": "true", "CommandOrder": "open",
	}, Commands: []CommandSet{command}}
	got, _, err := migrateCatalogGroups(defaults, []Group{patch})
	if err != nil || len(got) != 1 {
		t.Fatalf("groups=%+v err=%v", got, err)
	}
	if !reflect.DeepEqual(got[0].Commands, patch.Commands) || got[0].Fields["Name"] != "" || got[0].Fields["Icon.zoin"] != "none" {
		t.Fatalf("commands, explicit name clear or icon lost: %+v", got[0])
	}
	if raw, present := got[0].Fields["Icon.future"]; !present || raw != "" {
		t.Fatal("unknown catalog explicit clear lost")
	}
	snapshot, err := Compile(Merge(defaults, got))
	if err != nil {
		t.Fatal(err)
	}
	item := &vfs.VFSItem{Name: "a.png"}
	for _, catalog := range []string{"zoin", "lucide", "future"} {
		view := snapshot.ForIconTheme(catalog)
		matches := view.Matching(item, true, "Associations", time.Time{})
		if len(matches) != 1 || len(matches[0].Commands) != 1 || matches[0].Commands[0].Commands != command.Commands || matches[0].Commands[0].Enabled != command.Enabled || matches[0].Commands[0].Name != command.Name {
			t.Fatalf("%s lost universal command set: %+v", catalog, matches)
		}
	}
	got[0].Commands[0].Extra["Vendor.Command"] = "changed"
	if patch.Commands[0].Extra["Vendor.Command"] != "kept" {
		t.Fatal("result aliases input commands")
	}
}

func TestMigrateCatalogGroupsPreservesUnknownGroups(t *testing.T) {
	t.Parallel()
	defaults := []Group{{ID: "images", Fields: map[string]string{"LegacyIDs": "qt.zoin.image", "LegacyID": "qt.zoin.not-listed"}}}
	overrides := []Group{
		{ID: "qt.zoin.personal", Fields: map[string]string{"IconTheme": "future", "Mask": "*.mine"}},
		{ID: "qt.zoin.not-listed", Fields: map[string]string{"Icon.*": "none"}},
		{ID: "user-images", Fields: map[string]string{"Name": "Zoin: image", "IconTheme": "zoin"}},
	}
	got, changed, err := migrateCatalogGroups(defaults, overrides)
	if err != nil || changed || !reflect.DeepEqual(got, overrides) {
		t.Fatalf("unknown user groups changed: groups=%+v changed=%v err=%v", got, changed, err)
	}
}

func TestMigrateCatalogGroupsRejectsConflicts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		defaults  []Group
		overrides []Group
	}{
		{name: "ambiguous alias", defaults: []Group{
			{ID: "a", Fields: map[string]string{"LegacyIDs": "qt.zoin.old"}},
			{ID: "b", Fields: map[string]string{"LegacyIDs": "qt.zoin.old"}},
		}},
		{name: "conflicting fields", defaults: []Group{{ID: "a", Fields: map[string]string{"LegacyIDs": "qt.zoin.old"}}}, overrides: []Group{
			{ID: "a", Fields: map[string]string{"Icon.zoin": "none"}},
			{ID: "qt.zoin.old", Fields: map[string]string{"Icon.zoin": "file:/my.svg"}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, changed, err := migrateCatalogGroups(tc.defaults, tc.overrides); err == nil || changed || got != nil {
				t.Fatalf("unsafe migration accepted: groups=%+v changed=%v err=%v", got, changed, err)
			}
		})
	}
}

func TestMigrateCatalogGroupsFactoryProfile(t *testing.T) {
	t.Parallel()
	defaults, err := FactoryGroups()
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	source.WriteString("[FileGroups]\nVersion=3\n")
	// Reproduce the identity-only sections Differences writes on Apply,
	// including all aliases that now converge on the same shared group.
	for _, group := range defaults {
		for i, raw := range strings.Split(group.Fields["LegacyIDs"], ",") {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			source.WriteString("\n[Group." + id + "]\n")
			if i%2 == 0 {
				source.WriteString("IconTheme=" + legacyCatalog(Group{ID: id}) + "\n")
			}
		}
	}
	doc, err := Parse([]byte(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := doc.Groups()
	if err != nil {
		t.Fatal(err)
	}
	got, changed, err := migrateCatalogGroups(defaults, overrides)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	aliased := 0
	for _, group := range defaults {
		if group.Fields["LegacyIDs"] != "" {
			aliased++
		}
	}
	if len(got) != aliased {
		t.Fatalf("%d migrated groups, want %d aliased shared groups", len(got), aliased)
	}
	for _, group := range got {
		if !strings.HasPrefix(group.ID, "type.") || len(group.Fields) != 0 {
			t.Fatalf("obsolete or frozen factory rule survived: %+v", group)
		}
	}
	merged := Merge(defaults, got)
	if len(merged) != len(defaults) {
		t.Fatalf("duplicate classification groups: %d, want %d", len(merged), len(defaults))
	}
	if again, changed, err := migrateCatalogGroups(defaults, got); err != nil || changed || !reflect.DeepEqual(again, got) {
		t.Fatalf("migration repeated: changed=%v err=%v", changed, err)
	}
}

func TestMigrateCatalogGroupsStoreRoundTrip(t *testing.T) {
	t.Parallel()
	defaults, err := FactoryGroups()
	if err != nil {
		t.Fatal(err)
	}
	const source = "# Personal profile comment\n[FileGroups]\nVersion=3\nOrder=qt.zoin.image.extensions,qt.lucide.image,qt.zoin.go.extensions\n" +
		"[Group.qt.zoin.image.extensions]\nIconTheme=zoin\nIcons.Enabled=true\nIcon.*=none\nIcon.zoin=file:/my/image.svg\nIcon.future=\"\"\n" +
		"[Group.qt.lucide.image]\nIconTheme=lucide\n" +
		"[Group.qt.zoin.go.extensions]\nIconTheme=zoin\nMask=*.mine\nAssociations.Enabled=true\nCommandOrder=editor\n" +
		"[Command.qt.zoin.go.extensions/editor]\nName=My editor\nEdit=editor !.!\nEdit.Enabled=true\n" +
		"[Unknown.Section]\nKeep=literal\\path\n"
	path := filepath.Join(t.TempDir(), "highlight.ini")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path, Defaults: defaults}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly run the helper here as well, so this boundary test can run
	// before or after the composition task wires it into Store.Begin.
	patches, _, err := migrateCatalogGroups(defaults, tx.Overrides())
	if err != nil {
		t.Fatal(err)
	}
	if before, err := os.ReadFile(path); err != nil || string(before) != source {
		t.Fatalf("migration wrote before Apply: %v", err)
	}
	if err := tx.Commit(Merge(defaults, patches)); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	patches, changed, err := migrateCatalogGroups(defaults, reloaded.Overrides())
	if err != nil || changed {
		t.Fatalf("saved profile requires migration again: changed=%v err=%v", changed, err)
	}
	byID := map[string]Group{}
	for _, group := range patches {
		byID[group.ID] = group
	}
	image := byID["type.images"]
	for key, value := range map[string]string{"Icon.zoin": "file:/my/image.svg", "Icon.future": ""} {
		if got, present := image.Fields[key]; !present || got != value {
			t.Fatalf("lost explicit %s=%q: %+v", key, value, image)
		}
	}
	effective := map[string]Group{}
	for _, group := range reloaded.Groups() {
		effective[group.ID] = group
	}
	if !effective["type.images"].Enabled("Icons") {
		t.Fatal("effective image group lost inherited Icons.Enabled")
	}
	custom := byID["qt.zoin.go.extensions"]
	if custom.Fields["Name"] != "Go source" || custom.IconTheme() != "" || custom.Fields["Mask"] != "*.mine" || len(custom.Commands) != 1 || custom.Commands[0].Commands[4] != "editor !.!" || !custom.Commands[0].Enabled[4] {
		t.Fatalf("custom profile group lost data: %+v", custom)
	}
	snapshot, err := Compile(reloaded.Groups(), []ThemeSet{{ID: "lucide", Fields: map[string]string{
		"IconCatalog": "lucide", "GroupIcons": "true", "CatalogAssets": "go-source",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	choice := snapshot.ForIconTheme("lucide").IconChoices(&vfs.VFSItem{Name: "example.mine"}, true, time.Time{})["lucide"]
	if choice.GroupID != custom.ID || choice.Reference != (IconReference{Kind: "asset", Value: "go-source"}) {
		t.Fatalf("custom group lost automatic name-based catalog association: %+v", choice)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(saved), "# Personal profile comment") || !strings.Contains(string(saved), "[Unknown.Section]\nKeep=literal\\path") {
		t.Fatalf("lost profile comments/unknown section: %v", err)
	}
}
