package main

import (
	"os"
	"path/filepath"
	"testing"
)

// useColorerCross switches the cross options for the duration of a test.
func useColorerCross(t *testing.T, mode int, crosshair bool) {
	t.Helper()

	oldMode, oldCrosshair := AppConfig.EditorCrossMode, AppConfig.EditorCrosshair
	AppConfig.EditorCrossMode = mode
	AppConfig.EditorCrosshair = crosshair

	t.Cleanup(func() {
		AppConfig.EditorCrossMode = oldMode
		AppConfig.EditorCrosshair = oldCrosshair
	})
}

func TestColorerCross_ModeAxes(t *testing.T) {
	cases := []struct {
		mode int
		horz bool
		vert bool
	}{
		{ColorerCrossOff, false, false},
		{ColorerCrossVertical, false, true},
		{ColorerCrossHorizontal, true, false},
		{ColorerCrossBoth, true, true},
	}
	for _, tc := range cases {
		horz, vert := crossModeAxes(tc.mode)
		if horz != tc.horz || vert != tc.vert {
			t.Errorf("Mode %d gave horz=%v vert=%v, expected horz=%v vert=%v", tc.mode, horz, vert, tc.horz, tc.vert)
		}
	}
}

func TestColorerCross_DisabledByTheCrosshairSwitch(t *testing.T) {
	useColorerCross(t, ColorerCrossBoth, false)

	if horz, vert, _, _ := EditorCrossAttrs(); horz || vert {
		t.Errorf("Expected no cross with the crosshair off, got horz=%v vert=%v", horz, vert)
	}
}

func TestColorerConfigsDir_HonorsTheConfiguredCatalog(t *testing.T) {
	custom := t.TempDir()
	old := AppConfig.EditorColorerCatalog
	t.Cleanup(func() { AppConfig.EditorColorerCatalog = old })

	AppConfig.EditorColorerCatalog = "  " + custom + "  "
	if got := ColorerConfigsDir(); got != custom {
		t.Errorf("Expected the configured folder %q, got %q", custom, got)
	}

	AppConfig.EditorColorerCatalog = ""
	if got := ColorerConfigsDir(); got != filepath.Join(GetF4ConfigDir(), "colorer", "configs") {
		t.Errorf("Expected the default folder, got %q", got)
	}
}

func TestColorerSchemasExist_FollowsTheConfiguredCatalog(t *testing.T) {
	custom := t.TempDir()
	old := AppConfig.EditorColorerCatalog
	AppConfig.EditorColorerCatalog = custom
	t.Cleanup(func() { AppConfig.EditorColorerCatalog = old })

	if SchemasExist() {
		t.Fatal("Expected no schemas in an empty folder")
	}
	if err := os.MkdirAll(filepath.Join(custom, "base"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(custom, "base", "catalog.xml"), []byte("<catalog/>"), 0644); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if !SchemasExist() {
		t.Error("Expected the schemas of the configured folder to be found")
	}
}

func TestConfig_ColorerSettingsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	userIniPath := filepath.Join(tmpDir, "settings.ini")

	origUserPathFunc := getUserConfigIniPath
	getUserConfigIniPath = func() string { return userIniPath }
	origPathsFunc := getConfigIniPaths
	getConfigIniPaths = func() []string { return []string{userIniPath} }

	oldCfg := AppConfig
	t.Cleanup(func() {
		getUserConfigIniPath = origUserPathFunc
		getConfigIniPaths = origPathsFunc
		AppConfig = oldCfg
	})

	catalog := filepath.Join(tmpDir, "configs")
	AppConfig.EditorColorerSyntax = false
	AppConfig.EditorColorerCatalog = catalog
	AppConfig.EditorCrossMode = ColorerCrossVertical
	SaveConfig()

	AppConfig.EditorColorerSyntax = true
	AppConfig.EditorColorerCatalog = ""
	AppConfig.EditorCrossMode = ColorerCrossBoth

	LoadConfig()

	if AppConfig.EditorColorerSyntax {
		t.Error("LoadConfig failed to restore EditorColorerSyntax")
	}
	if AppConfig.EditorColorerCatalog != catalog {
		t.Errorf("LoadConfig failed to restore EditorColorerCatalog: %q", AppConfig.EditorColorerCatalog)
	}
	if AppConfig.EditorCrossMode != ColorerCrossVertical {
		t.Errorf("LoadConfig failed to restore EditorCrossMode: %d", AppConfig.EditorCrossMode)
	}
}
