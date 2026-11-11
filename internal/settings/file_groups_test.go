package settings

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFileGroupsVerticalArrowsStayInsidePage(t *testing.T) {
	setupFileGroupSettings(t)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	center := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}})
	center.selectCategory("filegroups")
	center.ResizeConsole(130, 35)
	center.SetPosition(0, 0, 129, 34)
	for _, key := range []struct {
		name string
		code uint16
	}{
		{name: "up", code: vtinput.VK_UP},
		{name: "down", code: vtinput.VK_DOWN},
	} {
		t.Run(key.name, func(t *testing.T) {
			center.SetFocusedItem(center.page)
			for range len(center.page.rows)*5 + 10 {
				center.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: key.code})
				if center.GetFocusedItem() != center.page {
					t.Fatalf("vertical arrow escaped File groups to %T", center.GetFocusedItem())
				}
			}
		})
	}
	// Keeping arrows local must not trap explicit pane navigation.
	center.fileGroupOverview.SetFocusedItem(center.fileGroupOverview.actions)
	center.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if center.GetFocusedItem() == center.page {
		t.Fatal("Tab could not leave File groups")
	}
}

func setupFileGroupSettings(t *testing.T) filegroups.Store {
	t.Helper()
	oldStore, oldRuntime, oldDir := fileGroupStore, fileGroupRuntime, config.GetF4ConfigDir()
	oldTheme := fileGroupActiveTheme
	fileGroupActiveTheme = "generic"
	config.CachedF4ConfigDir = t.TempDir()
	store := filegroups.Store{Path: filepath.Join(config.GetF4ConfigDir(), "highlight.ini"), Defaults: []filegroups.Group{{ID: "factory", Fields: map[string]string{"Name": "Factory", "Mask": "*.go", "Highlight.Enabled": "true", "NormalColor": "foreground:#123456"}}}}
	runtime, err := filegroups.NewRuntime(store.Defaults, nil)
	if err != nil {
		t.Fatal(err)
	}
	ConfigureFileGroups(store, runtime)
	t.Cleanup(func() {
		fileGroupStore, fileGroupRuntime, config.CachedF4ConfigDir = oldStore, oldRuntime, oldDir
		fileGroupActiveTheme = oldTheme
	})
	return store
}

func TestThemeScopedEditorRetainsHiddenDraftsAndCancelContext(t *testing.T) {
	store := setupFileGroupSettings(t)
	for _, themeID := range []string{"lucide", "zoin", "system"} {
		store.Defaults = append(store.Defaults, filegroups.Group{ID: themeID, Fields: map[string]string{
			"IconTheme": themeID, "Name": themeID, "Mask": "*", "Highlight.Enabled": "true",
		}})
	}
	runtime, err := filegroups.NewRuntime(store.Defaults, nil)
	if err != nil {
		t.Fatal(err)
	}
	ConfigureFileGroups(store, runtime)
	SetFileGroupIconTheme("lucide")
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	session := &settingsSession{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}
	center := newSettingsCenter([]*settingsSession{session})
	assertVisible := func(want ...string) {
		t.Helper()
		indices := center.collectionRecordIndices(session, "filegroups")
		if len(indices) != len(want) {
			t.Fatalf("visible=%v want=%v", indices, want)
		}
		for i, index := range indices {
			if draft.Records["filegroups"][index].ID != want[i] {
				t.Fatalf("visible row %d: %s want %s", i, draft.Records["filegroups"][index].ID, want[i])
			}
		}
	}
	assertVisible("factory", "lucide")
	center.fileGroupScopeFilter = "theme"
	assertVisible("lucide")
	draft.Records["filegroups"][1].Values["group.Name"] = "Hidden draft"
	SetFileGroupIconTheme("zoin")
	assertVisible("zoin")
	if err := draft.PreviewFunc(draft); err != nil {
		t.Fatal(err)
	}
	SetFileGroupIconTheme("system")
	assertVisible("system")
	center.fileGroupScopeFilter = "universal"
	assertVisible("factory")
	center.moveFileGroupScope(session, "system", "")
	assertVisible("factory", "system")
	result := draft.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	draft.Accept(result)
	SetFileGroupIconTheme("lucide")
	center.fileGroupScopeFilter = ""
	assertVisible("factory", "system", "lucide")
	if runtime.Committed().Groups()[2].Label() != "Hidden draft" {
		t.Fatal("Apply lost a hidden theme's draft")
	}
	SetFileGroupIconTheme("zoin")
	draft.Close()
	if runtime.IconTheme() != "zoin" {
		t.Fatal("Cancel restored obsolete theme")
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range tx.Groups() {
		if group.ID == "lucide" && group.Label() != "Hidden draft" {
			t.Fatal("hidden edit lost after restart")
		}
	}
}

func TestScopeFilterSemanticActionRebuildsVisibleRecords(t *testing.T) {
	store := setupFileGroupSettings(t)
	store.Defaults = append(store.Defaults, filegroups.Group{ID: "zoin", Fields: map[string]string{"IconTheme": "zoin", "Name": "Zoin rule"}})
	ConfigureFileGroups(store, fileGroupRuntime)
	SetFileGroupIconTheme("zoin")
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	session := &settingsSession{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}
	center := newSettingsCenter([]*settingsSession{session})
	center.selectCategory("filegroups")
	view := center.fileGroupOverview
	if view == nil {
		t.Fatal("overview missing")
	}
	if len(center.page.rows) != 0 {
		t.Fatal("inline fields remain on the overview")
	}
	view.themes.Menu.OnAction(0)
	if view.themeID != "generic" || fileGroupActiveTheme != "zoin" {
		t.Fatal("editor selector changed application theme")
	}
	for _, r := range view.rows {
		if r.record.ID == "zoin" {
			t.Fatal("inactive thematic row visible")
		}
	}
}

func TestThemeSwitchKeepsLastValidPreviewAndResetRestoresBaseline(t *testing.T) {
	store := setupFileGroupSettings(t)
	oldManager := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = oldManager })
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	draft.Records["filegroups"][0].Values["group.Name"] = "Valid draft"
	if err := draft.PreviewFunc(draft); err != nil {
		t.Fatal(err)
	}
	draft.Records["filegroups"][0].Values["group.Mask"] = "["
	if err := draft.PreviewFunc(draft); err == nil {
		t.Fatal("invalid draft accepted")
	}
	SetFileGroupIconTheme("zoin")
	if err := draft.PreviewFunc(draft); err == nil {
		t.Fatal("cached invalid draft lost its error")
	}
	if fileGroupRuntime.Appearance().Groups()[0].Label() != "Valid draft" {
		t.Fatal("theme dropped last valid preview")
	}
	draft.Records = groupsToRecords(store.Defaults)
	if err := draft.PreviewFunc(draft); err != nil {
		t.Fatal(err)
	}
	if fileGroupRuntime.Appearance().Groups()[0].Label() != "Factory" {
		t.Fatal("reset-to-clean left stale preview active")
	}
	if fileGroupRuntime.IconTheme() != "zoin" {
		t.Fatal("reset changed application context")
	}
}

func TestNewGroupIsSharedWhenAnIconThemeIsSelected(t *testing.T) {
	store := setupFileGroupSettings(t)
	store.Defaults = append(store.Defaults, filegroups.Group{ID: "tail", Fields: map[string]string{
		"IconTheme": "zoin", "FactoryFallback": "true", "Name": "Files", "Mask": "*", "Icons.Enabled": "true", "Icon.*": "asset:file",
	}})
	ConfigureFileGroups(store, fileGroupRuntime)
	SetFileGroupIconTheme("zoin")
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	session := &settingsSession{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}
	center := newSettingsCenter([]*settingsSession{session})
	center.fileGroupScopeFilter = "theme"
	center.selectCategory("filegroups")
	if !center.page.HandleSemanticAction(map[string]any{"target": "id:filegroups:action:0", "action": "activate"}) {
		t.Fatal("Add action not handled")
	}
	selected := center.offsets["record:filegroups"]
	records := draft.Records["filegroups"]
	addedID := records[selected].ID
	if records[selected].Values["group.IconTheme"] != "" {
		t.Fatal("icon theme selection must not scope newly added groups")
	}
	child := center.fileGroupOverview.editor
	if child == nil {
		t.Fatal("Add did not open child editor")
	}
	result := child.sessions[0].draft.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	child.sessions[0].draft.Close()
	records = draft.Records["filegroups"]
	for _, record := range records {
		if record.ID == addedID && record.Values["group.IconTheme"] != "" {
			t.Fatal("scope move failed")
		}
	}
	visible := center.collectionRecordIndices(session, "filegroups")
	if len(visible) != 1 || records[visible[0]].ID != "tail" {
		t.Fatal("moved group remained visible in old set")
	}
}

func TestOverviewOmitsInactiveColourStyleRules(t *testing.T) {
	store := setupFileGroupSettings(t)
	old := config.App.ColorStyle
	config.App.ColorStyle = "Active"
	t.Cleanup(func() { config.App.ColorStyle = old })
	for _, style := range []string{"Active", "Other", "Third"} {
		store.Defaults = append(store.Defaults, filegroups.Group{ID: "style-" + style, Fields: map[string]string{
			"Name": "Scripts", "ColorStyle": style, "Highlight.Enabled": "true", "Mask": "*.sh",
		}})
	}
	ConfigureFileGroups(store, fileGroupRuntime)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	center := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}})
	center.selectCategory("filegroups")
	count := 0
	for _, row := range center.fileGroupOverview.rows {
		if row.record.Values["group.Name"] == "Scripts" {
			count++
			if row.record.ID != "style-Active" {
				t.Fatal("inactive colour style visible")
			}
		}
	}
	if count != 1 {
		t.Fatalf("Scripts visible %d times", count)
	}
	if len(draft.Records["filegroups"]) != len(store.Defaults) {
		t.Fatal("hidden rules removed from draft")
	}
}

func TestFileGroupsSettingsApplyCancelAndConflict(t *testing.T) {
	store := setupFileGroupSettings(t)
	provider := fileGroupsProvider{}
	if err := f4settings.ValidateCatalog(provider.Catalog()); err != nil {
		t.Fatal(err)
	}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Records["filegroups"][0].Values["group.Name"] = "Applied"
	result := draft.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	draft.Accept(result)
	draft.Records["filegroups"][0].Values["group.Name"] = "Cancelled"
	draft.Close()
	if CommittedFileGroups().Groups()[0].Label() != "Applied" {
		t.Fatal("Cancel rolled back Apply")
	}
	transaction, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	want := store.Defaults[0].Clone()
	want.Fields["Name"] = "Applied"
	if !reflect.DeepEqual(transaction.Groups(), []filegroups.Group{want}) {
		t.Fatalf("saved snapshot differs from the applied working set: %+v", transaction.Groups())
	}
	draft, err = provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	draft.Records["filegroups"][0].Values["group.Name"] = "Draft"
	if err := os.WriteFile(store.Path, []byte("[FileGroups]\nVersion=2\n# external\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := draft.Commit(context.Background()); len(result.Errors) == 0 || !draft.Dirty("filegroups") {
		t.Fatal("external edit overwritten or draft lost")
	}
	if CommittedFileGroups().Groups()[0].Label() != "Applied" {
		t.Fatal("failed save changed runtime")
	}
}

func TestFileGroupsSettingsCommandsNestedAndPreviewSafe(t *testing.T) {
	setupFileGroupSettings(t)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	draft.Records["filegroups"] = append(draft.Records["filegroups"], f4settings.Record{ID: "user", Values: map[string]string{"group.Name": "Images", "group.Mask": "*.png", "group.Associations.Enabled": "true"}})
	draft.Records["filegroups.commands"] = []f4settings.Record{{ID: "app", Values: map[string]string{"command.Group": "user", "command.Name": "Editor", "command.Edit.Enabled": "true", "command.Edit": "edit !.!"}}}
	viewID := fileGroupActionPrefix + "Edit"
	otherGroup := f4settings.NewDraft(draft.Values, draft.Records)
	fileGroupActionCollections(otherGroup, draft.Records["filegroups"][0].ID)
	if len(otherGroup.Records[viewID]) != 0 {
		t.Fatal("commands leaked into another group")
	}
	owner := f4settings.NewDraft(draft.Values, draft.Records)
	fileGroupActionCollections(owner, "user")
	if len(owner.Records[viewID]) != 1 {
		t.Fatal("own commands missing")
	}
	groups, err := recordsToGroups(draft.Records)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := previewMatchText(groups, "example.png")
	if err != nil || preview == "" {
		t.Fatal(preview, err)
	}
	if len(CommittedFileGroups().Groups()) != 1 {
		t.Fatal("preview activated commands")
	}
	removeFileGroupCommands(draft, "user")
	if len(draft.Records["filegroups.commands"]) != 0 {
		t.Fatal("orphan commands")
	}
}

func TestFileGroupsUnknownCatalogRoundTripDoesNotInventOverrides(t *testing.T) {
	groups := []filegroups.Group{{ID: "qt.unavailable", Fields: map[string]string{"Icon.future": "asset:custom"}}}
	rows := groupsToRecords(groups)
	restored, err := recordsToGroups(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored[0].Fields) != 1 || restored[0].Commands != nil {
		t.Fatalf("unavailable factory defaults frozen: %+v", restored)
	}
}
