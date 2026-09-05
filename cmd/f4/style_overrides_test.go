package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func withOverrides(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "farcolors.ini"), []byte(body), 0600); err != nil {
			t.Fatalf("write overrides: %v", err)
		}
	}
	old := userColorOverridesPath
	userColorOverridesPath = func() string { return filepath.Join(dir, "farcolors.ini") }
	t.Cleanup(func() { userColorOverridesPath = old })

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	t.Cleanup(func() { AppConfig = oldCfg })
}

// Switching styles at runtime must land on the same palette a restart would
// produce. The user's farcolors.ini used to be applied only during startup, so
// picking a style in the Appearance dialog quietly dropped every override until
// the next launch — half the screen changed, half did not.
func TestApplyColorStyle_ReappliesUserOverrides(t *testing.T) {
	withOverrides(t, "[farcolors]\nPanel.Text = foreground:#123456 | background:#654321\n")

	for _, style := range []string{"Modern", "Default Dark", "Classic"} {
		if err := ApplyColorStyle(style); err != nil {
			t.Fatalf("ApplyColorStyle(%q): %v", style, err)
		}
		fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelText])
		if fg != 0x123456 || bg != 0x654321 {
			t.Errorf("after switching to %s, Panel.Text = #%06x on #%06x, want the override #123456 on #654321",
				style, fg, bg)
		}
	}
}

// Without an overrides file the style must come through untouched.
func TestApplyColorStyle_WithoutOverrides(t *testing.T) {
	withOverrides(t, "")

	if err := ApplyColorStyle("Default Dark"); err != nil {
		t.Fatalf("ApplyColorStyle: %v", err)
	}
	fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelText])
	if fg != 0x34E2E2 || bg != 0x2E3436 {
		t.Errorf("Panel.Text = #%06x on #%06x, want #34e2e2 on #2e3436", fg, bg)
	}
}

// Overrides must survive the contrast pass being switched on, and the pass must
// run once over the finished palette rather than after each layer.
func TestApplyColorStyle_OverridesWithContrastCorrection(t *testing.T) {
	withOverrides(t, "[farcolors]\nKeybar.Text = foreground:#2E3436 | background:#06989A\n")
	AppConfig.EnforceColorCorrection = true

	if err := ApplyColorStyle("Modern"); err != nil {
		t.Fatalf("ApplyColorStyle: %v", err)
	}
	fg, bg := GetColorRGBBoth(vtui.Palette[vtui.ColKeyBarText])
	if fg != 0x2E3436 || bg != 0x06989A {
		t.Errorf("Keybar.Text = #%06x on #%06x, want the override left as authored", fg, bg)
	}
}
