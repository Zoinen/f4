package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func actionsCoverageBatch17ButtonCaption(key string) string {
	return testutil.GetCleanText(vtui.NewButton(0, 0, i18n.Msg(key)))
}

func actionsCoverageBatch17FindFileState(t *testing.T) {
	t.Helper()
	oldMask, oldText := LastFindFileMask, LastFindFileText
	oldCase, oldWhole := LastFindFileCaseSensitive, LastFindFileWholeWords
	oldRegexp, oldNotContaining := LastFindFileRegexp, LastFindFileNotContaining
	oldFolders, oldSymlinks := LastFindFileFolders, LastFindFileSymlinks
	t.Cleanup(func() {
		LastFindFileMask, LastFindFileText = oldMask, oldText
		LastFindFileCaseSensitive, LastFindFileWholeWords = oldCase, oldWhole
		LastFindFileRegexp, LastFindFileNotContaining = oldRegexp, oldNotContaining
		LastFindFileFolders, LastFindFileSymlinks = oldFolders, oldSymlinks
	})
}

func actionsCoverageBatch17PluginList(dlg vtui.Container) *vtui.ListBox {
	for _, child := range dlg.GetChildren() {
		if list, ok := child.(*vtui.ListBox); ok {
			return list
		}
	}
	return nil
}

func TestActionFindFileInitializesAllOptions(t *testing.T) {
	actionsCoverageBatch17FindFileState(t)
	pf := settingsCoveragePanel(t)
	LastFindFileMask = "*.go"
	LastFindFileText = "needle"
	LastFindFileCaseSensitive = true
	LastFindFileWholeWords = false
	LastFindFileRegexp = true
	LastFindFileNotContaining = true
	LastFindFileFolders = false
	LastFindFileSymlinks = true

	actionFindFile(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	want := map[string]int{
		settingsCoverageCaption("FindFile.CaseSensitive"): 1,
		settingsCoverageCaption("FindFile.WholeWords"):    0,
		settingsCoverageCaption("FindFile.Regexp"):        1,
		settingsCoverageCaption("FindFile.NotContaining"): 1,
		settingsCoverageCaption("FindFile.Folders"):       0,
		settingsCoverageCaption("FindFile.Symlinks"):      1,
	}
	for caption, state := range want {
		checkbox := settingsCoverageCheckbox(dlg, caption)
		if checkbox == nil {
			t.Fatalf("find-file checkbox %q was not found", caption)
		}
		if checkbox.State != state {
			t.Fatalf("find-file checkbox %q has state %d, want %d", caption, checkbox.State, state)
		}
	}
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("vtui.Cancel"))
}

func TestActionFindFileEmptyMaskPersistsOptions(t *testing.T) {
	actionsCoverageBatch17FindFileState(t)
	pf := settingsCoveragePanel(t)
	LastFindFileMask = ""
	LastFindFileText = "before"
	actionFindFile(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	var edits []*vtui.Edit
	for _, child := range dlg.GetChildren() {
		if edit, ok := child.(*vtui.Edit); ok {
			edits = append(edits, edit)
		}
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 1
		}
	}
	if len(edits) != 2 {
		t.Fatalf("expected mask and text edits, got %d", len(edits))
	}
	edits[0].SetText("")
	edits[1].SetText("after")
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("FindFile.BtnFind"))

	if LastFindFileMask != "" || LastFindFileText != "after" ||
		!LastFindFileCaseSensitive || !LastFindFileWholeWords || !LastFindFileRegexp ||
		!LastFindFileNotContaining || !LastFindFileFolders || !LastFindFileSymlinks {
		t.Fatal("empty-mask Find File did not persist all options")
	}
}

func TestActionFindFileLayout(t *testing.T) {
	actionsCoverageBatch17FindFileState(t)
	pf := settingsCoveragePanel(t)
	actionFindFile(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	vtui.AssertLayout(t, dlg)
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("vtui.Cancel"))
}

func TestActionSaveSettingsInitializesAllGroups(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	actionSaveSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	count := 0
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			count++
			if checkbox.State != 1 {
				t.Fatalf("save-settings checkbox %d is not enabled by default", count)
			}
		}
	}
	if count != 3 {
		t.Fatalf("expected three save-settings groups, got %d", count)
	}
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("vtui.Cancel"))
}

func TestActionSaveSettingsCancel(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	actionSaveSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	for _, child := range dlg.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok {
			checkbox.State = 0
		}
	}
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("vtui.Cancel"))
}

func TestActionSaveSettingsSavesAllGroups(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	pf := settingsCoveragePanel(t)
	actionSaveSettings(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("SaveSettings.Save"))
}

func TestActionManagePluginsEmptyLayout(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	config.App.RegisteredPlugins = nil
	pf := settingsCoveragePanel(t)
	actionManagePlugins(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	list := actionsCoverageBatch17PluginList(dlg)
	if list == nil || len(list.Items) != 0 {
		t.Fatalf("expected an empty plugin list, got %#v", list)
	}
	vtui.AssertLayout(t, dlg)
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("Plugins.BtnClose"))
}

func TestActionManagePluginsClose(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	config.App.RegisteredPlugins = []string{"/tmp/plugin.so"}
	pf := settingsCoveragePanel(t)
	actionManagePlugins(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("Plugins.BtnClose"))
}

func TestActionManagePluginsDeleteCancel(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	config.App.RegisteredPlugins = []string{"/tmp/plugin.so"}
	pf := settingsCoveragePanel(t)
	actionManagePlugins(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	list := actionsCoverageBatch17PluginList(dlg)
	if list == nil {
		t.Fatal("plugin list was not created")
	}
	list.SelectPos = 0
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("Plugins.BtnRemove"))
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm.OnResult == nil {
		t.Fatal("remove confirmation dialog was not opened")
	}
	confirm.OnResult(1)
	if len(config.App.RegisteredPlugins) != 1 {
		t.Fatal("cancelling plugin removal changed the configured list")
	}
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("Plugins.BtnClose"))
}

func TestActionManagePluginsDeleteWithNoSelection(t *testing.T) {
	isolateSettingsCoverageConfig(t)
	config.App.RegisteredPlugins = []string{"/tmp/plugin.so"}
	pf := settingsCoveragePanel(t)
	actionManagePlugins(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	list := actionsCoverageBatch17PluginList(dlg)
	if list == nil {
		t.Fatal("plugin list was not created")
	}
	list.SelectPos = -1
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("Plugins.BtnRemove"))
	testutil.ClickDialogButton(t, dlg, actionsCoverageBatch17ButtonCaption("Plugins.BtnClose"))
}
