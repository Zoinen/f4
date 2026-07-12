package main

import (
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"testing"
)

func TestColors_SetDefaultF4Palette(t *testing.T) {
	// Initialize default palettes
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	if len(vtui.Palette) < LastF4PaletteColor {
		t.Fatalf("Expected palette size of at least %d, got %d", LastF4PaletteColor, len(vtui.Palette))
	}

	// Verify theme palette defaults
	if vtui.ThemePalette[0] != 0x2E3436 {
		t.Errorf("ThemePalette[0] mismatch: expected %06X, got %06X", 0x2E3436, vtui.ThemePalette[0])
	}
	if vtui.ThemePalette[7] != 0xD3D7CF {
		t.Errorf("ThemePalette[7] mismatch: expected %06X, got %06X", 0xD3D7CF, vtui.ThemePalette[7])
	}

	// Verify some critical f4 palette defaults
	blue := uint32(0x0000A0)
	expectedPanelText := vtui.SetRGBBoth(0, 0x00FFFF, blue)
	if vtui.Palette[ColPanelText] != expectedPanelText {
		t.Errorf("ColPanelText mismatch: expected %016X, got %016X", expectedPanelText, vtui.Palette[ColPanelText])
	}
}

func TestColors_InitColors_FromIni(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "farcolors.ini")

	// Pre-populate with custom colors
	iniContent := `
[farcolors]
Panel.Text = foreground:#FF0000 | background:#0000FF
CommandLine.Prompt = foreground:#00FF00
CommandLine.UserScreen = foreground:#D0D0D0 | background:#010203
Editor.Text = foreground:#A0A0A0 | background:#232323
`
	if err := os.WriteFile(iniPath, []byte(iniContent), 0644); err != nil {
		t.Fatalf("Failed to write mock INI: %v", err)
	}

	ini := LoadIni(iniPath)
	InitColors(ini)

	// Check that ColPanelText got updated to custom Red on Blue
	panelTextAttr := vtui.Palette[ColPanelText]
	if vtui.GetRGBFore(panelTextAttr) != 0xFF0000 {
		t.Errorf("Expected foreground Red #FF0000, got %06X", vtui.GetRGBFore(panelTextAttr))
	}
	if vtui.GetRGBBack(panelTextAttr) != 0x0000FF {
		t.Errorf("Expected background Blue #0000FF, got %06X", vtui.GetRGBBack(panelTextAttr))
	}

	// Check that ColCommandLinePrompt got updated to custom Green
	promptAttr := vtui.Palette[ColCommandLinePrompt]
	if vtui.GetRGBFore(promptAttr) != 0x00FF00 {
		t.Errorf("Expected foreground Green #00FF00, got %06X", vtui.GetRGBFore(promptAttr))
	}
	if vtui.ThemePalette[0] != 0x010203 {
		t.Errorf("Expected terminal background #010203, got %06X", vtui.ThemePalette[0])
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColEditorText]); got != 0x232323 {
		t.Errorf("Expected editor background #232323, got %06X", got)
	}
}
