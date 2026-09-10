package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_AutoSaveCategoriesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	origUserPathFunc := config.GetUserConfigIniPath
	origPathsFunc := config.GetConfigIniPaths
	oldConfig := config.App
	defer func() {
		config.GetUserConfigIniPath = origUserPathFunc
		config.GetConfigIniPaths = origPathsFunc
		config.App = oldConfig
	}()
	config.GetUserConfigIniPath = func() string { return path }
	config.GetConfigIniPaths = func() []string { return []string{path} }

	config.App.AutoSaveSettings = true
	config.App.AutoSaveDialogSettings = true
	config.App.AutoSavePanelSettings = false
	config.App.AutoSaveCurrentPanel = true
	config.App.AutoSaveGUIWindow = false
	config.SaveConfig()

	config.App.AutoSaveSettings = false
	config.App.AutoSaveDialogSettings = false
	config.App.AutoSavePanelSettings = true
	config.App.AutoSaveCurrentPanel = false
	config.App.AutoSaveGUIWindow = true
	config.LoadConfig()

	if !config.App.AutoSaveSettings || !config.App.AutoSaveDialogSettings || config.App.AutoSavePanelSettings ||
		!config.App.AutoSaveCurrentPanel || config.App.AutoSaveGUIWindow {
		t.Fatalf("autosave categories did not round-trip: master=%v dialog=%v panel=%v current=%v gui=%v",
			config.App.AutoSaveSettings, config.App.AutoSaveDialogSettings, config.App.AutoSavePanelSettings,
			config.App.AutoSaveCurrentPanel, config.App.AutoSaveGUIWindow)
	}
}

func TestConfig_AutoSaveCategoriesMigrateLegacyMaster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(path, []byte("[System]\nAutoSaveSettings = 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	origUserPathFunc := config.GetUserConfigIniPath
	origPathsFunc := config.GetConfigIniPaths
	oldConfig := config.App
	defer func() {
		config.GetUserConfigIniPath = origUserPathFunc
		config.GetConfigIniPaths = origPathsFunc
		config.App = oldConfig
	}()
	config.GetUserConfigIniPath = func() string { return path }
	config.GetConfigIniPaths = func() []string { return []string{path} }

	config.LoadConfig()
	if config.App.AutoSaveSettings || config.App.AutoSaveDialogSettings || config.App.AutoSavePanelSettings ||
		config.App.AutoSaveCurrentPanel || config.App.AutoSaveGUIWindow {
		t.Fatalf("legacy disabled autosave did not disable all categories: master=%v dialog=%v panel=%v current=%v gui=%v",
			config.App.AutoSaveSettings, config.App.AutoSaveDialogSettings, config.App.AutoSavePanelSettings,
			config.App.AutoSaveCurrentPanel, config.App.AutoSaveGUIWindow)
	}
}

func TestSaveSession_DisabledWhenAllCategoriesAreOff(t *testing.T) {
	oldConfig := config.App
	oldSessionPath := GetSessionIniPath
	oldLoaded := sessionLoaded
	defer func() {
		config.App = oldConfig
		GetSessionIniPath = oldSessionPath
		sessionLoaded = oldLoaded
	}()

	path := filepath.Join(t.TempDir(), "session.ini")
	GetSessionIniPath = func() string { return path }
	// Otherwise the assertion passes for the wrong reason: unloaded state.
	sessionLoaded = true
	config.App.AutoSaveSettings = true
	config.App.AutoSaveDialogSettings = false
	config.App.AutoSavePanelSettings = false
	config.App.AutoSaveCurrentPanel = false
	config.App.AutoSaveGUIWindow = false
	SaveSession()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("all-disabled automatic session save created %s, err=%v", path, err)
	}
}

// The session client, --version and --help reach SaveSession in main's defer
// without ever calling LoadSession: their defaults would cost the daemon's file
// its panel paths and wide mode.
func TestSaveSession_SkippedWhenStateWasNeverLoaded(t *testing.T) {
	oldConfig := config.App
	oldSessionPath := GetSessionIniPath
	oldLoaded := sessionLoaded
	defer func() {
		config.App = oldConfig
		GetSessionIniPath = oldSessionPath
		sessionLoaded = oldLoaded
	}()

	path := filepath.Join(t.TempDir(), "session.ini")
	GetSessionIniPath = func() string { return path }
	sessionLoaded = false
	config.App.AutoSaveSettings = true
	config.App.AutoSaveDialogSettings = true
	config.App.AutoSavePanelSettings = true
	config.App.AutoSaveCurrentPanel = true
	config.App.AutoSaveGUIWindow = true
	SaveSession()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("session save without a loaded state created %s, err=%v", path, err)
	}
}

func TestMergeWorkspaceSessionSaveGranularPolicies(t *testing.T) {
	previous := []panel.WorkspaceSessionState{{
		Number: 4, ActivePanel: 0, WidePanel: 1, ShowPanels: true,
		Left:  panel.PanelSessionState{Path: "old-left", Cursor: "old-left.txt", ViewMode: int(panel.ViewModeBrief), SortMode: int(panel.SortName)},
		Right: panel.PanelSessionState{Path: "old-right", Cursor: "old-right.txt", ViewMode: int(panel.ViewModeDetailed), SortMode: int(panel.SortTime)},
	}}
	current := []panel.WorkspaceSessionState{{
		Number: 4, ActivePanel: 1, WidePanel: -1, ShowPanels: false,
		Left:  panel.PanelSessionState{Path: "new-left", Cursor: "new-left.txt", ViewMode: int(panel.ViewModeMedium), SortMode: int(panel.SortSize)},
		Right: panel.PanelSessionState{Path: "new-right", Cursor: "new-right.txt", ViewMode: int(panel.ViewModeBrief), SortMode: int(panel.SortExt)},
	}}

	pathsOnly, active := mergeWorkspaceSessionSave(previous, 0, current, 1, false, true)
	if active != 0 || pathsOnly[0].ShowPanels != previous[0].ShowPanels ||
		pathsOnly[0].Left.ViewMode != previous[0].Left.ViewMode || pathsOnly[0].Left.Path != current[0].Left.Path ||
		pathsOnly[0].Right.Cursor != current[0].Right.Cursor {
		t.Fatalf("current-panel-only merge changed the wrong fields: active=%d state=%#v", active, pathsOnly[0])
	}

	settingsOnly, active := mergeWorkspaceSessionSave(previous, 0, current, 1, true, false)
	if active != 1 || settingsOnly[0].ShowPanels != current[0].ShowPanels ||
		settingsOnly[0].Left.SortMode != current[0].Left.SortMode || settingsOnly[0].Left.Path != previous[0].Left.Path ||
		settingsOnly[0].Right.Cursor != previous[0].Right.Cursor {
		t.Fatalf("panel-settings-only merge changed the wrong fields: active=%d state=%#v", active, settingsOnly[0])
	}
}
