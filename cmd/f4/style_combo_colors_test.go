package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func comboStyle(t *testing.T, name string) {
	t.Helper()
	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	t.Cleanup(func() { AppConfig = oldCfg })
	if err := ApplyColorStyle(name); err != nil {
		t.Fatalf("ApplyColorStyle(%q): %v", name, err)
	}
}

// The dropdown has to stand apart from the dialog it opens over. In Default
// Dark that means far2l's turquoise against the light grey dialog.
func TestDefaultDarkStyle_ComboStandsApartFromDialog(t *testing.T) {
	comboStyle(t, "Default Dark")

	_, dialogBg := GetColorRGBBoth(vtui.Palette[vtui.ColDialogText])
	comboFg, comboBg := GetColorRGBBoth(vtui.Palette[vtui.ColDialogComboText])

	if comboBg == dialogBg {
		t.Errorf("combo background #%06x is the same as the dialog background", comboBg)
	}
	if comboFg != 0xEEEEEC || comboBg != 0x06989A {
		t.Errorf("Dialog.Combo.Text = #%06x on #%06x, want #eeeeec on #06989a", comboFg, comboBg)
	}
}

// Modern editable and dropdown-only combos use the same input surface as
// regular edit controls instead of disappearing into the dialog background.
func TestModernStyle_ComboMatchesEdit(t *testing.T) {
	comboStyle(t, "Modern")

	editFg, editBg := GetColorRGBBoth(vtui.Palette[vtui.ColDialogEdit])
	comboFg, comboBg := GetColorRGBBoth(vtui.Palette[vtui.ColDialogComboText])
	if comboFg != editFg || comboBg != editBg {
		t.Errorf("Modern combo = #%06x on #%06x, want the edit colours #%06x on #%06x",
			comboFg, comboBg, editFg, editBg)
	}
}

// Yellow on light grey is unreadable, so Default Dark uses red for hotkeys the
// way it already does in menus.
func TestDefaultDarkStyle_HotkeysAreReadable(t *testing.T) {
	comboStyle(t, "Default Dark")

	cases := []struct {
		name  string
		index int
	}{
		{"Dialog.Text.Highlight", vtui.ColDialogHighlightText},
		{"Dialog.Button.Highlight", vtui.ColDialogHighlightButton},
		{"Dialog.Box.Title.Highlight", vtui.ColDialogHighlightBoxTitle},
		{"WarnDialog.Button.Highlight.Selected", vtui.ColWarnHighlightSelectedButton},
	}
	for _, tc := range cases {
		fg, bg := GetColorRGBBoth(vtui.Palette[tc.index])
		if bg != 0xD3D7CF {
			continue
		}
		if fg != 0xCC0000 {
			t.Errorf("%s = #%06x on light grey, want the red #cc0000", tc.name, fg)
		}
	}
}
