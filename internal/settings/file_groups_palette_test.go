package settings

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestFileGroupNestedSettingsControlsHaveDistinctIDs(t *testing.T) {
	setupFileGroupSettings(t)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	parent := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}})
	parent.selectCategory("filegroups")
	child := parent.fileGroupOverview.childEditor(0)
	defer child.sessions[0].draft.Close()
	pairs := []struct {
		name          string
		parent, child vtui.UIElement
	}{
		{name: "categories", parent: parent.sidebar, child: child.sidebar},
		{name: "search", parent: parent.search, child: child.search},
		{name: "content", parent: parent.page, child: child.page},
		{name: "accept", parent: parent.ok, child: child.ok},
		{name: "cancel", parent: parent.cancel, child: child.cancel},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			if vtui.SemanticID(pair.parent) == vtui.SemanticID(pair.child) {
				t.Fatal("child event identity collides with parent Settings")
			}
		})
	}
}

func TestCanonicalShippedFileGroupsHaveNoDuplicateOrUserRows(t *testing.T) {
	store := setupFileGroupSettings(t)
	store.Defaults = theme.FileGroupDefaults()
	ConfigureFileGroups(store, fileGroupRuntime)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	center := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}})
	center.selectCategory("filegroups")
	names := map[string]string{}
	for _, row := range center.fileGroupOverview.rows {
		if row.separator != "" {
			continue
		}
		name := strings.ToLower(row.record.Values["group.Name"])
		if previous := names[name]; previous != "" {
			t.Errorf("duplicate %q: %s and %s", name, previous, row.record.ID)
		}
		names[name] = row.record.ID
		if source := row.cellText(4); source != "" {
			t.Errorf("group %s still has a source cell: %s", row.record.ID, source)
		}
	}
	for _, name := range []string{"archives", "executables", "temporary files"} {
		if names[name] == "" {
			t.Errorf("canonical group %s missing", name)
		}
	}
	t.Logf("canonical built-in groups: %d", len(names))
}

func TestPaletteGroupEditorKeepsOneRecordAndEditsCurrentPalette(t *testing.T) {
	store := setupFileGroupSettings(t)
	oldStyle := config.App.ColorStyle
	config.App.ColorStyle = "Test palette"
	t.Cleanup(func() { config.App.ColorStyle = oldStyle })
	prefix := "Palette." + hex.EncodeToString([]byte(config.App.ColorStyle)) + "."
	other := "Palette." + hex.EncodeToString([]byte("Other")) + "."
	store.Defaults = []filegroups.Group{{ID: "type.executables", Fields: map[string]string{
		"Name": "Executables", "Mask": "*.exe", "MaskSyntax": "far",
		prefix + "Highlight.Enabled": "true", prefix + "NormalColor": "foreground:#112233",
		prefix + "IncludeAttributes": "Executable", prefix + "Mask": "*",
		other + "Highlight.Enabled": "true", other + "NormalColor": "foreground:#ABCDEF",
	}}}
	ConfigureFileGroups(store, fileGroupRuntime)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	center := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}})
	center.selectCategory("filegroups")
	child := center.fileGroupOverview.childEditor(0)
	scratch := child.sessions[0].draft
	defer scratch.Close()
	foundColour, foundCondition := false, false
	for _, f := range child.sessions[0].catalog.Fields {
		foundColour = foundColour || f.ID == "group."+prefix+"NormalColor"
		foundCondition = foundCondition || f.ID == "group."+prefix+"IncludeAttributes"
	}
	if !foundColour || !foundCondition {
		t.Fatalf("missing current-palette controls: colour=%v condition=%v", foundColour, foundCondition)
	}
	scratch.Values["group."+prefix+"NormalColor"] = "foreground:#445566"
	result := scratch.Commit(context.Background())
	if len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if len(draft.Records["filegroups"]) != 1 {
		t.Fatal("palette edit created another file group")
	}
	group := draft.Records["filegroups"][0]
	if group.Values["group.Mask"] != "*.exe" || group.Values["group."+other+"NormalColor"] != "foreground:#ABCDEF" {
		t.Fatal("palette edit changed classification or another palette")
	}
	child.resetFileGroupEditorFieldValue("group." + prefix + "NormalColor")
	if scratch.Values["group."+prefix+"NormalColor"] != "foreground:#112233" {
		t.Fatal("palette reset lost factory colour")
	}
}

func TestPaletteTableReorderChangesOnlyCurrentPalettePriority(t *testing.T) {
	setupFileGroupSettings(t)
	old := config.App.ColorStyle
	config.App.ColorStyle = "A"
	t.Cleanup(func() { config.App.ColorStyle = old })
	prefix := filegroups.PalettePrefix("A")
	other := filegroups.PalettePrefix("B")
	groups := []filegroups.Group{
		{ID: "first", Fields: map[string]string{prefix + "Highlight.Enabled": "true", prefix + "Order": "9", other + "Order": "1"}},
		{ID: "second", Fields: map[string]string{prefix + "Highlight.Enabled": "true", prefix + "Order": "0", other + "Order": "2"}},
	}
	draft := f4settings.NewDraft(nil, groupsToRecords(groups))
	reorderFileGroupPalette(draft.Records["filegroups"], "")
	restored, err := recordsToGroups(draft.Records)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := filegroups.Compile(restored)
	if err != nil {
		t.Fatal(err)
	}
	ordered := snapshot.HighlightGroups("A")
	if ordered[0].ID != "first" || ordered[1].ID != "second" {
		t.Fatal("palette order ignored table move")
	}
	if restored[0].Fields[other+"Order"] != "1" || restored[1].Fields[other+"Order"] != "2" {
		t.Fatal("moving a group changed an inactive palette")
	}
}

func TestFileGroupLabelsHaveNoOriginDistinction(t *testing.T) {
	setupFileGroupSettings(t)
	rows := groupsToRecords([]filegroups.Group{{ID: "legacy.highlight_5", Fields: map[string]string{
		"Name": "Executable", "Mask": "*.exe", "Highlight.Enabled": "true",
	}}})
	label := fileGroupRecordLabel(rows["filegroups"][0])
	for _, origin := range []string{"[Imported]", "[User]", "[Built-in]", "[Modified]", "[Disabled]"} {
		if strings.Contains(label, origin) {
			t.Fatal("origin label remains:", label)
		}
	}
	view := &fileGroupsOverview{}
	row := fileGroupOverviewRow{view: view, record: rows["filegroups"][0]}
	if row.cellText(4) != "" {
		t.Fatal("overview still shows source")
	}
}
