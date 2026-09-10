package settings

import (
	"context"
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
