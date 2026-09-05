package vtui

import "testing"

func TestComboBox_ResizeKeepsEditAdjacentToArrow(t *testing.T) {
	cb := NewComboBox(2, 3, 10, []string{"One", "Two"})
	cb.SetPosition(4, 5, 39, 5)

	if cb.Edit.X1 != cb.X1 || cb.Edit.Y1 != cb.Y1 {
		t.Fatalf("edit origin = (%d,%d), want (%d,%d)", cb.Edit.X1, cb.Edit.Y1, cb.X1, cb.Y1)
	}
	if cb.Edit.X2 != cb.X2-1 {
		t.Fatalf("edit right edge = %d, want %d before arrow at %d", cb.Edit.X2, cb.X2-1, cb.X2)
	}

	cb.MoveRelative(3, 2)
	if cb.Edit.X1 != cb.X1 || cb.Edit.X2 != cb.X2-1 || cb.Edit.Y1 != cb.Y1 {
		t.Fatalf("after MoveRelative combo = (%d,%d)-(%d,%d), edit = (%d,%d)-(%d,%d)",
			cb.X1, cb.Y1, cb.X2, cb.Y2, cb.Edit.X1, cb.Edit.Y1, cb.Edit.X2, cb.Edit.Y2)
	}
}
