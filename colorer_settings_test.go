package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
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

func TestColorerCross_ColorsComeFromTheStyle(t *testing.T) {
	SetDefaultF4Palette()
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:horzcross": {fore: 0x111111, back: 0x222222, hasFore: true, hasBack: true},
		"def:vertcross": {fore: 0x333333, back: 0x444444, hasFore: true, hasBack: true},
	})
	useColorerHighlighter(t, false)
	useColorerCross(t, ColorerCrossBoth, true)

	horz, vert, horzAttr, vertAttr := EditorCrossAttrs()
	if !horz || !vert {
		t.Fatalf("Expected both cross lines, got horz=%v vert=%v", horz, vert)
	}
	if got := vtui.GetRGBBack(horzAttr); got != 0x222222 {
		t.Errorf("Expected the horizontal cross background 222222, got %06X", got)
	}
	if got := vtui.GetRGBFore(vertAttr); got != 0x333333 {
		t.Errorf("Expected the vertical cross foreground 333333, got %06X", got)
	}
}

func TestColorerCross_FallsBackToThePalette(t *testing.T) {
	SetDefaultF4Palette()
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:text": {fore: 0x00FF00, hasFore: true},
	})
	useColorerHighlighter(t, false)
	useColorerCross(t, ColorerCrossHorizontal, true)

	horz, vert, horzAttr, _ := EditorCrossAttrs()
	if !horz || vert {
		t.Fatalf("Expected the horizontal cross only, got horz=%v vert=%v", horz, vert)
	}
	if horzAttr != vtui.Palette[ColEditorCrosshair] {
		t.Errorf("Expected the f4 palette color, got %016X", horzAttr)
	}
}

func TestColorerCross_DisabledByTheCrosshairSwitch(t *testing.T) {
	useColorerCross(t, ColorerCrossBoth, false)

	if horz, vert, _, _ := EditorCrossAttrs(); horz || vert {
		t.Errorf("Expected no cross with the crosshair off, got horz=%v vert=%v", horz, vert)
	}
}

func TestColorerCross_StyleIgnoredWithoutColorer(t *testing.T) {
	SetDefaultF4Palette()
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:horzcross": {back: 0x222222, hasBack: true},
	})
	oldHighlighter := AppConfig.EditorHighlighter
	AppConfig.EditorHighlighter = "Chroma"
	t.Cleanup(func() { AppConfig.EditorHighlighter = oldHighlighter })
	useColorerCross(t, ColorerCrossBoth, true)

	_, _, horzAttr, _ := EditorCrossAttrs()
	if horzAttr != vtui.Palette[ColEditorCrosshair] {
		t.Errorf("Expected Chroma to keep the f4 palette color, got %016X", horzAttr)
	}
}

func TestColorer_SyntaxCanBeSwitchedOff(t *testing.T) {
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment": {fore: 0x123456, hasFore: true},
	})
	old := AppConfig.EditorColorerSyntax
	AppConfig.EditorColorerSyntax = false
	t.Cleanup(func() { AppConfig.EditorColorerSyntax = old })

	base := vtui.SetRGBBoth(0, 0xD3D7CF, 0x000000)
	if got := getColorerAttr("def:Comment", base); got != base {
		t.Errorf("Expected the base attribute with syntax off, got %016X", got)
	}
}

func TestColorer_ResetSchemeForcesAReload(t *testing.T) {
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:text": {fore: 0x00FF00, hasFore: true},
	})

	before := ColorerSchemeGeneration()
	ResetColorerScheme()
	if after := ColorerSchemeGeneration(); after == before {
		t.Errorf("Expected the generation to change, it stayed at %d", after)
	}
	if _, ok := colorerSchemeExactStyle("def:Text"); ok {
		t.Error("Expected the styles to be dropped by the reset")
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
