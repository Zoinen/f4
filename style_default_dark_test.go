package main

import (
	"testing"

	"github.com/unxed/vtui"
)

// Default Dark is the port of the far2l theme of the same name. Far2l Dark was
// an earlier, approximate attempt at the same thing and has been dropped.
func TestDefaultDarkStyle(t *testing.T) {
	if err := ApplyColorStyle("Default Dark"); err != nil {
		t.Fatalf("Failed to apply Default Dark style: %v", err)
	}
}

// Colours the earlier attempt got wrong, spot-checked against the far2l theme.
// Contrast correction is off here so the palette is compared as authored.
func TestDefaultDarkStyle_PanelColors(t *testing.T) {
	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	defer func() { AppConfig = oldCfg }()

	if err := ApplyColorStyle("Default Dark"); err != nil {
		t.Fatalf("Failed to apply Default Dark style: %v", err)
	}

	cases := []struct {
		name   string
		index  int
		fg, bg uint32
	}{
		// Panel.Text is B_BLACK | F_LIGHTCYAN — the black background Far2l Dark
		// replaced with far2l's stock blue.
		{"Panel.Text", ColPanelText, 0x34E2E2, 0x2E3436},
		{"Panel.Title", ColPanelTitle, 0xD3D7CF, 0x2E3436},
		{"Panel.Title.Selected", ColPanelSelectedTitle, 0xD3D7CF, 0x555753},
		{"Panel.Title.Column", ColPanelColumnTitle, 0x8AE234, 0x2E3436},
		{"Panel.Text.Selected", ColPanelSelectedText, 0xFCE94F, 0x2E3436},
		{"Panel.Info.Total", ColPanelTotalInfo, 0xEEEEEC, 0x2E3436},
		// Dark text on teal: the pair that used to be flipped to white by the
		// old contrast approximation.
		{"Keybar.Text", vtui.ColKeyBarText, 0x2E3436, 0x06989A},
		{"HMenu.Text", vtui.ColMenuBarItem, 0x2E3436, 0xD3D7CF},
	}
	for _, tc := range cases {
		fg, bg := GetColorRGBBoth(vtui.Palette[tc.index])
		if fg != tc.fg || bg != tc.bg {
			t.Errorf("%s = #%06x on #%06x, want #%06x on #%06x", tc.name, fg, bg, tc.fg, tc.bg)
		}
	}
}

// With correction on, far2l's algorithm leaves these pairs alone. If a future
// change makes the keybar go white again, this is the test that catches it.
func TestDefaultDarkStyle_SurvivesContrastCorrection(t *testing.T) {
	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = true
	defer func() { AppConfig = oldCfg }()

	if err := ApplyColorStyle("Default Dark"); err != nil {
		t.Fatalf("Failed to apply Default Dark style: %v", err)
	}

	cases := []struct {
		name   string
		index  int
		fg, bg uint32
	}{
		{"Keybar.Text", vtui.ColKeyBarText, 0x2E3436, 0x06989A},
		{"Panel.Text", ColPanelText, 0x34E2E2, 0x2E3436},
		{"Menu.Text.Selected", vtui.ColMenuSelectedText, 0x2E3436, 0x4E9A06},
	}
	for _, tc := range cases {
		fg, bg := GetColorRGBBoth(vtui.Palette[tc.index])
		if fg != tc.fg || bg != tc.bg {
			t.Errorf("%s = #%06x on #%06x after correction, want #%06x on #%06x",
				tc.name, fg, bg, tc.fg, tc.bg)
		}
	}
}
