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

	// A zero color means that the label resolves ColDialogText when it is
	// painted, so an already open dialog follows runtime theme changes.
	if label.color != 0 {
		t.Errorf("Label stored a fixed color %#x instead of the live Dialog.Text color", label.color)
	}
}
