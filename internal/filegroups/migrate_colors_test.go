package filegroups

import (
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestNormalizeColorGroupsPreservesIndependentHighlightEnablement(t *testing.T) {
	for _, tc := range []struct {
		name, groupEnabled, highlightEnabled string
	}{
		{name: "disabled group retains enabled highlighting", groupEnabled: "false", highlightEnabled: "true"},
		{name: "numeric disabled group retains highlighting", groupEnabled: "0", highlightEnabled: "1"},
		{name: "explicitly disabled highlighting stays off", groupEnabled: "false", highlightEnabled: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := Group{ID: "programs", Fields: map[string]string{
				"Name": "Programs", "Mask": "*.exe", "IconTheme": "lucide", "Enabled": tc.groupEnabled,
				"Highlight.Enabled": tc.highlightEnabled, "Highlight.Mask": "*", "Highlight.IncludeAttributes": "Executable",
				"NormalColor": "foreground:#123456", "Mark": "*", "ContinueProcessing": "true", "UseDefaults": "true",
			}}
			group := NormalizeColorGroups([]Group{original}, "Modern")[0]
			if group.Fields["Enabled"] != tc.groupEnabled || booleanValue(group.Fields["Highlight.Enabled"], false) != booleanValue(tc.highlightEnabled, false) {
				t.Fatalf("group/feature enablement became coupled: %v", group.Fields)
			}
			group.Fields["Enabled"] = "true"
			if group.Enabled("Highlight") != booleanValue(tc.highlightEnabled, false) {
				t.Fatal("re-enabling a group lost its independent highlight setting")
			}
			for _, key := range []string{"Mask", "IconTheme", "Highlight.Mask", "Highlight.IncludeAttributes", "Mark", "ContinueProcessing", "UseDefaults"} {
				if group.Fields[key] != original.Fields[key] {
					t.Fatalf("unrelated field %s changed", key)
				}
			}
			if original.Fields["NormalColor"] != "foreground:#123456" {
				t.Fatal("normalization mutated the caller")
			}
		})
	}
}

func TestCaptureGroupColorsResolvesAliasesAndPreservesCanonicalClears(t *testing.T) {
	for _, tc := range []struct {
		name, alias, canonical string
	}{
		{name: "normal", alias: "NormalFileName", canonical: "NormalColor"},
		{name: "selected", alias: "SelectedFileName", canonical: "SelectedColor"},
		{name: "cursor", alias: "NormalColorUnderCursor", canonical: "CursorColor"},
		{name: "cursor alternative", alias: "FileNameUnderCursor", canonical: "CursorColor"},
		{name: "selected cursor", alias: "SelectedColorUnderCursor", canonical: "SelectedCursorColor"},
		{name: "selected cursor alternative", alias: "FileNameSelectedUnderCursor", canonical: "SelectedCursorColor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := Group{ID: "alias", Fields: map[string]string{"Name": "Alias", "Mask": "*.mine", "Highlight.Enabled": "true", tc.alias: "foreground:#123456"}}
			groups, current := captureGroupColors([]Group{original}, "Modern")
			if got := current.Colors("Alias")[tc.canonical]; got != "foreground:#123456" {
				t.Fatalf("alias was not captured: got %q", got)
			}
			if _, leftover := groups[0].Fields[tc.alias]; leftover {
				t.Fatal("live alias remained outside the colour theme")
			}
			if groups[0].Fields["Legacy.Color."+tc.alias] != original.Fields[tc.alias] {
				t.Fatal("original alias value was not preserved")
			}
			normalized := NormalizeColorGroups([]Group{original}, "Modern")[0]
			if _, leftover := normalized.Fields[tc.alias]; leftover {
				t.Fatal("alias-only record bypassed normalization")
			}
			original.Fields[tc.canonical] = ""
			_, current = captureGroupColors([]Group{original}, "Modern")
			if got, present := current.Colors("Alias")[tc.canonical]; !present || got != "" {
				t.Fatal("alias resurrected an explicitly cleared canonical state")
			}
		})
	}
	group := Group{ID: "palette", Fields: map[string]string{"Name": "Palette", PalettePrefix("Modern") + "NormalFileName": "foreground:#abcdef", PalettePrefix("Modern") + "Highlight.Enabled": "true"}}
	groups, current := captureGroupColors([]Group{group}, "Modern")
	if current.Colors("Palette")["NormalColor"] != "foreground:#abcdef" || groups[0].Fields["Legacy.Color."+PalettePrefix("Modern")+"NormalFileName"] != "foreground:#abcdef" {
		t.Fatal("active palette alias was lost")
	}
	group = Group{ID: "cursor", Fields: map[string]string{"Name": "Cursor", "NormalColorUnderCursor": "", "FileNameUnderCursor": "foreground:#123456"}}
	_, current = captureGroupColors([]Group{group}, "Modern")
	if current.Colors("Cursor")["CursorColor"] != "foreground:#123456" {
		t.Fatal("empty earlier alias hid a populated supported alias")
	}
}

func TestCaptureGroupColorsPrefersEnabledActiveRulesWithoutReordering(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first map[string]string
	}{
		{name: "group disabled", first: map[string]string{"Enabled": "false"}},
		{name: "highlight disabled", first: map[string]string{"Highlight.Enabled": "false"}},
		{name: "inactive style", first: map[string]string{"ColorStyle": "Other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := Group{ID: "earlier", Fields: map[string]string{"Name": "IMAGES", "Mask": "*.png", "NormalColor": "red", "SelectedColor": "disabled-only", "Highlight.Enabled": "true", "IconTheme": "zoin"}}
			maps.Copy(first.Fields, tc.first)
			second := Group{ID: "active", Fields: map[string]string{"Name": "Images", "Mask": "*.jpg", "NormalColor": "green", "Highlight.Enabled": "true", "ColorStyle": "Modern"}}
			groups, current := captureGroupColors([]Group{first, second}, "Modern")
			if current.Colors("images")["NormalColor"] != "green" {
				t.Fatal("earlier disabled/inactive group replaced active colours")
			}
			if _, leaked := current.Colors("images")["SelectedColor"]; leaked {
				t.Fatal("disabled duplicate supplied a state absent from the active winner")
			}
			if groups[0].ID != first.ID || groups[1].ID != second.ID || groups[0].Fields["Mask"] != "*.png" || groups[1].Fields["Mask"] != "*.jpg" || groups[0].IconTheme() != "zoin" {
				t.Fatal("colour capture reordered rules or changed classification/scope")
			}
			if groups[0].Fields["Legacy.Color.NormalColor"] != "red" || groups[1].Fields["Legacy.Color.NormalColor"] != "green" {
				t.Fatal("ambiguous original colours were discarded")
			}
		})
	}
	first := Group{ID: "first", Fields: map[string]string{"Name": "Images", "Mask": "*.png", "Highlight.Enabled": "true", "NormalColor": "first"}}
	second := Group{ID: "second", Fields: map[string]string{"Name": "Images", "Mask": "*.jpg", "Highlight.Enabled": "true", "NormalColor": "second"}}
	groups, current := captureGroupColors([]Group{first, second}, "Modern")
	if current.Colors("Images")["NormalColor"] != "first" || groups[1].Fields["Legacy.Color.NormalColor"] != "second" {
		t.Fatal("active duplicate policy or recovery metadata changed")
	}
}

func TestColorMigrationAliasesAndDisabledFeatureRoundTrip(t *testing.T) {
	store, original := colorMigrationStore(t)
	doc, err := Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := doc.Groups()
	if err != nil {
		t.Fatal(err)
	}
	prefix := PalettePrefix("Modern")
	groups[0].Fields["Enabled"] = "false"
	delete(groups[0].Fields, prefix+"NormalColor")
	groups[0].Fields[prefix+"NormalFileName"] = "foreground:#abcdef"
	data, err := doc.encodeSnapshot(groups, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CommitColors(tx.Groups(), tx.Themes(), tx.ColorThemes(), tx.ColorThemeID()); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	clean := reloaded.Groups()[0]
	if clean.Fields["Enabled"] != "false" || clean.Fields["Highlight.Enabled"] != "true" || clean.Fields["Legacy.Color."+prefix+"NormalFileName"] != "foreground:#abcdef" {
		t.Fatalf("round trip lost feature enablement or original alias: %v", clean.Fields)
	}
	if clean.Commands[0].Commands != groups[0].Commands[0].Commands || clean.Commands[0].Enabled != groups[0].Commands[0].Enabled {
		t.Fatal("migration changed application commands")
	}
	clean.Fields["Enabled"] = "true"
	snapshot, err := Compile([]Group{clean})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = snapshot.WithColorThemes(reloaded.ColorThemes(), reloaded.ColorThemeID())
	if err != nil {
		t.Fatal(err)
	}
	projected := snapshot.HighlightGroups("Modern")[0]
	if !projected.Enabled("Highlight") || projected.Fields["NormalColor"] != "foreground:#abcdef" || !snapshot.HighlightPredicate(clean.ID, "Modern").Match(&vfs.VFSItem{Name: "extensionless", IsExecutable: true}, true, time.Now()) {
		t.Fatal("re-enabled migrated group lost its colour or highlight predicate")
	}
}

func TestNormalizeColorGroupsRetainsConflictingRecoveryMetadata(t *testing.T) {
	original := Group{ID: "x", Fields: map[string]string{"Name": "X", "NormalColor": "new", "Legacy.Color.NormalColor": "previous", "NormalFileName": "alias", "Vendor.Keep": "yes"}}
	groups := NormalizeColorGroups([]Group{original}, "Modern")
	if groups[0].Fields["Legacy.Color.NormalColor"] != "previous" || groups[0].Fields["Vendor.Keep"] != "yes" {
		t.Fatal("existing metadata overwritten")
	}
	found := false
	for key, value := range groups[0].Fields {
		found = found || strings.HasPrefix(key, "Legacy.Color.") && value == "new"
	}
	if !found {
		t.Fatal("new conflicting colour value not recoverable")
	}
	if !reflect.DeepEqual(NormalizeColorGroups(groups, "Modern"), groups) {
		t.Fatal("normalization duplicated recovery metadata")
	}
}
