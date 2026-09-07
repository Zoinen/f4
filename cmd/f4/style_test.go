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
	if err := os.WriteFile(filepath.Join(userDir, "solarized.ini"), custom, 0600); err != nil {
		t.Fatal(err)
	}

	styles := AvailableColorStyles()
	// The built-in set grows with the repo (Radiola landed after this
	// test was first written); accept "the built-ins plus our
	// Solarized" without pinning the exact count.
	if len(styles) < 4 {
		t.Fatalf("expected at least 4 styles (built-ins + user Solarized); got %v", styleNames(styles))
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
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelHighlightText]); got != 0xC678DD {
		t.Fatalf("modern fast-find match highlight: got %06X", got)
	}
	if got := vtui.GetRGBFore(vtui.Palette[ColPanelSelectedCursor]); got != 0xF1EC0E {
		t.Fatalf("modern selected panel cursor text: got %06X", got)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelSelectedTitle]); fg != 0xFFFFFF || bg != 0x434343 {
		t.Fatalf("modern selected panel title: got %06X on %06X", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelTotalInfo]); fg != 0xD0D0D0 || bg != 0x232323 {
		t.Fatalf("modern panel total info: got %06X on %06X", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelSelectedInfo]); fg != 0xF1EC0E || bg != 0x3B6290 {
		t.Fatalf("modern panel selected info: got %06X on %06X", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabs]); fg != 0xAAAAAA || bg != 0x000000 {
		t.Fatalf("modern workspace tabs: got %06X on %06X", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsActive]); fg != 0xFFFFFF || bg != 0x232323 {
		t.Fatalf("modern active workspace tab: got %06X on %06X", fg, bg)
	}
	if fg, _ := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsAccent]); fg != 0x7FAFE3 {
		t.Fatalf("modern workspace tab accent: got %06X", fg)
	}
	if fg, _ := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsAttention]); fg != 0xFF9F43 {
		t.Fatalf("modern workspace tab attention: got %06X", fg)
	}
	if _, editBG := GetColorRGBBoth(vtui.Palette[vtui.ColDialogEdit]); editBG != 0x232323 {
		t.Fatalf("modern dialog edit background: got %06X", editBG)
	}
	for name, idx := range map[string]int{
		"text":      vtui.ColDialogComboText,
		"highlight": vtui.ColDialogComboHighlight,
		"box":       vtui.ColDialogComboBox,
		"title":     vtui.ColDialogComboTitle,
		"scrollbar": vtui.ColDialogComboScrollbar,
	} {
		if _, bg := GetColorRGBBoth(vtui.Palette[idx]); bg != 0x232323 {
			t.Fatalf("modern dialog combo %s background: got %06X, want edit background 232323", name, bg)
		}
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
	if got := vtui.GetRGBBack(vtui.Palette[ColPanelSelectedText]); got != 0x0000A0 {
		t.Fatalf("classic selected panel background: got %06X, want 0000A0", got)
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

func TestApplyColorStyleRadiolaWorkspaceTabs(t *testing.T) {
	oldDir := getUserStylesDir
	getUserStylesDir = func() string { return t.TempDir() }
	defer func() { getUserStylesDir = oldDir }()

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	defer func() { AppConfig = oldCfg }()

	if err := ApplyColorStyle("Radiola"); err != nil {
		t.Fatal(err)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabs]); fg != 0xA69F94 || bg != 0x292620 {
		t.Fatalf("radiola inactive workspace tabs: got %06X on %06X", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsActive]); fg != 0xEEEEEC || bg != 0x37322C {
		t.Fatalf("radiola active workspace tab: got %06X on %06X", fg, bg)
	}
	if fg, _ := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsAccent]); fg != 0xE6B450 {
		t.Fatalf("radiola workspace tab accent: got %06X", fg)
	}
	if fg, _ := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsAttention]); fg != 0xEC6A2C {
		t.Fatalf("radiola workspace tab attention: got %06X", fg)
	}
	if _, textBG := GetColorRGBBoth(vtui.Palette[vtui.ColDialogComboText]); textBG != 0xC4B8A8 {
		t.Fatalf("radiola combo dropdown background: got %06X", textBG)
	} else if _, boxBG := GetColorRGBBoth(vtui.Palette[vtui.ColDialogComboBox]); boxBG != textBG {
		t.Fatalf("radiola combo border background: got %06X, want dropdown background %06X", boxBG, textBG)
	}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(20, 6)
	combo := vtui.NewComboBox(0, 0, 10, []string{"One", "Two"})
	combo.Menu.SetPosition(0, 0, 10, 4)
	combo.Menu.Show(scr)
	borderBG := vtui.GetRGBBack(scr.GetCell(0, 2).Attributes)
	itemBG := vtui.GetRGBBack(scr.GetCell(2, 2).Attributes)
	if borderBG != itemBG {
		t.Fatalf("rendered radiola combo border background %06X differs from dropdown row %06X", borderBG, itemBG)
	}
}

func TestApplyColorStyleDefaultDarkWorkspaceTabs(t *testing.T) {
	oldDir := getUserStylesDir
	userDir := t.TempDir()
	getUserStylesDir = func() string { return userDir }
	defer func() { getUserStylesDir = oldDir }()

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	defer func() { AppConfig = oldCfg }()

	if err := ApplyColorStyle("Default Dark"); err != nil {
		t.Fatal(err)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabs]); fg != 0x8F9696 || bg != 0x252A2B {
		t.Fatalf("default dark inactive workspace tabs: got %06X on %06X", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsActive]); fg != 0xEEEEEC || bg != 0x2E3436 {
		t.Fatalf("default dark active workspace tab: got %06X on %06X", fg, bg)
	}
	if fg, _ := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsAccent]); fg != 0xFCE94F {
		t.Fatalf("default dark workspace tab accent: got %06X", fg)
	}
	if fg, _ := GetColorRGBBoth(vtui.Palette[ColPanelWorkspaceTabsAttention]); fg != 0xEF2929 {
		t.Fatalf("default dark workspace tab attention: got %06X", fg)
	}
}

func styleNames(styles []ColorStyle) []string {
	names := make([]string, len(styles))
	for i, style := range styles {
		names[i] = style.Name
	}
	return names
}
