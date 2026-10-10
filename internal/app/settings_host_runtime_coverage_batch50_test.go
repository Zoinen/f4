package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func prepareSettingsHostRuntimeCoverageBatch50(t *testing.T) *vtui.FrameManagerType {
	t.Helper()
	oldApp := config.App
	oldFrameManager := vtui.FrameManager
	fm := vtui.NewFrameManager()
	vtui.FrameManager = fm
	t.Cleanup(func() {
		fm.Shutdown()
		config.App = oldApp
		vtui.FrameManager = oldFrameManager
	})
	return fm
}

func TestSettingsHostApplyRuntimeSetsDirectWorkspaceTabModeCoverageBatch50(t *testing.T) {
	fm := prepareSettingsHostRuntimeCoverageBatch50(t)
	config.App.CtrlTabShowsMenu = false
	config.App.WorkspaceTabMode = int(vtui.WorkspaceTabsAlways)
	config.App.WorkspaceTabsOverlay = false
	config.App.AltNumberSwitchesTabs = false
	(settingsHost{}).ApplyRuntime(config.App, nil)
	if fm.WorkspaceCtrlTabMode != vtui.WorkspaceCtrlTabDirect || fm.WorkspaceTabMode != vtui.WorkspaceTabsAlways {
		t.Fatalf("workspace modes = (%v,%v), want direct/always", fm.WorkspaceCtrlTabMode, fm.WorkspaceTabMode)
	}
}

func TestSettingsHostApplyRuntimeSetsMenuWorkspaceTabModeCoverageBatch50(t *testing.T) {
	fm := prepareSettingsHostRuntimeCoverageBatch50(t)
	config.App.CtrlTabShowsMenu = true
	(settingsHost{}).ApplyRuntime(config.App, nil)
	if fm.WorkspaceCtrlTabMode != vtui.WorkspaceCtrlTabMenu {
		t.Fatalf("CtrlTab mode = %v, want menu", fm.WorkspaceCtrlTabMode)
	}
}

func TestSettingsHostApplyRuntimeSetsWorkspaceOverlayCoverageBatch50(t *testing.T) {
	fm := prepareSettingsHostRuntimeCoverageBatch50(t)
	config.App.WorkspaceTabsOverlay = true
	(settingsHost{}).ApplyRuntime(config.App, nil)
	if !fm.WorkspaceTabOverlay {
		t.Fatal("workspace tab overlay was not applied")
	}
}

func TestSettingsHostApplyRuntimeSetsAltNumberSwitchCoverageBatch50(t *testing.T) {
	fm := prepareSettingsHostRuntimeCoverageBatch50(t)
	config.App.AltNumberSwitchesTabs = true
	(settingsHost{}).ApplyRuntime(config.App, nil)
	if !fm.WorkspaceAltNumberSwitch {
		t.Fatal("Alt-number workspace switching was not applied")
	}
}

func TestSettingsHostApplyRuntimeHandlesMenuBarChangeWithoutFramesCoverageBatch50(t *testing.T) {
	prepareSettingsHostRuntimeCoverageBatch50(t)
	before := config.App
	config.App.AlwaysShowMenuBar = !before.AlwaysShowMenuBar
	(settingsHost{}).ApplyRuntime(before, nil)
}

func TestSettingsHostApplyRuntimeHandlesNavigationChangeWithoutPanelsCoverageBatch50(t *testing.T) {
	prepareSettingsHostRuntimeCoverageBatch50(t)
	before := config.App
	config.App.NavigationMode = before.NavigationMode + 1
	(settingsHost{}).ApplyRuntime(before, nil)
}

func TestSettingsHostApplyRuntimeHandlesWorkspaceNumberingChangeWithoutScreensCoverageBatch50(t *testing.T) {
	prepareSettingsHostRuntimeCoverageBatch50(t)
	before := config.App
	config.App.WorkspaceTabNumbering = config.WorkspaceTabNumbersOrder
	if before.WorkspaceTabNumbering == config.App.WorkspaceTabNumbering {
		before.WorkspaceTabNumbering = config.WorkspaceTabNumbersSession
	}
	(settingsHost{}).ApplyRuntime(before, nil)
}

func TestSettingsHostApplyRuntimeHandlesColorStyleChangeCoverageBatch50(t *testing.T) {
	prepareSettingsHostRuntimeCoverageBatch50(t)
	before := config.App
	config.App.ColorStyle = "missing-style"
	(settingsHost{}).ApplyRuntime(before, []string{"ColorStyle"})
}

func TestSettingsHostApplyRuntimeHandlesEditorColorerChangeCoverageBatch50(t *testing.T) {
	prepareSettingsHostRuntimeCoverageBatch50(t)
	(settingsHost{}).ApplyRuntime(config.App, []string{"EditorColorerScheme"})
}

func TestSettingsHostApplyRuntimeHandlesLanguageChangeCoverageBatch50(t *testing.T) {
	prepareSettingsHostRuntimeCoverageBatch50(t)
	before := config.App
	config.App.Language = before.Language + "-test"
	config.App.HelpLanguage = before.HelpLanguage + "-test"
	(settingsHost{}).ApplyRuntime(before, nil)
}
