package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// useColorerCross switches the cross options for the duration of a test.
func useColorerCross(t *testing.T, mode int, crosshair bool) {
	t.Helper()

	oldMode, oldCrosshair := config.App.EditorCrossMode, config.App.EditorCrosshair
	config.App.EditorCrossMode = mode
	config.App.EditorCrosshair = crosshair

	t.Cleanup(func() {
		config.App.EditorCrossMode = oldMode
		config.App.EditorCrosshair = oldCrosshair
	})
}

func TestColorerCrossModeItems_FollowTheModeOrder(t *testing.T) {
	got := colorerCrossModeItems()
	want := []string{
		i18n.Msg("ColorerSettings.CrossOff"),
		i18n.Msg("ColorerSettings.CrossVertical"),
		i18n.Msg("ColorerSettings.CrossHorizontal"),
		i18n.Msg("ColorerSettings.CrossBoth"),
		i18n.Msg("ColorerSettings.CrossScheme"),
	}
	if len(got) != len(want) {
		t.Fatalf("colorerCrossModeItems() has %d items; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("colorerCrossModeItems()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestColorerIsActive_IsCaseInsensitive(t *testing.T) {
	old := config.App.EditorHighlighter
	t.Cleanup(func() { config.App.EditorHighlighter = old })

	for _, tc := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "exact", value: "Colorer", want: true},
		{name: "lowercase", value: "colorer", want: true},
		{name: "other highlighter", value: "Chroma", want: false},
		{name: "empty", value: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.App.EditorHighlighter = tc.value
			if got := colorerIsActive(); got != tc.want {
				t.Fatalf("colorerIsActive(%q) = %v; want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestEditorCrossAttrs_MapsEnabledModesAndFallsBackToPalette(t *testing.T) {
	oldHighlighter := config.App.EditorHighlighter
	t.Cleanup(func() { config.App.EditorHighlighter = oldHighlighter })
	config.App.EditorHighlighter = "None"

	base := vtui.Palette[theme.ColEditorCrosshair]
	for _, tc := range []struct {
		name     string
		mode     int
		wantHorz bool
		wantVert bool
	}{
		{name: "off", mode: config.ColorerCrossOff},
		{name: "vertical", mode: config.ColorerCrossVertical, wantVert: true},
		{name: "horizontal", mode: config.ColorerCrossHorizontal, wantHorz: true},
		{name: "both", mode: config.ColorerCrossBoth, wantHorz: true, wantVert: true},
		{name: "invalid", mode: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useColorerCross(t, tc.mode, true)
			horz, vert, horzAttr, vertAttr := EditorCrossAttrs()
			if horz != tc.wantHorz || vert != tc.wantVert {
				t.Fatalf("EditorCrossAttrs() axes = %v, %v; want %v, %v", horz, vert, tc.wantHorz, tc.wantVert)
			}
			wantAttr := uint64(0)
			if tc.wantHorz || tc.wantVert {
				wantAttr = base
			}
			if horzAttr != wantAttr || vertAttr != wantAttr {
				t.Fatalf("EditorCrossAttrs() attrs = %#x, %#x; want %#x, %#x", horzAttr, vertAttr, wantAttr, wantAttr)
			}
		})
	}
}

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
