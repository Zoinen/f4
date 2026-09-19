package settings

import (
	"context"
	"encoding/json"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func userMenuSettingsTestDraft(t *testing.T, commands []string) (*panel.MenuSettingsSource, *settingsCenter, *settingsSession) {
	t.Helper()
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	state := &panel.MenuSettingsSource{Mode: panel.MenuModeLocal, SourcePath: filepath.Join(t.TempDir(), panel.FarMenuFileName), RootTitle: "Local menu", RootItems: []panel.UserMenuItem{{HotKey: "1", Label: "old label", Commands: commands}}}
	state.Saved = func(items []panel.UserMenuItem) { state.RootItems = items }
	OpenUserMenu(*state, vtui.NewVMenu("dummy"), 0, false, false)
	center, ok := vtui.FrameManager.GetTopFrame().(*settingsCenter)
	if !ok {
		t.Fatalf("expected Settings Center, got %T", vtui.FrameManager.GetTopFrame())
	}
	t.Cleanup(center.Close)
	for _, session := range center.sessions {
		if _, ok := session.draft.Records["usermenu.local"]; ok {
			return state, center, session
		}
	}
	t.Fatal("scoped menu draft missing")
	return nil, nil, nil
}
func TestUserMenu_InteractiveEdit(t *testing.T) {
	state, center, session := userMenuSettingsTestDraft(t, []string{"echo 1"})
	if _, err := os.Stat(state.SourcePath); !os.IsNotExist(err) {
		t.Fatal("opening Settings wrote the menu")
	}
	var edit *vtui.Edit
	for _, r := range center.page.rows {
		if r.field.ID == "menu.local.Label" {
			if input, ok := r.control.(*settingsEdit); ok {
				edit = input.Edit
			}
		}
	}
	if edit == nil || edit.GetText() != "old label" {
		t.Fatal("selected inline label missing")
	}
	edit.SetText("new label")
	edit.OnTextChange("new label")
	if result := session.draft.Commit(context.Background()); len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	if state.RootItems[0].Label != "new label" {
		t.Fatal("successful Apply did not update menu state")
	}
	loaded, err := panel.LoadFarMenuFile(state.SourcePath)
	if err != nil || loaded[0].Label != "new label" {
		t.Fatal("Apply did not persist to captured source")
	}
}
func TestUserMenu_EditItemMultilineCommand(t *testing.T) {
	state, center, session := userMenuSettingsTestDraft(t, []string{"go build ./...", "go vet ./..."})
	var edit *vtui.MultiLineEdit
	for _, r := range center.page.rows {
		if r.field.ID == "menu.local.Commands" {
			edit, _ = r.control.(*vtui.MultiLineEdit)
		}
	}
	if edit == nil || len(edit.GetLines()) != 2 {
		t.Fatal("inline multiline editor missing")
	}
	value := "go build ./...\ngo vet ./...\ngo test ./..."
	edit.SetLines(strings.Split(value, "\n"))
	edit.OnTextChange(value)
	if result := session.draft.Commit(context.Background()); len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	if strings.Join(state.RootItems[0].Commands, "\n") != value {
		t.Fatal("multiline commands were not preserved")
	}
}
func TestUserMenu_EditItemStripsBlankLines(t *testing.T) {
	state, _, session := userMenuSettingsTestDraft(t, []string{"echo a"})
	session.draft.Records["usermenu.local"][0].Values["menu.local.Commands"] = "\n\necho a\n\necho b\n\n"
	if result := session.draft.Commit(context.Background()); len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	if got := strings.Join(state.RootItems[0].Commands, "\n"); got != "echo a\n\necho b" {
		t.Fatalf("command lines = %q", got)
	}
}

func TestUserMenuMultilineSemanticCommand(t *testing.T) {
	_, center, session := userMenuSettingsTestDraft(t, []string{"echo first", "echo second"})
	center.SetPosition(0, 0, 99, 49)
	var node map[string]any
	for _, candidate := range settingsSemanticNodes(center.SemanticNode(&vtui.SemanticContext{Width: 100, Height: 50})) {
		if candidate["kind"] == "multiLineEdit" {
			node = candidate
			break
		}
	}
	if node == nil || node["text"] != "echo first\necho second" || node["fillWidth"] != true {
		t.Fatalf("missing full-width multiline command: %#v", node)
	}
	if !center.HandleSemanticAction(map[string]any{"target": node["id"], "action": "control.select", "anchor": 0, "cursor": 22}) {
		t.Fatal("selection not routed")
	}
	if !center.HandleSemanticAction(map[string]any{"target": node["id"], "action": "control.insertText", "text": "one\ntwo"}) {
		t.Fatal("edit not routed")
	}
	if got := session.draft.Records["usermenu.local"][0].Values["menu.local.Commands"]; got != "one\ntwo" {
		t.Fatalf("draft = %v", got)
	}
}

func TestFar3MenuImportStagesUntilApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main_menu.ini")
	provider := coreRecordSettingsProvider{stores: []settingsRecordStore{newUserMenuSettingsStore(settingsMenuSource{"main", "Global user menu", path, panel.MenuModeMain})}}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	items := []panel.UserMenuItem{{HotKey: "a", Label: "Imported", Commands: []string{"echo !.!"}}}
	if err := stageFar3UserMenu(draft, items); err != nil {
		t.Fatal(err)
	}
	if !draft.Dirty("usermenu.main") {
		t.Fatal("import did not dirty draft")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("import saved before Apply")
	}
	if err := stageFar3UserMenu(draft, items); err != nil {
		t.Fatal(err)
	}
	if len(draft.Records["usermenu.main"]) != 1 {
		t.Fatal("duplicate import")
	}
	result := draft.CommitFunc(context.Background(), draft)
	if len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	saved, err := panel.LoadMainMenu(path)
	if err != nil || len(saved) != 1 || saved[0].Commands[0] != "echo !.!" {
		t.Fatalf("save: %#v %v", saved, err)
	}
}

func TestUserMenuEditorContainsOnlySelectedRecord(t *testing.T) {
	_, center, _ := userMenuSettingsTestDraft(t, []string{"echo selected"})
	if !center.recordOnly || center.sidebar.IsVisible() || center.search.IsVisible() {
		t.Fatal("F4 opened global settings instead of a record dialog")
	}
	center.SetPosition(0, 0, 99, 49)
	if path := os.Getenv("F4_USERMENU_EDITOR_SCENE"); path != "" {
		data, err := json.Marshal(center.SemanticNode(&vtui.SemanticContext{Width: 100, Height: 50}))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if len(center.sessions) != 1 || len(center.sessions[0].catalog.Collections) != 1 {
		t.Fatal("unrelated settings were opened")
	}
	for _, row := range center.page.rows {
		if row.control != nil && strings.HasPrefix(row.control.GetId(), "collection:") {
			t.Fatal("record list is visible in the single-item editor")
		}
	}
}

func TestUserMenuRecordEditorPreservesSiblingsAndCancel(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	path := filepath.Join(t.TempDir(), "FarMenu.ini")
	items := []panel.UserMenuItem{
		{HotKey: "s", Label: "Submenu", Submenu: []panel.UserMenuItem{
			{HotKey: "a", Label: "First", Commands: []string{"echo first"}},
			{HotKey: "--"},
			{HotKey: "b", Label: "Second", Commands: []string{"echo second"}},
		}},
		{HotKey: "z", Label: "Sibling", Commands: []string{"echo sibling"}},
	}
	state := panel.MenuSettingsSource{Mode: panel.MenuModeLocal, SourcePath: path, RootTitle: "Menu", RootItems: items, Path: []int{0}}
	state.Saved = func(saved []panel.UserMenuItem) { state.RootItems = saved }
	for _, save := range []bool{false, true} {
		if !OpenUserMenu(state, nil, 2, false, false) {
			t.Fatal("not opened")
		}
		c := vtui.FrameManager.GetTopFrame().(*settingsCenter)
		found := false
		for _, row := range c.page.rows {
			if row.field.ID == "menu.local.Label" {
				edit := row.control.(*settingsEdit)
				if edit.GetText() != "Second" {
					t.Fatal("wrong nested item selected")
				}
				edit.OnTextChange("Changed")
				found = true
			}
		}
		if !found {
			t.Fatal("editor field absent")
		}
		if save {
			if result := c.sessions[0].draft.Commit(context.Background()); len(result.Errors) > 0 {
				t.Fatal(result.Errors)
			}
			c.Close()
			saved, err := panel.LoadFarMenuFile(path)
			if err != nil || len(saved) != 2 || len(saved[0].Submenu) != 3 || !saved[0].Submenu[1].IsSeparator() || saved[0].Submenu[0].Label != "First" || saved[0].Submenu[2].Label != "Changed" || saved[1].Label != "Sibling" {
				t.Fatalf("sibling mutation: %#v %v", saved, err)
			}
		} else {
			c.Close()
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("Cancel wrote menu")
			}
			if state.RootItems[0].Submenu[2].Label != "Second" {
				t.Fatal("Cancel changed live menu")
			}
		}
	}
}
