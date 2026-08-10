package vtui

import "testing"

// A dropdown must take its colours from the combo group rather than from the
// menu group, so that a theme can keep it distinct from the dialog behind it.
func TestComboBox_DropdownUsesComboPalette(t *testing.T) {
	cb := NewComboBox(0, 0, 10, []string{"one", "two"})

	cases := []struct {
		name      string
		got, want int
	}{
		{"text", cb.Menu.ColorTextIdx, ColDialogComboText},
		{"selected text", cb.Menu.ColorSelectedTextIdx, ColDialogComboSelectedText},
		{"highlight", cb.Menu.ColorHighlightIdx, ColDialogComboHighlight},
		{"selected highlight", cb.Menu.ColorSelectedHighlightIdx, ColDialogComboSelectedHighlight},
		{"box", cb.Menu.ColorBoxIdx, ColDialogComboBox},
		{"title", cb.Menu.ColorTitleIdx, ColDialogComboTitle},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("dropdown %s index = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
	if cb.Menu.ScrollBar == nil {
		t.Fatal("dropdown has no scrollbar")
	}
	if cb.Menu.ScrollBar.ColorIdx != ColDialogComboScrollbar {
		t.Errorf("dropdown scrollbar index = %d, want %d",
			cb.Menu.ScrollBar.ColorIdx, ColDialogComboScrollbar)
	}
}

// The visible symptom the indices above are there to prevent: a dropdown
// painted in the dialog's own background, indistinguishable from it.
func TestComboBox_DropdownStandsApartFromDialog(t *testing.T) {
	SetDefaultPalette()
	Palette[ColDialogText] = SetRGBBoth(0, 0x2E3436, 0xD3D7CF)
	Palette[ColDialogComboText] = SetRGBBoth(0, 0xEEEEEC, 0x06989A)
	Palette[ColDialogComboSelectedText] = SetRGBBoth(0, 0xEEEEEC, 0x2E3436)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 8)

	cb := NewComboBox(0, 0, 10, []string{"one", "two"})
	cb.Menu.SetPosition(0, 0, 10, 4)
	cb.Menu.Show(scr)

	// A menu selects its first item on construction, so row 1 is the selected
	// one and row 2 an ordinary item. Both must come from the combo group;
	// neither may fall back to the dialog background behind the dropdown.
	if got := GetRGBBack(scr.GetCell(2, 2).Attributes); got != 0x06989A {
		t.Errorf("dropdown background = #%06x, want the combo colour #06989a", got)
	}
	if got := GetRGBBack(scr.GetCell(2, 1).Attributes); got != 0x2E3436 {
		t.Errorf("selected row background = #%06x, want #2e3436", got)
	}
}

// A plain menu keeps the Menu.* group.
func TestVMenu_DefaultsToMenuPalette(t *testing.T) {
	m := NewVMenu("Menu")
	if m.ColorTextIdx != ColMenuText || m.ColorBoxIdx != ColMenuBox || m.ColorTitleIdx != ColMenuTitle {
		t.Errorf("plain menu should default to the Menu.* entries, got %d/%d/%d",
			m.ColorTextIdx, m.ColorBoxIdx, m.ColorTitleIdx)
	}
}
