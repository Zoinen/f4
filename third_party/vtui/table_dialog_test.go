package vtui

import "testing"

func tableDialogFixture(t *testing.T) (*Window, *Table) {
	t.Helper()
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	FrameManager.Init(scr)

	btnA := NewButton(0, 0, "&Alpha")
	btnB := NewButton(0, 0, "&Beta")
	dlg, table := NewTableDialog(60, 20, " Test ", []TableColumn{
		{Title: "ID", Width: 4},
		{Title: "Name"},
	}, btnA, btnB)
	return dlg, table
}

func TestNewTableDialogLayout(t *testing.T) {
	dlg, table := tableDialogFixture(t)

	dx1, dy1, dx2, dy2 := dlg.GetPosition()
	tx1, ty1, tx2, ty2 := table.GetPosition()
	if tx1 != dx1+2 || ty1 != dy1+2 {
		t.Errorf("Table origin: got (%d,%d), want (%d,%d)", tx1, ty1, dx1+2, dy1+2)
	}
	if tx2 != dx2-2 {
		t.Errorf("Table right edge: got %d, want %d", tx2, dx2-2)
	}
	// One blank row between the table and the button row
	btnY := dy2 - 2
	if ty2 != btnY-2 {
		t.Errorf("Table bottom: got %d, want %d (button row %d)", ty2, btnY-2, btnY)
	}

	// Buttons: bottom interior row, centered as a row with spacing 2
	var items []UIElement
	for _, it := range dlg.rootGroup.items {
		if b, ok := it.(*Button); ok {
			items = append(items, b)
		}
	}
	if len(items) != 2 {
		t.Fatalf("Expected 2 buttons in dialog, got %d", len(items))
	}
	b1x1, b1y1, b1x2, _ := items[0].GetPosition()
	b2x1, b2y1, b2x2, _ := items[1].GetPosition()
	if b1y1 != btnY || b2y1 != btnY {
		t.Errorf("Buttons not on the bottom interior row: y1=%d,%d want %d", b1y1, b2y1, btnY)
	}
	if b2x1-b1x2-1 != 2 {
		t.Errorf("Button spacing: got %d, want 2", b2x1-b1x2-1)
	}
	rowW := b2x2 - b1x1 + 1
	leftPad := b1x1 - (dx1 + 2)
	rightPad := (dx2 - 2) - b2x2
	if leftPad != rightPad && leftPad != rightPad-1 {
		t.Errorf("Button row not centered: left pad %d, right pad %d (row width %d)", leftPad, rightPad, rowW)
	}

	// Grow modes: table stretches, the button row container sticks to the
	// bottom and grows horizontally; buttons themselves are owned by the row.
	if table.GetGrowMode() != (GrowHiX | GrowHiY) {
		t.Errorf("Table grow mode: got %#x, want GrowHiX|GrowHiY", table.GetGrowMode())
	}
	var row *HBoxLayout
	for _, it := range dlg.rootGroup.items {
		if h, ok := it.(*HBoxLayout); ok {
			row = h
		}
	}
	if row == nil {
		t.Fatal("Button row HBoxLayout is not a dialog item")
	}
	if row.GetGrowMode() != (GrowLoY | GrowHiY | GrowHiX) {
		t.Errorf("Button row grow mode: got %#x, want GrowLoY|GrowHiY|GrowHiX", row.GetGrowMode())
	}
}

func TestNewTableDialogButtonsStayCenteredOnResize(t *testing.T) {
	dlg, _ := tableDialogFixture(t)
	dx1, _, _, _ := dlg.GetPosition()

	centered := func(w int) bool {
		var btns []UIElement
		for _, it := range dlg.rootGroup.items {
			if b, ok := it.(*Button); ok {
				btns = append(btns, b)
			}
		}
		b1x1, _, _, _ := btns[0].GetPosition()
		_, _, b2x2, _ := btns[1].GetPosition()
		leftPad := b1x1 - (dx1 + 2)
		rightPad := (dx1 + w - 3) - b2x2
		return leftPad == rightPad || leftPad == rightPad-1
	}

	if !centered(60) {
		t.Fatal("Button row not centered initially")
	}
	dlg.ChangeSize(80, 20)
	if !centered(80) {
		t.Error("Button row lost centering after growing the dialog")
	}
	dlg.ChangeSize(40, 20)
	if !centered(40) {
		t.Error("Button row lost centering after shrinking the dialog")
	}
}

func TestNewTableDialogMinWidth(t *testing.T) {
	dlg, _ := tableDialogFixture(t)
	// "[ Alpha ]"=9 + spacing 2 + "[ Beta ]"=8 = 19, plus 4 for frame and padding
	if dlg.MinW != 23 {
		t.Errorf("MinW: got %d, want 23", dlg.MinW)
	}
}

func TestNewTableDialogRequiresFlexibleColumn(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	FrameManager.Init(scr)

	defer func() {
		if recover() == nil {
			t.Error("Expected panic when all columns are fixed-width")
		}
	}()
	NewTableDialog(60, 20, " Test ", []TableColumn{
		{Title: "ID", Width: 4},
		{Title: "Name", Width: 20},
	}, NewButton(0, 0, "&Ok"))
}

func TestNewTableDialogPassesValidator(t *testing.T) {
	dlg, _ := tableDialogFixture(t)
	AssertLayout(t, dlg)
}
