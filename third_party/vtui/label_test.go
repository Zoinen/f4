package vtui

import "testing"

func TestNewLabel_Linkage(t *testing.T) {
	SetDefaultPalette()
	edit := NewEdit(10, 10, 20, "")
	label := NewLabel(0, 0, "&Name:", edit)

	// Check that the link is established
	if label.FocusLink != edit {
		t.Error("Label FocusLink was not set correctly")
	}

	// Check the hotkey
	if label.GetHotkey() != 'n' {
		t.Errorf("Expected hotkey 'n', got %c", label.GetHotkey())
	}

	// Check the color (should be ColDialogText by default)
	if label.color != Palette[ColDialogText] {
		t.Error("Label did not use default ColDialogText color")
	}
}
