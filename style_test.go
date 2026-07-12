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
	if len(styles) != 3 {
		t.Fatalf("expected Modern, Classic, and Solarized; got %v", styleNames(styles))
	}
	if styles[0].Name != "Modern" || styles[1].Name != "Classic" || styles[2].Name != "Solarized" {
		t.Fatalf("unexpected style order: %v", styleNames(styles))
	}
}

func TestApplyColorStyleModernAndClassic(t *testing.T) {
	oldDir := getUserStylesDir
	userDir := t.TempDir()
	getUserStylesDir = func() string { return userDir }
	defer func() { getUserStylesDir = oldDir }()

	if err := ApplyColorStyle("Modern"); err != nil {
		t.Fatal(err)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelText]); got != 0xAAAAAA {
		t.Fatalf("modern panel text: got %06X", got)
	}
	if got := vtui.GetRGBBack(vtui.Palette[ColPanelText]); got != 0x232323 {
		t.Fatalf("modern panel background: got %06X", got)
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
}

func styleNames(styles []ColorStyle) []string {
	names := make([]string, len(styles))
	for i, style := range styles {
		names[i] = style.Name
	}
	return names
}
