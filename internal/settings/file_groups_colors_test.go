package settings

import (
	"context"
	"maps"
	"testing"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/vtui"
)

func setupFileGroupColors(t *testing.T) filegroups.Store {
	t.Helper()
	store := setupFileGroupSettings(t)
	manager := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = manager })
	store.Defaults[0].Fields["Name"] = "Go source"
	store.Defaults[0].Fields["NormalColor"] = "foreground:#112233"
	store.ColorDefaults = []filegroups.ColorTheme{
		{ID: "current", Fields: map[string]string{"Name": "Current", "Color.go-source.NormalColor": "foreground:#112233"}},
		{ID: "zoin", Fields: map[string]string{"Name": "Zoin", "Color.go-source.NormalColor": "foreground:#6FABE2"}},
	}
	runtime, err := filegroups.NewRuntime(store.StandardGroups(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ApplyColors(store.StandardGroups(), nil, store.ColorDefaults, "current"); err != nil {
		t.Fatal(err)
	}
	ConfigureFileGroups(store, runtime)
	return store
}

func TestFileGroupsColorSelectorIsIndependentAndReversible(t *testing.T) {
	setupFileGroupColors(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	iconBefore := v.themeID
	for i, id := range v.colorIDs {
		if id == "zoin" {
			v.colors.Menu.OnAction(i)
		}
	}
	if v.themeID != iconBefore || fileGroupActiveTheme != "generic" {
		t.Fatal("colour selection changed icon context")
	}
	if fileGroupRuntime.Appearance().ColorThemeID() != "zoin" || CommittedFileGroups().ColorThemeID() != "current" {
		t.Fatalf("colour selection did not remain reversible preview: appearance=%q committed=%q draft=%q status=%q",
			fileGroupRuntime.Appearance().ColorThemeID(), CommittedFileGroups().ColorThemeID(), fileGroupDraftColorThemeID(d), c.status)
	}
	if got := v.previews["factory"].style.Normal.Foreground; got != "#6FABE2" {
		t.Fatalf("selected colour palette did not reach table: %q", got)
	}
	SetFileGroupIconTheme("lucide")
	if fileGroupRuntime.Appearance().ColorThemeID() != "zoin" {
		t.Fatal("icon switch restored stale colours")
	}
	d.Close()
	if fileGroupRuntime.Appearance().ColorThemeID() != "current" || fileGroupRuntime.IconTheme() != "lucide" {
		t.Fatal("Cancel reverted icon selection or kept preview colours")
	}
}

func TestFileGroupsColorSelectionApplyAndCancel(t *testing.T) {
	store := setupFileGroupColors(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Values[fileGroupColorThemeKey] = "zoin"
	result := d.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	d.Accept(result)
	d.Values[fileGroupColorThemeKey] = "current"
	if err := d.PreviewFunc(d); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if got := fileGroupRuntime.Appearance().ColorThemeID(); got != "zoin" {
		t.Fatalf("Cancel rolled back an applied colour theme: %s", got)
	}
	tx, err := store.Begin()
	if err != nil || tx.ColorThemeID() != "zoin" {
		t.Fatalf("saved theme not restored: %v", err)
	}
	for key := range tx.Groups()[0].Fields {
		if key == "NormalColor" {
			t.Fatal("colours were saved inside a group instead of the selected theme")
		}
	}
}

func TestFileGroupsInvalidColorKeepsLastPreview(t *testing.T) {
	setupFileGroupColors(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Values[fileGroupColorThemeKey] = "zoin"
	if err := d.PreviewFunc(d); err != nil {
		t.Fatal(err)
	}
	before := fileGroupRuntime.Appearance().Revision()
	for i, set := range d.Records["filegroups.colorthemes"] {
		if set.ID == "zoin" {
			set.Values = maps.Clone(set.Values)
			set.Values["Color.go-source.NormalColor"] = "foreground:#oops"
			d.Records["filegroups.colorthemes"][i] = set
		}
	}
	if err := d.PreviewFunc(d); err == nil {
		t.Fatal("invalid theme colour accepted")
	}
	if before != fileGroupRuntime.Appearance().Revision() {
		t.Fatal("invalid colour replaced the last valid preview")
	}
}
