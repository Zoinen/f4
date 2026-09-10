package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"os"
	"path/filepath"
	"testing"
)

// useColorerCross switches the cross options for the duration of a test.

func TestColorerConfigsDir_HonorsTheConfiguredCatalog(t *testing.T) {
	custom := t.TempDir()
	old := config.App.EditorColorerCatalog
	t.Cleanup(func() { config.App.EditorColorerCatalog = old })

	config.App.EditorColorerCatalog = "  " + custom + "  "
	if got := editor.ColorerConfigsDir(); got != custom {
		t.Errorf("Expected the configured folder %q, got %q", custom, got)
	}

	config.App.EditorColorerCatalog = ""
	if got := editor.ColorerConfigsDir(); got != filepath.Join(config.GetF4ConfigDir(), "colorer", "configs") {
		t.Errorf("Expected the default folder, got %q", got)
	}
}

func TestColorerSchemasExist_FollowsTheConfiguredCatalog(t *testing.T) {
	custom := t.TempDir()
	old := config.App.EditorColorerCatalog
	config.App.EditorColorerCatalog = custom
	t.Cleanup(func() { config.App.EditorColorerCatalog = old })

	if editor.SchemasExist() {
		t.Fatal("Expected no schemas in an empty folder")
	}
	if err := os.MkdirAll(filepath.Join(custom, "base"), 0700); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(custom, "base", "catalog.xml"), []byte("<catalog/>"), 0600); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if !editor.SchemasExist() {
		t.Error("Expected the schemas of the configured folder to be found")
	}
}

func TestConfig_ColorerSettingsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	userIniPath := filepath.Join(tmpDir, "settings.ini")

	origUserPathFunc := config.GetUserConfigIniPath
	config.GetUserConfigIniPath = func() string { return userIniPath }
	origPathsFunc := config.GetConfigIniPaths
	config.GetConfigIniPaths = func() []string { return []string{userIniPath} }

	oldCfg := config.App
	t.Cleanup(func() {
		config.GetUserConfigIniPath = origUserPathFunc
		config.GetConfigIniPaths = origPathsFunc
		config.App = oldCfg
	})

	catalog := filepath.Join(tmpDir, "configs")
	config.App.EditorColorerSyntax = false
	config.App.EditorSyntaxAnimation = true
	config.App.EditorColorerCatalog = catalog
	config.App.EditorCrossMode = config.ColorerCrossVertical
	config.SaveConfig()

	config.App.EditorColorerSyntax = true
	config.App.EditorSyntaxAnimation = false
	config.App.EditorColorerCatalog = ""
	config.App.EditorCrossMode = config.ColorerCrossBoth

	config.LoadConfig()

	if config.App.EditorColorerSyntax {
		t.Error("config.LoadConfig failed to restore EditorColorerSyntax")
	}
	if !config.App.EditorSyntaxAnimation {
		t.Error("config.LoadConfig failed to restore EditorSyntaxAnimation")
	}
	if config.App.EditorColorerCatalog != catalog {
		t.Errorf("config.LoadConfig failed to restore EditorColorerCatalog: %q", config.App.EditorColorerCatalog)
	}
	if config.App.EditorCrossMode != config.ColorerCrossVertical {
		t.Errorf("config.LoadConfig failed to restore EditorCrossMode: %d", config.App.EditorCrossMode)
	}
}
