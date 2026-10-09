package app

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

type editorSettingsControlsBatch24 struct {
	dlg    vtui.Container
	combos []*vtui.ComboBox
	edits  []*vtui.Edit
	checks []*vtui.Checkbox
	ok     *vtui.Button
	cancel *vtui.Button
}

func preserveEditorConfigBatch24(t *testing.T) {
	t.Helper()
	before := config.App
	t.Cleanup(func() {
		config.App = before
		editor.SetColorerScheme(before.EditorColorerScheme)
	})
}

func openEditorSettingsBatch24(t *testing.T) editorSettingsControlsBatch24 {
	t.Helper()
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	oldUserPath := config.GetUserConfigIniPath
	oldConfigPaths := config.GetConfigIniPaths
	oldConfig := config.App
	path := filepath.Join(t.TempDir(), "settings.ini")
	config.GetUserConfigIniPath = func() string { return path }
	config.GetConfigIniPaths = func() []string { return []string{path} }
	t.Cleanup(func() {
		config.GetUserConfigIniPath = oldUserPath
		config.GetConfigIniPaths = oldConfigPaths
		config.App = oldConfig
		editor.SetColorerScheme(oldConfig.EditorColorerScheme)
	})

	actionEditorSettings(nil)
	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("editor settings dialog is not a container: %T", top)
	}
	controls := editorSettingsControlsBatch24{dlg: dlg}
	for _, child := range dlg.GetChildren() {
		switch value := child.(type) {
		case *vtui.ComboBox:
			controls.combos = append(controls.combos, value)
		case *vtui.Edit:
			controls.edits = append(controls.edits, value)
		case *vtui.Checkbox:
			controls.checks = append(controls.checks, value)
		case *vtui.Button:
			if strings.Contains(value.GetText(), i18n.Msg("vtui.Ok")) {
				controls.ok = value
			} else if strings.Contains(value.GetText(), i18n.Msg("vtui.Cancel")) {
				controls.cancel = value
			}
		}
	}
	if controls.ok == nil || controls.cancel == nil {
		t.Fatal("editor settings dialog buttons were not found")
	}
	return controls
}

func TestActionEditorSettingsSaveAllControlsBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.EditorTabSize = 4
	config.App.EditorAutoIndent = false
	c := openEditorSettingsBatch24(t)
	if len(c.checks) != 10 || len(c.combos) < 4 || len(c.edits) < 4 {
		t.Fatalf("editor settings controls = checks %d, combos %d, edits %d", len(c.checks), len(c.combos), len(c.edits))
	}
	for _, check := range c.checks {
		check.Toggle()
	}
	for _, combo := range c.combos {
		if len(combo.Menu.Items) > 1 {
			combo.Menu.SetSelectPos(1)
		}
	}
	for _, edit := range c.edits {
		if edit.GetText() == "4" {
			edit.SetText("3")
		}
	}
	c.ok.OnClick()
	if config.App.EditorTabSize != 3 || !config.App.EditorAutoIndent {
		t.Fatalf("editor settings were not saved: tab=%d autoIndent=%v", config.App.EditorTabSize, config.App.EditorAutoIndent)
	}
}

func TestActionEditorSettingsCancelBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.EditorAutoIndent = false
	before := config.App
	c := openEditorSettingsBatch24(t)
	c.checks[0].Toggle()
	c.cancel.OnClick()
	if !reflect.DeepEqual(config.App, before) {
		t.Fatal("cancel changed editor settings")
	}
}

func TestActionEditorSettingsInvalidTabSizeDefaultsBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.EditorTabSize = 6
	c := openEditorSettingsBatch24(t)
	for _, edit := range c.edits {
		if edit.GetText() == "6" {
			edit.SetText("0")
		}
	}
	c.ok.OnClick()
	if config.App.EditorTabSize != 8 {
		t.Fatalf("invalid tab size = %d, want fallback 8", config.App.EditorTabSize)
	}
}

func TestActionEditorSettingsHighlighterSelectionBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.EditorHighlighter = "Colorer"
	c := openEditorSettingsBatch24(t)
	if len(c.combos) < 2 {
		t.Fatal("highlighter combo was not found")
	}
	combo := c.combos[1]
	combo.Menu.SetSelectPos(2)
	c.ok.OnClick()
	if config.App.EditorHighlighter != "None" {
		t.Fatalf("selected highlighter = %q, want None", config.App.EditorHighlighter)
	}
}

func TestActionEditorSettingsExternalEditorLinksBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.UseExternalEditor = false
	c := openEditorSettingsBatch24(t)
	if len(c.checks) < 10 || len(c.edits) < 4 {
		t.Fatal("external editor controls were not found")
	}
	external := c.checks[len(c.checks)-1]
	consoleEdit, guiEdit := c.edits[len(c.edits)-2], c.edits[len(c.edits)-1]
	if !consoleEdit.IsDisabled() || !guiEdit.IsDisabled() {
		t.Fatal("external editor fields are unexpectedly enabled")
	}
	external.Toggle()
	if consoleEdit.IsDisabled() || guiEdit.IsDisabled() {
		t.Fatal("external editor fields stayed disabled after enabling editor")
	}
	c.cancel.OnClick()
}

func TestActionEditorSettingsInvalidExpandTabsBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.EditorExpandTabs = 99
	c := openEditorSettingsBatch24(t)
	if len(c.combos) == 0 || c.combos[0].Menu.SelectPos != 0 {
		t.Fatalf("invalid expand-tabs selection = %d, want 0", c.combos[0].Menu.SelectPos)
	}
	c.cancel.OnClick()
}

func TestActionEditorSettingsMaskRoundTripBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.EditorAutoCompleteMask = "*.go"
	c := openEditorSettingsBatch24(t)
	for _, edit := range c.edits {
		if edit.GetText() == "*.go" {
			edit.SetText("*.lua")
		}
	}
	c.ok.OnClick()
	if config.App.EditorAutoCompleteMask != "*.lua" {
		t.Fatalf("autocomplete mask = %q, want *.lua", config.App.EditorAutoCompleteMask)
	}
}

func TestActionEditorSettingsExternalCommandsRoundTripBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	config.App.ExternalEditorConsole = "old-console"
	config.App.ExternalEditorGUI = "old-gui"
	c := openEditorSettingsBatch24(t)
	for _, edit := range c.edits {
		switch edit.GetText() {
		case "old-console":
			edit.SetText("new-console")
		case "old-gui":
			edit.SetText("new-gui")
		}
	}
	c.ok.OnClick()
	if config.App.ExternalEditorConsole != "new-console" || config.App.ExternalEditorGUI != "new-gui" {
		t.Fatalf("external commands = %q / %q", config.App.ExternalEditorConsole, config.App.ExternalEditorGUI)
	}
}

func TestActionEditorSettingsCodepageSelectionBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	c := openEditorSettingsBatch24(t)
	if len(c.combos) < 4 {
		t.Fatal("codepage combo was not found")
	}
	combo := c.combos[3]
	if len(combo.Menu.Items) > 1 {
		combo.Menu.SetSelectPos(len(combo.Menu.Items) - 1)
	}
	c.ok.OnClick()
}

func TestActionEditorSettingsColorerSchemeSelectionBatch24(t *testing.T) {
	preserveEditorConfigBatch24(t)
	c := openEditorSettingsBatch24(t)
	if len(c.combos) < 3 {
		t.Fatal("colorer scheme combo was not found")
	}
	combo := c.combos[2]
	if len(combo.Menu.Items) > 1 {
		combo.Menu.SetSelectPos(len(combo.Menu.Items) - 1)
	}
	c.ok.OnClick()
	if len(combo.Menu.Items) > 1 && config.App.EditorColorerScheme == "" {
		t.Fatal("colorer scheme selection was not saved")
	}
}
