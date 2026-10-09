package app

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func settingsCoveragePanel(t *testing.T) *panel.PanelsFrame {
	t.Helper()
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	pf := panel.NewPanelsFrame()
	pf.ResizeConsole(80, 25)
	t.Cleanup(pf.Close)
	return pf
}

func isolateSettingsCoverageConfig(t *testing.T) {
	t.Helper()
	oldCfg := config.App
	oldUserPath := config.GetUserConfigIniPath
	oldPaths := config.GetConfigIniPaths
	path := filepath.Join(t.TempDir(), "settings.ini")
	config.GetUserConfigIniPath = func() string { return path }
	config.GetConfigIniPaths = func() []string { return []string{path} }
	t.Cleanup(func() {
		config.App = oldCfg
		config.GetUserConfigIniPath = oldUserPath
		config.GetConfigIniPaths = oldPaths
	})
}

func settingsCoverageCheckbox(dlg vtui.Container, caption string) *vtui.Checkbox {
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok && testutil.GetCleanText(checkbox) == caption {
			return checkbox
		}
	}
	return nil
}

func settingsCoverageCaption(key string) string {
	return testutil.GetCleanText(vtui.NewCheckbox(0, 0, i18n.Msg(key), false))
}

func TestActionEditorSettingsSavesCoreOptions(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.EditorAutoIndent = false
	config.App.EditorCursorBeyondEOL = false
	config.App.EditorUseEditorConfig = false
	config.App.EditorAutoComplete = false
	config.App.EditorCrosshair = false
	config.App.EditorMarkOccurrences = false
	config.App.EditorColorerBackground = false
	config.App.EditorSyntaxAnimation = false
	config.App.EditorAutodetectCodePage = false
	config.App.UseExternalEditor = false

	actionEditorSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 1
		}
	}
	testutil.ClickDialogButton(t, dlg, "Ok")

	if !config.App.EditorAutoIndent || !config.App.EditorCursorBeyondEOL ||
		!config.App.EditorUseEditorConfig || !config.App.EditorAutoComplete ||
		!config.App.EditorCrosshair || !config.App.EditorMarkOccurrences ||
		!config.App.EditorColorerBackground || !config.App.EditorSyntaxAnimation ||
		!config.App.EditorAutodetectCodePage || !config.App.UseExternalEditor {
		t.Fatal("editor checkbox settings were not saved")
	}
}

func TestActionEditorSettingsInvalidTabSizeDefaults(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.EditorTabSize = 4
	actionEditorSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	found := false
	for _, child := range dlg.GetChildren() {
		if edit, ok := child.(*vtui.Edit); ok {
			x1, _, x2, _ := edit.GetPosition()
			if x2-x1+1 <= 4 {
				edit.SetText("0")
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("tab-size edit was not found")
	}
	testutil.ClickDialogButton(t, dlg, "Ok")
	if config.App.EditorTabSize != 8 {
		t.Fatalf("invalid tab size should default to 8, got %d", config.App.EditorTabSize)
	}
}

func TestActionEditorSettingsCancelLeavesConfig(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.EditorAutoIndent = false
	config.App.UseExternalEditor = false
	actionEditorSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 1
		}
	}
	testutil.ClickDialogButton(t, dlg, "Cancel")
	if config.App.EditorAutoIndent || config.App.UseExternalEditor {
		t.Fatal("cancel changed editor settings")
	}
}

func TestActionEditorSettingsInvalidExpandTabsSelection(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.EditorExpandTabs = 99
	actionEditorSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	found := false
	for _, child := range dlg.GetChildren() {
		if combo, ok := child.(*vtui.ComboBox); ok && len(combo.Menu.Items) == 3 &&
			combo.Menu.Items[0].Text == i18n.Msg("EditorSettings.TabExpandNone") {
			found = true
			if combo.Menu.SelectPos != 0 {
				t.Fatalf("invalid expand-tabs value selected position %d", combo.Menu.SelectPos)
			}
		}
	}
	if !found {
		t.Fatal("expand-tabs combo was not found")
	}
	testutil.ClickDialogButton(t, dlg, "Cancel")
}

func TestActionEditorSettingsUnknownHighlighterSelection(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.EditorHighlighter = "unknown"
	actionEditorSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	found := false
	for _, child := range dlg.GetChildren() {
		if combo, ok := child.(*vtui.ComboBox); ok && len(combo.Menu.Items) == 3 && combo.Menu.Items[0].Text == "Chroma" {
			found = true
			if combo.Menu.SelectPos != 0 || combo.Edit.GetText() != "Chroma" {
				t.Fatalf("unknown highlighter was not reset to Chroma")
			}
		}
	}
	if !found {
		t.Fatal("highlighter combo was not found")
	}
	testutil.ClickDialogButton(t, dlg, "Cancel")
}

func TestActionConfirmationsSettingsSavesAll(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.ConfirmCopy = false
	config.App.ConfirmMove = false
	config.App.ConfirmDelete = false
	config.App.ConfirmExit = false
	config.App.DeleteCancelFocused = false
	actionConfirmationsSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 1
		}
	}
	testutil.ClickDialogButton(t, dlg, "Ok")
	if !config.App.ConfirmCopy || !config.App.ConfirmMove || !config.App.ConfirmDelete ||
		!config.App.ConfirmExit || !config.App.DeleteCancelFocused {
		t.Fatal("confirmation settings were not saved as enabled")
	}
}

func TestActionConfirmationsSettingsSavesNone(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.ConfirmCopy = true
	config.App.ConfirmMove = true
	config.App.ConfirmDelete = true
	config.App.ConfirmExit = true
	config.App.DeleteCancelFocused = true
	actionConfirmationsSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 0
		}
	}
	testutil.ClickDialogButton(t, dlg, "Ok")
	if config.App.ConfirmCopy || config.App.ConfirmMove || config.App.ConfirmDelete ||
		config.App.ConfirmExit || config.App.DeleteCancelFocused {
		t.Fatal("confirmation settings were not saved as disabled")
	}
}

func TestActionConfirmationsSettingsInitializesState(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.ConfirmCopy = true
	config.App.ConfirmMove = false
	config.App.ConfirmDelete = true
	config.App.ConfirmExit = false
	config.App.DeleteCancelFocused = true
	actionConfirmationsSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	want := map[string]int{
		settingsCoverageCaption("ConfirmationsSettings.Copy"):                1,
		settingsCoverageCaption("ConfirmationsSettings.Move"):                0,
		settingsCoverageCaption("ConfirmationsSettings.Delete"):              1,
		settingsCoverageCaption("ConfirmationsSettings.Exit"):                0,
		settingsCoverageCaption("ConfirmationsSettings.DeleteCancelFocused"): 1,
	}
	for caption, state := range want {
		checkbox := settingsCoverageCheckbox(dlg, caption)
		if checkbox == nil {
			t.Fatalf("confirmation checkbox %q was not found", caption)
		}
		if checkbox.State != state {
			t.Fatalf("confirmation checkbox %q has state %d, want %d", caption, checkbox.State, state)
		}
	}
	testutil.ClickDialogButton(t, dlg, "Cancel")
}

func TestActionConfirmationsSettingsCancelLeavesConfig(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	config.App.ConfirmCopy = false
	config.App.ConfirmMove = false
	config.App.ConfirmDelete = false
	config.App.ConfirmExit = false
	config.App.DeleteCancelFocused = false
	actionConfirmationsSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 1
		}
	}
	testutil.ClickDialogButton(t, dlg, "Cancel")
	if config.App.ConfirmCopy || config.App.ConfirmMove || config.App.ConfirmDelete ||
		config.App.ConfirmExit || config.App.DeleteCancelFocused {
		t.Fatal("cancel changed confirmation settings")
	}
}

func TestActionConfirmationsSettingsLayout(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	actionConfirmationsSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	vtui.AssertLayout(t, dlg)
	testutil.ClickDialogButton(t, dlg, "Cancel")
}
