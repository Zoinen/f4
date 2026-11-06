package filegroups

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func colorTestThemes() []ColorTheme {
	return []ColorTheme{
		{ID: "warm", Fields: map[string]string{"Name": "Warm", "Color.source-code.NormalColor": "foreground:#ff0000", "Color.source-code.SelectedColor": "", "Vendor.Future": "kept"}},
		{ID: "cool", Fields: map[string]string{"Name": "Cool", "Color.source-code.NormalColor": "foreground:#0000ff"}},
	}
}

func TestColorThemesComposeWithoutCompilingOrChangingIcons(t *testing.T) {
	groups := []Group{{ID: "source", Fields: map[string]string{
		"Name": "SOURCE code", "Mask": "*.exe", "Highlight.Mask": "*", "Highlight.IncludeAttributes": "Executable",
		"NormalColor": "old", "Highlight.Enabled": "true", "Icons.Enabled": "true", "Icon.*": "asset:source",
		"Mark": "*", "ContinueProcessing": "false", "UseDefaults": "true", "Associations.Enabled": "true",
	}, Commands: []CommandSet{{ID: "open", Commands: [6]string{"execute", "alternate", "view", "alt-view", "edit", "alt-edit"}, Enabled: [6]bool{true, false, true, true, false, true}}}}}
	base, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := base.ForIconTheme("zoin").WithColorThemes(colorTestThemes(), "warm")
	if err != nil {
		t.Fatal(err)
	}
	for _, icon := range []string{"zoin", "lucide", "system", "zoin"} {
		for _, selected := range []string{"cool", "warm"} {
			view, err := warm.ForIconTheme(icon).WithColorThemes(colorTestThemes(), selected)
			if err != nil {
				t.Fatal(err)
			}
			if view.ColorThemeID() != selected || view.iconTheme != icon || view.ForIconTheme("system").ColorThemeID() != selected {
				t.Fatal("icon and colour selections became coupled")
			}
			if view.Predicate("source") != base.Predicate("source") || view.HighlightPredicate("source", "any-style") != base.HighlightPredicate("source", "") {
				t.Fatal("colour switch recompiled predicates")
			}
			if view.SortRevision() != base.SortRevision() {
				t.Fatal("colour selection requested a resort")
			}
			g := view.HighlightGroups("unrelated palette")[0]
			if g.Fields["NormalColor"] != colorTestThemes()[map[string]int{"warm": 0, "cool": 1}[selected]].Colors("source code")["NormalColor"] || g.Fields["Mark"] != "*" || g.Fields["UseDefaults"] != "true" || g.Fields["ContinueProcessing"] != "false" {
				t.Fatalf("colour projection changed group policy: %v", g.Fields)
			}
			if !reflect.DeepEqual(view.Groups()[0].Commands, groups[0].Commands) {
				t.Fatal("colour selection changed command dispatch")
			}
		}
	}
	if base.ColorThemeID() != "" || base.Groups()[0].Fields["NormalColor"] != "old" {
		t.Fatal("composition mutated source")
	}
	colors := warm.ColorThemes()
	colors[0].Fields["Vendor.Future"] = "changed"
	if warm.ColorThemes()[0].Fields["Vendor.Future"] != "kept" {
		t.Fatal("colour themes expose mutable snapshot state")
	}
	if _, present := warm.HighlightGroups("")[0].Fields["SelectedColor"]; !present {
		t.Fatal("explicit empty colour state lost")
	}
	if !warm.HighlightPredicate("source", "ignored").Match(&vfs.VFSItem{Name: "extensionless", IsExecutable: true}, true, time.Now()) || warm.Predicate("source").Match(&vfs.VFSItem{Name: "extensionless", IsExecutable: true}, true, time.Now()) {
		t.Fatal("highlighting used the icon predicate")
	}
}

func TestColorThemeMissingEntriesFallThroughAndExplicitDisable(t *testing.T) {
	groups := []Group{
		{ID: "special", Fields: map[string]string{"Name": "Special", "Mask": "*.go", "Highlight.Enabled": "true"}},
		{ID: "marked", Fields: map[string]string{"Name": "Marked", "Mask": "*.go", "Highlight.Enabled": "true", "Mark": "!"}},
		{ID: "off", Fields: map[string]string{"Name": "Off", "Mask": "*", "Highlight.Enabled": "false", "Mark": "?"}},
		{ID: "files", Fields: map[string]string{"Name": "Files", "Mask": "*"}},
	}
	base, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := base.WithColorThemes([]ColorTheme{{ID: "colors", Fields: map[string]string{"Color.files.NormalColor": "foreground:#abcdef", "Color.off.NormalColor": "foreground:#123456"}}}, "colors")
	if err != nil {
		t.Fatal(err)
	}
	projected := snapshot.HighlightGroups("Modern")
	if projected[0].Enabled("Highlight") || !projected[1].Enabled("Highlight") || projected[1].Fields["ContinueProcessing"] != "true" || projected[1].Fields["Mark"] != "!" || projected[2].Enabled("Highlight") || !projected[3].Enabled("Highlight") {
		t.Fatalf("missing artwork blocked fallback or explicit disable lost: %+v", projected)
	}
	var color, mark string
	for _, group := range projected {
		if !group.Enabled("Highlight") || !snapshot.HighlightPredicate(group.ID, "Modern").Match(&vfs.VFSItem{Name: "main.go"}, true, time.Now()) {
			continue
		}
		if group.Fields["Mark"] != "" {
			mark = group.Fields["Mark"]
		}
		if group.Fields["NormalColor"] != "" {
			color = group.Fields["NormalColor"]
		}
		if !booleanValue(group.Fields["ContinueProcessing"], false) {
			break
		}
	}
	if color != "foreground:#abcdef" || mark != "!" {
		t.Fatalf("fallback colour=%q marker=%q", color, mark)
	}
}

func TestColorThemeNamesAndUnknownFields(t *testing.T) {
	theme := ColorTheme{ID: "custom", Fields: map[string]string{"Color.go-source.NormalColor": "", "Color.go-source.CursorColor": "foreground:#123456", "Color.go-source.Unknown": "ignored", "Vendor.Future": "preserved"}}
	if theme.Name() != "custom" || !maps.Equal(theme.Colors("GO / Source"), map[string]string{"NormalColor": "", "CursorColor": "foreground:#123456"}) {
		t.Fatal("name normalization or explicit clears changed")
	}
	if err := ValidateColorThemes([]ColorTheme{theme, theme}); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	if len(FactoryColorThemes()) == 0 {
		t.Fatal("embedded colour themes were not loaded")
	}
}

func TestColorThemeRuntimePreviewCancelAndSelection(t *testing.T) {
	groups := []Group{{ID: "source", Fields: map[string]string{"Name": "Source code", "Mask": "*"}}}
	runtime, err := NewRuntime(groups, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ApplyColors(groups, nil, colorTestThemes(), "warm"); err != nil {
		t.Fatal(err)
	}
	predicate := runtime.Appearance().HighlightPredicate("source", "")
	first, second := runtime.BeginPreview(), runtime.BeginPreview()
	preview, err := runtime.Appearance().WithColorThemes(colorTestThemes(), "cool")
	if err != nil {
		t.Fatal(err)
	}
	first.Publish(preview)
	second.Publish(preview)
	first.Cancel()
	if runtime.Appearance().ColorThemeID() != "cool" || runtime.Committed().ColorThemeID() != "warm" {
		t.Fatal("colour preview escaped its owner or changed committed baseline")
	}
	runtime.SetIconTheme("zoin")
	second.Cancel()
	if runtime.Appearance().ColorThemeID() != "warm" || runtime.IconTheme() != "zoin" {
		t.Fatal("Cancel restored stale icon context")
	}
	if err := runtime.SetColorTheme("cool"); err != nil {
		t.Fatal(err)
	}
	if runtime.Appearance().HighlightPredicate("source", "") != predicate {
		t.Fatal("selecting colour theme recompiled masks")
	}
	if err := runtime.Apply(groups); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Preview(groups); err != nil {
		t.Fatal(err)
	}
	if runtime.Appearance().ColorThemeID() != "cool" || len(runtime.Appearance().ColorThemes()) != 2 {
		t.Fatal("legacy runtime API discarded colour data")
	}
	runtime.CancelPreview()
}

func TestColorThemeSelectionAndFieldEditsChangePreviewRevision(t *testing.T) {
	groups := []Group{{ID: "source", Fields: map[string]string{"Name": "Source code", "Mask": "*"}}}
	runtime, err := NewRuntime(groups, nil)
	if err != nil {
		t.Fatal(err)
	}
	themes := colorTestThemes()
	if err := runtime.ApplyColors(groups, nil, themes, "warm"); err != nil {
		t.Fatal(err)
	}
	base := runtime.Committed()
	selected, err := base.WithColorThemes(themes, "cool")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Revision() == base.Revision() || selected.source.Revision() == base.source.Revision() {
		t.Fatal("colour selection missing from root or icon-view revision")
	}
	themes[1].Fields["Color.source-code.NormalColor"] = "foreground:#abcdef"
	edited, err := selected.WithColorThemes(themes, "cool")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Revision() == selected.Revision() || edited.source.Revision() == selected.source.Revision() {
		t.Fatal("colour fields missing from revision")
	}
	owner := runtime.BeginPreview()
	owner.Publish(selected)
	if runtime.Appearance().ColorThemeID() != "cool" || runtime.Committed().ColorThemeID() != "warm" {
		t.Fatal("runtime suppressed a selected-colour preview")
	}
	owner.Publish(edited)
	if runtime.Appearance().HighlightGroups("")[0].Fields["NormalColor"] != "foreground:#abcdef" {
		t.Fatal("runtime suppressed a colour-field preview")
	}
	owner.Cancel()
}

func colorMigrationStore(t *testing.T) (Store, []byte) {
	t.Helper()
	prefix := PalettePrefix("Modern")
	group := Group{ID: "programs", Fields: map[string]string{
		"Name": "Source code", "Mask": "*.exe", "Icons.Enabled": "true", "Icon.*": "asset:source", "Vendor.Keep": "yes",
		prefix + "Mask": "*", prefix + "IncludeAttributes": "Executable", prefix + "ExcludeAttributes": "Directory", prefix + "Highlight.Enabled": "true",
		prefix + "NormalColor": "foreground:#112233", prefix + "SelectedColor": "", prefix + "Mark": "*", prefix + "ContinueProcessing": "true", prefix + "UseDefaults": "false",
		prefix + "Vendor.Future": "keep legacy extension",
	}, Commands: []CommandSet{{ID: "open", Name: "Application", Commands: [6]string{"one", "two", "three", "four", "five\nline two", `C:\six.exe`}, Enabled: [6]bool{true, false, true, false, true, true}, Extra: map[string]string{"Vendor.Command": "kept"}}}}
	doc, err := Parse([]byte("# personal comment\n[Vendor.Section]\nKeep=personal\n"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.encodeSnapshot([]Group{group}, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Path: filepath.Join(t.TempDir(), "highlight.ini"), ColorDefaults: colorTestThemes(), ColorStyle: "Modern", Defaults: []Group{group}}
	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return store, data
}

func TestColorThemeStoreMigrationAndLegacyCommit(t *testing.T) {
	store, original := colorMigrationStore(t)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	assertPaletteFile(t, store.Path, original)
	if tx.ColorThemeID() != "current" || !tx.Migrated() || len(tx.ColorThemes()) != 3 {
		t.Fatal("old working set not captured into Current")
	}
	group := tx.Groups()[0]
	for key := range group.Fields {
		if strings.HasPrefix(key, "Palette.") || key == "NormalColor" || key == "SelectedColor" {
			t.Fatalf("old colour namespace survived: %s", key)
		}
	}
	if group.Fields["Highlight.IncludeAttributes"] != "Executable" || group.Fields["Highlight.Mask"] != "*" || group.Fields["Mask"] != "*.exe" || group.Fields["Mark"] != "*" || group.Fields["ContinueProcessing"] != "true" || group.Fields["Vendor.Keep"] != "yes" || group.Fields["Legacy."+PalettePrefix("Modern")+"Vendor.Future"] != "keep legacy extension" {
		t.Fatalf("migration changed conditions/policy/unknowns: %v", group.Fields)
	}
	current := tx.ColorThemes()[2]
	if current.Colors("SOURCE CODE")["NormalColor"] != "foreground:#112233" {
		t.Fatal("current colour was not captured")
	}
	if _, present := current.Colors("source code")["SelectedColor"]; !present {
		t.Fatal("explicit selected clear lost")
	}
	if err := tx.Commit(tx.Groups()); err != nil {
		t.Fatal(err)
	}
	assertPaletteFile(t, store.Path+".pre-filegroups-colors", original)
	reloaded, err := store.Begin()
	if err != nil || reloaded.Migrated() {
		t.Fatal("colour migration repeated", err)
	}
	if !reflect.DeepEqual(reloaded.ColorThemes(), tx.ColorThemes()) || reloaded.ColorThemeID() != "current" {
		t.Fatal("legacy Commit did not preserve captured themes")
	}
	command := reloaded.Groups()[0].Commands[0]
	want := store.Defaults[0].Commands[0]
	if command.Commands != want.Commands || command.Enabled != want.Enabled || command.Extra["Vendor.Command"] != "kept" {
		t.Fatal("migration changed commands")
	}
	saved, err := os.ReadFile(store.Path)
	if err != nil || !strings.Contains(string(saved), "[Vendor.Section]\nKeep=personal") || !strings.Contains(string(saved), "# personal comment") {
		t.Fatal("unknown document data lost", err)
	}
}

func TestColorThemeStoreFailureAndExternalConflict(t *testing.T) {
	for _, external := range []bool{false, true} {
		store, original := colorMigrationStore(t)
		failure := errors.New("write failed")
		store.Write = func(string, []byte, os.FileMode) error { return failure }
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if external {
			if err := os.WriteFile(store.Path, append(original, []byte("# external\n")...), 0600); err != nil {
				t.Fatal(err)
			}
		}
		err = tx.CommitColors(tx.Groups(), tx.Themes(), colorTestThemes(), "cool")
		want := failure
		if external {
			want = ErrConflict
		}
		if !errors.Is(err, want) || tx.ColorThemeID() != "current" || !tx.Migrated() {
			t.Fatal("failed save changed baseline or ignored conflict", err)
		}
		if _, err := os.Stat(store.Path + ".pre-filegroups-colors"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed Apply retained newly created colour backup", err)
		}
		if !external {
			assertPaletteFile(t, store.Path, original)
		}
	}
}

func TestCapturedCurrentDoesNotInheritUncapturedFactoryColors(t *testing.T) {
	store, _ := colorMigrationStore(t)
	store.ColorDefaults = append(store.ColorDefaults, ColorTheme{ID: "current", Fields: map[string]string{
		"Name": "Factory Current", "Color.unseen.NormalColor": "factory-only", "Color.source-code.CursorColor": "factory-only",
	}})
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range tx.ColorThemes() {
		if theme.ID != "current" {
			continue
		}
		if len(theme.Colors("Unseen")) != 0 {
			t.Fatal("capture enabled an absent legacy highlighting rule")
		}
		if _, inherited := theme.Colors("Source code")["CursorColor"]; inherited {
			t.Fatal("capture added a state missing from the old profile")
		}
	}
}

func TestCommitColorsNormalizesRawDefaultsAndPreservesUnknowns(t *testing.T) {
	store, _ := colorMigrationStore(t)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	colors := colorTestThemes()
	colors[1].Fields["Vendor.Windows"] = `C:\theme\unknown`
	if err := tx.CommitColors(store.Defaults, tx.Themes(), colors, "cool"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Begin()
	if err != nil || reloaded.ColorThemeID() != "cool" {
		t.Fatal("selection did not round trip", err)
	}
	if reloaded.ColorThemes()[0].Colors("source code")["NormalColor"] != "foreground:#0000ff" && reloaded.ColorThemes()[1].Colors("source code")["NormalColor"] != "foreground:#0000ff" {
		t.Fatal("raw group colours replaced selected theme")
	}
	for key := range reloaded.Groups()[0].Fields {
		if strings.HasPrefix(key, "Palette.") {
			t.Fatal("CommitColors stored raw palette data")
		}
	}
	if err := reloaded.Commit(reloaded.Groups()); err != nil {
		t.Fatal(err)
	}
	second, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, theme := range second.ColorThemes() {
		found = found || theme.Fields["Vendor.Windows"] == `C:\theme\unknown`
	}
	if !found {
		t.Fatal("unknown colour fields were discarded")
	}
	legacyStore := store
	legacyStore.ColorDefaults = nil
	legacyStore.ColorStyle = ""
	legacy, err := legacyStore.Begin()
	if err != nil || legacy.ColorThemeID() != "cool" {
		t.Fatal("zero-default store discarded loaded colour metadata", err)
	}
}

func TestColorDefaultsNilRetainsLegacyPathAndStandardGroupsClone(t *testing.T) {
	store, _ := colorMigrationStore(t)
	store.ColorDefaults = nil
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if tx.ColorThemeID() != "" || len(tx.ColorThemes()) != 0 || tx.Migrated() || tx.Groups()[0].Fields[PalettePrefix("Modern")+"NormalColor"] != "foreground:#112233" {
		t.Fatal("nil ColorDefaults changed legacy behaviour")
	}
	raw := store.StandardGroups()
	if !reflect.DeepEqual(raw, store.Defaults) {
		t.Fatal("nil ColorDefaults normalized standards")
	}
	raw[0].Fields["Vendor.Keep"] = "changed"
	if store.Defaults[0].Fields["Vendor.Keep"] != "yes" {
		t.Fatal("standard groups expose mutable defaults")
	}
	store.ColorDefaults = colorTestThemes()
	clean := store.StandardGroups()
	if clean[0].Fields["Highlight.IncludeAttributes"] != "Executable" || clean[0].Fields[PalettePrefix("Modern")+"NormalColor"] != "" || store.Defaults[0].Fields[PalettePrefix("Modern")+"NormalColor"] == "" {
		t.Fatal("standard groups failed to isolate predicate from appearance")
	}
	if !reflect.DeepEqual(NormalizeColorGroups(clean, "Modern"), clean) {
		t.Fatal("normalization was not idempotent")
	}
}
