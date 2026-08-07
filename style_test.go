package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func TestAvailableColorStylesIncludesBuiltInsAndUserStyles(t *testing.T) {
	oldDir := getUserStylesDir
	userDir := t.TempDir()
	getUserStylesDir = func() string { return userDir }
	defer func() { getUserStylesDir = oldDir }()

	custom := []byte("[style]\nName = Solarized\n\n[farcolors]\nPanel.Text = foreground:#123456\n")
	if err := os.WriteFile(filepath.Join(userDir, "solarized.ini"), custom, 0644); err != nil {
		t.Fatal(err)
	}

	styles := AvailableColorStyles()
	if len(styles) != 4 {
		t.Fatalf("expected 4 styles (Modern, Classic, Default Dark, Solarized); got %v", styleNames(styles))
	}
	if styles[0].Name != "Modern" || styles[1].Name != "Classic" {
		t.Fatalf("first two styles should be Modern and Classic, got %v", styleNames(styles[:2]))
	}
	// Check that both Default Dark and Solarized are present
	foundDefaultDark, foundSolarized := false, false
	for _, s := range styles {
		if s.Name == "Default Dark" {
			foundDefaultDark = true
		}
		if s.Name == "Solarized" {
			foundSolarized = true
		}
	}
	if !foundDefaultDark || !foundSolarized {
		t.Fatalf("expected Default Dark and Solarized, got %v", styleNames(styles))
	}
}

func TestApplyColorStyleModernAndClassic(t *testing.T) {
	oldDir := getUserStylesDir
	userDir := t.TempDir()
	getUserStylesDir = func() string { return userDir }
	defer func() { getUserStylesDir = oldDir }()

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	defer func() { AppConfig = oldCfg }()

	if err := ApplyColorStyle("Modern"); err != nil {
		t.Fatal(err)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelText]); got != 0xAAAAAA {
		t.Fatalf("modern panel text: got %06X", got)
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColPanelText]); got != 0x232323 {
		t.Fatalf("modern panel background: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelCursor]); got != 0xAAAAAA {
		t.Fatalf("modern panel cursor text: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelSelectedText]); got != 0xF1EC0E {
		t.Fatalf("modern selected panel text: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelSelectedCursor]); got != 0xF1EC0E {
		t.Fatalf("modern selected panel cursor text: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelScrollbar]); got != 0x5A5A5A {
		t.Fatalf("modern full panel scrollbar: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelMinimalScrollbar]); got != 0xA0A0A0 {
		t.Fatalf("modern minimal panel scrollbar: got %06X", got)
	}

	if err := ApplyColorStyle("Classic"); err != nil {
		t.Fatal(err)
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColPanelText]); got != 0x0000A0 {
		t.Fatalf("classic panel background: got %06X", got)
	}
	if got := vtui.GetRGBBack(vtui.Palette[vtui.ColKeyBarNum]); got != 0x000000 {
		t.Fatalf("classic key bar gap background: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[vtui.ColKeyBarText]); got != 0x000000 {
		t.Fatalf("classic key bar label text: got %06X", got)
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColViewerText]); got != 0x0000A0 {
		t.Fatalf("classic viewer background: got %06X", got)
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColEditorText]); got != 0x0000A0 {
		t.Fatalf("classic editor background: got %06X", got)
	}
}

func styleNames(styles []ColorStyle) []string {
	names := make([]string, len(styles))
	for i, style := range styles {
		names[i] = style.Name
	}
	return names
}
