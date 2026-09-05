package main

import (
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func TestApplyColorStyle_ExportedSchemeIsCustom(t *testing.T) {
	dir := t.TempDir()
	oldOverrides := userColorOverridesPath
	userColorOverridesPath = func() string { return filepath.Join(dir, "farcolors.ini") }
	t.Cleanup(func() { userColorOverridesPath = oldOverrides })

	oldStyles := getUserStylesDir
	getUserStylesDir = func() string { return filepath.Join(dir, "styles") }
	t.Cleanup(func() { getUserStylesDir = oldStyles })

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	AppConfig.ColorStyle = "Modern"
	t.Cleanup(func() { AppConfig = oldCfg })

	if err := ApplyColorStyle("Modern"); err != nil {
		t.Fatalf("ApplyColorStyle(Modern): %v", err)
	}
	wantCustom := vtui.SetRGBBoth(0, 0x123456, 0x654321)
	vtui.Palette[ColPanelText] = wantCustom
	if err := ExportColors(userColorOverridesPath()); err != nil {
		t.Fatalf("ExportColors: %v", err)
	}

	styles := AvailableColorStyles()
	custom, found := findColorStyle(styles, customColorStyleName)
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

	if err := ApplyColorStyle(customColorStyleName); err != nil {
		t.Fatalf("ApplyColorStyle(Custom): %v", err)
	}
	if got := vtui.Palette[ColPanelText]; got != wantCustom {
		t.Fatalf("Custom panel text = %#x, want %#x", got, wantCustom)
	}
}
