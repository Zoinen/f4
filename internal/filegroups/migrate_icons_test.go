package filegroups

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestImportedBundledIconDoesNotOverrideOtherCatalogs(t *testing.T) {
	for _, version := range []string{"legacy", "v3"} {
		t.Run(version, func(t *testing.T) {
			const icon = "qrc:/Example/icons/outline/archive.svg"
			data := []byte("[Highlight_0]\nName=Archive\nMask=*.zip\nNormalColor=foreground:#123456\nIcon=" + icon + "\n")
			if version == "v3" {
				data = []byte("[FileGroups]\nVersion=3\nOrder=legacy.highlight_0\n[Group.legacy.highlight_0]\nName=Archive\nMask=*.zip\nHighlight.Enabled=true\nNormalColor=foreground:#123456\nIcons.Enabled=true\nIcon=" + icon + "\nIcon.*=" + icon + "\n")
			}
			path := filepath.Join(t.TempDir(), "highlight.ini")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			store := Store{Path: path, ThemeDefaults: []ThemeSet{{ID: "outline", Fields: map[string]string{"IconCatalog": "outline", "LegacyIconPrefix": "qrc:/Example/icons/outline/", "GroupIcons": "true", "CatalogAssets": "archive"}}}, Defaults: []Group{
				{ID: "filled.zip", Fields: map[string]string{"IconTheme": "filled", "Mask": "*.zip", "Icons.Enabled": "true", "Icon.filled": "asset:zip"}},
			}}
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			if string(before) != string(data) {
				t.Fatal("startup migration wrote profile")
			}
			snapshot, err := Compile(tx.Groups(), tx.Themes())
			if err != nil {
				t.Fatal(err)
			}
			item := &vfs.VFSItem{Name: "test.zip"}
			outline := snapshot.ForIconTheme("outline").IconChoices(item, true, time.Now())
			if _, exists := outline["*"]; exists {
				t.Fatal("imported bundled icon still overrides every theme")
			}
			if outline["outline"].Reference.Kind != "asset" || outline["outline"].Reference.Value != "archive" {
				t.Fatal("imported group did not adopt group-named artwork")
			}
			filled := snapshot.ForIconTheme("filled").IconChoices(item, true, time.Now())
			if _, exists := filled["*"]; exists || filled["filled"].Reference.Value != "zip" {
				t.Fatal("filled theme is shadowed")
			}
			if err := tx.Commit(tx.Groups()); err != nil {
				t.Fatal(err)
			}
			backup := path + ".pre-filegroups"
			if version == "v3" {
				backup = path + ".pre-filegroups-icon-scopes"
			}
			if original, err := os.ReadFile(backup); err != nil || string(original) != string(data) {
				t.Fatal("migration did not back up the exact original file", err)
			}
			reloaded, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Migrated() {
				t.Fatal("migration is not idempotent")
			}
			for _, group := range reloaded.Groups() {
				if group.ID == "legacy.highlight_0" && (group.Fields["IconTheme"] != "" || group.Fields["NormalColor"] != "foreground:#123456" || group.Fields["Icon"] != "") {
					t.Fatal("universal colours or legacy icon projection not preserved")
				}
			}
		})
	}
}

func TestIconScopeMigrationPreservesExplicitOverridesAndCommands(t *testing.T) {
	const bundled = "qrc:/Example/outline/file.svg"
	themes := []ThemeSet{{ID: "outline", Fields: map[string]string{"IconCatalog": "outline", "LegacyIconPrefix": "qrc:/Example/outline/"}}}
	for _, tc := range []struct {
		name, id, icon, generic string
		migrate                 bool
	}{
		{"imported artwork", "legacy.highlight_0", bundled, bundled, true},
		{"custom icon", "legacy.highlight_0", "file:/tmp/custom.svg", "file:/tmp/custom.svg", false},
		{"edited generic override", "legacy.highlight_0", bundled, "asset:custom", false},
		{"new user group", "user-icons", bundled, bundled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := []Group{{ID: tc.id, Fields: map[string]string{"Icon": tc.icon, "Icon.*": tc.generic, "Icons.Enabled": "false", "NormalColor": "foreground:#123456", "UseDefaults": "true"}, Commands: []CommandSet{{ID: "application", Name: "Editor", Commands: [6]string{"one", "two", "three", "four", "five", "six"}, Enabled: [6]bool{true, true, true, true, true, true}}}}}
			if changed := migrateLegacyIconScopes(groups, themes); changed != tc.migrate {
				t.Fatalf("changed=%v", changed)
			}
			if !tc.migrate && groups[0].Fields["Icon.*"] != tc.generic {
				t.Fatal("explicit override changed")
			}
			if groups[0].Fields["Icons.Enabled"] != "false" || groups[0].Fields["UseDefaults"] != "true" || groups[0].Fields["NormalColor"] != "foreground:#123456" || groups[0].Commands[0].Commands[5] != "six" || !groups[0].Commands[0].Enabled[5] {
				t.Fatal("unrelated features changed")
			}
		})
	}
}
