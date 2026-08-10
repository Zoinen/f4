package vtui

import "testing"

func TestDialog_GrowLogic_Complex(t *testing.T) {
	// 20x10 Dialog at (0,0)
	d := NewDialog(0, 0, 19, 9, "Coverage")

	// 1. GrowNone: Should stay in place
	none := NewButton(1, 1, "Fixed")
	none.SetGrowMode(GrowNone)
	d.AddItem(none)

	// 2. GrowHiX: Stretch to the right
	stretch := NewEdit(5, 5, 5, "") // x1:5, x2:9
	stretch.SetGrowMode(GrowHiX)
	d.AddItem(stretch)

	// 3. GrowLoX | GrowHiX: Full movement
	move := NewButton(10, 8, "Move") // x1:10
	move.SetGrowMode(GrowLoX | GrowHiX)
	d.AddItem(move)

	// Increase width by 20 (becomes 40)
	d.ChangeSize(40, 10)

	// Checks:
	// None: x1=1
	nx1, _, _, _ := none.GetPosition()
	if nx1 != 1 {
		t.Errorf("GrowNone moved! Got x1=%d", nx1)
	}

	// Stretch: x1=5, x2=9 + 20 = 29
	sx1, _, sx2, _ := stretch.GetPosition()
	if sx1 != 5 || sx2 != 29 {
		t.Errorf("Stretch failed: x1=%d, x2=%d", sx1, sx2)
	}

	// Move: x1=10 + 20 = 30
	mx1, _, _, _ := move.GetPosition()
	if mx1 != 30 {
		t.Errorf("Move failed: x1=%d", mx1)
	}

	// 4. Compression test (Negative delta)
	d.ChangeSize(20, 10) // Restore to 20
	mx1, _, _, _ = move.GetPosition()
	if mx1 != 10 {
		t.Errorf("Shrink failed: x1=%d", mx1)
	}
}

func TestDialog_ResizeConsole_Centering(t *testing.T) {
	// 10x10 Dialog
	d := NewDialog(0, 0, 9, 9, "Centering")
	btn := NewButton(1, 1, "B") // x1:1, y1:1
	btn.SetGrowMode(GrowNone)
	d.AddItem(btn)

	// Emulate 100x100 screen. Dialog should center.
	// x1 = (100 - 10) / 2 = 45, y1 = 45.
	// Dialog movement delta: +45, +45.
	d.ResizeConsole(100, 100)

	if d.X1 != 45 || d.Y1 != 45 {
		t.Errorf("Centering failed: got (%d,%d)", d.X1, d.Y1)
	}

	// Button with GrowNone (relative to dialog) should move along with it.
	// MoveRelative inside ResizeConsole should ensure this.
	bx1, by1, _, _ := btn.GetPosition()
	if bx1 != 46 || by1 != 46 { // 1 (orig) + 45 (offset) = 46
		t.Errorf("Child didn't follow centered dialog: got (%d,%d)", bx1, by1)
	}
}

func TestScreenObject_GrowModeAccessors(t *testing.T) {
	so := &ScreenObject{}
	so.SetGrowMode(GrowLoX | GrowHiY)
	if so.GetGrowMode() != (GrowLoX | GrowHiY) {
		t.Error("GrowMode accessors failed")
	}
}
