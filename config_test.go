package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	userIniPath := filepath.Join(tmpDir, "settings.ini")

	origUserPathFunc := getUserConfigIniPath
	getUserConfigIniPath = func() string { return userIniPath }
	origPathsFunc := getConfigIniPaths
	getConfigIniPaths = func() []string { return []string{userIniPath} }

	// Save original config to restore after test
	oldCfg := AppConfig
	defer func() {
		getUserConfigIniPath = origUserPathFunc
		getConfigIniPaths = origPathsFunc
		AppConfig = oldCfg
	}()

	// 1. Set some non-default values
	AppConfig.ShowHiddenFiles = false
	AppConfig.ColorStyle = "Classic"
	AppConfig.HighlightDir = true
	AppConfig.SavePanelPaths = false
	AppConfig.EditorCrosshair = true
	AppConfig.CommandLineAutoComplete = false

	// 2. Save
	SaveConfig()

	// 3. Reset to defaults
	AppConfig.ShowHiddenFiles = true
	AppConfig.ColorStyle = "Modern"
	AppConfig.HighlightDir = false
	AppConfig.EditorCrosshair = false

	// 4. Load
	LoadConfig()
	if AppConfig.ColorStyle != "Classic" {
		t.Errorf("LoadConfig failed to restore color style: %q", AppConfig.ColorStyle)
	}

	// 5. Verify
	if AppConfig.ShowHiddenFiles {
		t.Error("LoadConfig failed to restore ShowHiddenFiles")
	}
	if !AppConfig.HighlightDir {
		t.Error("LoadConfig failed to restore HighlightDir")
	}
	if AppConfig.SavePanelPaths {
		t.Error("LoadConfig failed to restore SavePanelPaths")
	}
	if !AppConfig.EditorCrosshair {
		t.Error("LoadConfig failed to restore EditorCrosshair")
	}
	if AppConfig.CommandLineAutoComplete {
		t.Error("LoadConfig failed to restore CommandLineAutoComplete")
	}
}

func TestConfig_Merge(t *testing.T) {
	tmpDir := t.TempDir()
	globalPath := filepath.Join(tmpDir, "global.ini")
	userPath := filepath.Join(tmpDir, "user.ini")

	globalContent := `
[Panel]
ShowHiddenFiles = 0
[Editor]
TabSize = 8
`
	userContent := `
[Panel]
ShowHiddenFiles = 1
[Editor]
Crosshair = 1
`
	os.WriteFile(globalPath, []byte(globalContent), 0644)
	os.WriteFile(userPath, []byte(userContent), 0644)

	// Mock paths
	origPathsFunc := getConfigIniPaths
	getConfigIniPaths = func() []string { return []string{globalPath, userPath} }
	defer func() { getConfigIniPaths = origPathsFunc }()

	// Save original config to restore after test
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()

	// Reset config to defaults before loading
	AppConfig = F4Config{
		ShowHiddenFiles:         true,
		HighlightDir:            true,
		EditorTabSize:           4,
		EditorCrosshair:         false,
		CommandLineAutoComplete: true, // A default that shouldn't be touched
	}

	LoadConfig()

	if !AppConfig.ShowHiddenFiles {
		t.Error("User config (ShowHiddenFiles=1) should override global (0)")
	}
	if AppConfig.EditorTabSize != 8 {
		t.Errorf("Global config (TabSize=8) was not loaded. Got %d", AppConfig.EditorTabSize)
	}
	if !AppConfig.EditorCrosshair {
		t.Error("User config (Crosshair=1) was not loaded.")
	}
	if !AppConfig.HighlightDir {
		t.Error("Default value (HighlightDir=true) was incorrectly overwritten.")
	}
}
