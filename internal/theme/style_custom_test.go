package theme

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
	"path/filepath"
	"testing"
)

func TestApplyColorStyle_ExportedSchemeIsCustom(t *testing.T) {
	dir := t.TempDir()
	oldOverrides := UserColorOverridesPath
	UserColorOverridesPath = func() string { return filepath.Join(dir, "farcolors.ini") }
	t.Cleanup(func() { UserColorOverridesPath = oldOverrides })

	oldStyles := getUserStylesDir
	getUserStylesDir = func() string { return filepath.Join(dir, "styles") }
	t.Cleanup(func() { getUserStylesDir = oldStyles })

	oldCfg := config.App
	config.App.EnforceColorCorrection = false
	config.App.ColorStyle = "Modern"
	t.Cleanup(func() { config.App = oldCfg })

	if err := ApplyColorStyle("Modern"); err != nil {
		t.Fatalf("ApplyColorStyle(Modern): %v", err)
	}
	wantCustom := vtui.SetRGBBoth(0, 0x123456, 0x654321)
	vtui.Palette[ColPanelText] = wantCustom
	if err := ExportColors(UserColorOverridesPath()); err != nil {
		t.Fatalf("ExportColors: %v", err)
	}

	styles := AvailableColorStyles()
	custom, found := findColorStyle(styles, CustomColorStyleName)
	if !found || !custom.custom {
		t.Fatalf("AvailableColorStyles() = %v, want a Custom style", styleNames(styles))
	}
	if base := custom.ini.GetString("style", "Base", ""); base != "Modern" {
		t.Fatalf("Custom base style = %q, want Modern", base)
	}

	if err := ApplyColorStyle("Classic"); err != nil {
		t.Fatalf("ApplyColorStyle(Classic): %v", err)
	}
	if got := vtui.Palette[ColPanelText]; got == wantCustom {
		t.Fatalf("Classic was masked by exported farcolors.ini: got %#x", got)
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColPanelText]); got != 0x0000A0 {
		t.Fatalf("Classic panel background = %06X, want 0000A0", got)
	}

	if err := ApplyColorStyle(CustomColorStyleName); err != nil {
		t.Fatalf("ApplyColorStyle(Custom): %v", err)
	}
	if got := vtui.Palette[ColPanelText]; got != wantCustom {
		t.Fatalf("Custom panel text = %#x, want %#x", got, wantCustom)
	}
}
