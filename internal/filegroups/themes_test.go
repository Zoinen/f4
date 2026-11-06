package filegroups

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemeIconTintPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, catalog, value string
		want                 bool
	}{
		{"lucide", "lucide", "true", true},
		{"zoin", "zoin", "true", true},
		{"opt out", "zoin", "false", false},
		{"unspecified", "zoin", "", false},
		{"native cannot tint", "system", "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := ThemeSet{ID: "test", Fields: map[string]string{"IconCatalog": tc.catalog, "TintIcons": tc.value}}
			if got := set.TintIcons(); got != tc.want {
				t.Fatalf("TintIcons() = %v, want %v", got, tc.want)
			}
		})
	}
	if err := ValidateThemes([]ThemeSet{{ID: "bad", Fields: map[string]string{"TintIcons": "invalid"}}}); err == nil {
		t.Fatal("invalid tint policy accepted")
	}
}

func TestThemeTintSnapshotsAndRuntime(t *testing.T) {
	t.Parallel()
	sets := []ThemeSet{{ID: "zoin", Fields: map[string]string{"IconCatalog": "zoin", "TintIcons": "true"}}}
	runtime, err := NewRuntime(nil, nil, sets)
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetIconTheme("zoin")
	before := runtime.Appearance()
	sets[0].Fields["TintIcons"] = "false"
	if tint, present := before.IconTint(); !present || !tint {
		t.Fatal("snapshot did not retain immutable theme policy")
	}
	draft, err := Compile(nil, sets)
	if err != nil {
		t.Fatal(err)
	}
	session := runtime.BeginPreview()
	session.Publish(draft)
	if tint, present := runtime.Appearance().IconTint(); !present || tint {
		t.Fatal("tint opt-out not previewed")
	}
	if before.Revision() == runtime.Appearance().Revision() || before.SortRevision() != runtime.Appearance().SortRevision() {
		t.Fatal("tint must invalidate appearance, not sorting")
	}
	session.Cancel()
	if tint, _ := runtime.Appearance().IconTint(); !tint {
		t.Fatal("cancel lost baseline tint")
	}
	if err := runtime.Apply(nil, sets); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Preview([]Group{{ID: "added", Fields: map[string]string{"Name": "draft"}}}); err != nil {
		t.Fatal(err)
	}
	if tint, present := runtime.Appearance().IconTint(); !present || tint {
		t.Fatal("group preview lost explicit opt-out")
	}
	runtime.CancelPreview()
	if tint, present := runtime.Appearance().IconTint(); !present || tint {
		t.Fatal("cancel rolled back applied policy")
	}
	if _, present := runtime.Appearance().ForIconTheme("unknown").IconTint(); present {
		t.Fatal("unknown theme borrowed another policy")
	}
}

func TestThemeDescriptorsLayerAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "highlight.ini")
	store := Store{Path: path, ThemeDefaults: []ThemeSet{{ID: "zoin", Fields: map[string]string{"Name": "Zoin", "IconCatalog": "zoin", "TintIcons": "true", "Future": "retained"}}}}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	themes := tx.Themes()
	themes = append(themes, ThemeSet{ID: "colours", Fields: map[string]string{"Name": "Colours only", "IconCatalog": "", "Unknown": `C:\Users\test`}})
	for i := range themes {
		if themes[i].ID == "zoin" {
			themes[i].Fields["Name"] = "My Zoin"
			themes[i].Fields["TintIcons"] = "false"
		}
	}
	if err := tx.Commit(nil, themes); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Future =") {
		t.Fatal("untouched default frozen in overrides")
	}
	reloaded, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range reloaded.Themes() {
		switch set.ID {
		case "zoin":
			if set.Name() != "My Zoin" || set.Catalog() != "zoin" || set.Fields["Future"] != "retained" || set.TintIcons() {
				t.Fatal(set)
			}
		case "colours":
			if set.Catalog() != "" || set.Fields["Unknown"] != `C:\Users\test` {
				t.Fatal(set)
			}
		}
	}
	// A group referring to an unavailable theme still exposes that set.
	sets := MergeThemes(nil, nil, []Group{{ID: "rule", Fields: map[string]string{"IconTheme": "missing"}}})
	if len(sets) != 1 || sets[0].ID != "missing" {
		t.Fatal(sets)
	}
}
