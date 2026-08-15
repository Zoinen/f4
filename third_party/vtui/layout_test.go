package vtui

import (
	"testing"
)

// --- VBoxLayout Tests ---

func TestVBoxLayout_Alignment(t *testing.T) {
	// Container: X=0, W=100
	vbox := NewVBoxLayout(0, 0, 100, 50)

	left := NewButton(0, 0, "L")   // width 5: "[ L ]"
	center := NewButton(0, 0, "C") // width 5
	right := NewButton(0, 0, "R")  // width 5
	fill := NewEdit(0, 0, 10, "F") // initial width 10

	vbox.Add(left, Margins{Left: 10}, AlignLeft)
	vbox.Add(center, Margins{}, AlignCenter)
	vbox.Add(right, Margins{Right: 10}, AlignRight)
	vbox.Add(fill, Margins{Left: 5, Right: 5}, AlignFill)

	vbox.Apply()

	// 1. AlignLeft: 0 (X) + 10 (Margin) = 10
	x1, _, _, _ := left.GetPosition()
	if x1 != 10 {
		t.Errorf("AlignLeft failed: expected 10, got %d", x1)
	}

	// 2. AlignCenter: (100 - 5) / 2 = 47 (approx)
	x1, _, _, _ = center.GetPosition()
	if x1 != 47 && x1 != 48 {
		t.Errorf("AlignCenter failed: expected ~47, got %d", x1)
	}

	// 3. AlignRight: 100 - 10 (Margin) - 5 (Width) = 85
	x1, _, _, _ = right.GetPosition()
	if x1 != 85 {
		t.Errorf("AlignRight failed: expected 85, got %d", x1)
	}

	// 4. AlignFill: X = 0+5=5, Width = 100-5-5=90. X2 = 5+90-1 = 94
	x1, _, x2, _ := fill.GetPosition()
	if x1 != 5 || x2 != 94 {
		t.Errorf("AlignFill failed: expected 5..94, got %d..%d", x1, x2)
	}
}

func TestVBoxLayout_VerticalSpacing(t *testing.T) {
	vbox := NewVBoxLayout(0, 10, 50, 50)

	// Buttons are 1 line high
	b1 := NewButton(0, 0, "B1")
	b2 := NewButton(0, 0, "B2")

	// B1: top margin 2, bottom margin 3. Starts at Y=10+2=12. Ends at 12.
	vbox.Add(b1, Margins{Top: 2, Bottom: 3}, AlignLeft)
	// B2: top margin 1. Should start at B1_Y2 (12) + B1_Bottom (3) + B2_Top (1) + 1 = 17
	vbox.Add(b2, Margins{Top: 1}, AlignLeft)

	vbox.Apply()

	_, y1, _, y2 := b1.GetPosition()
	if y1 != 12 || y2 != 12 {
		t.Errorf("B1 Y coords wrong: %d..%d", y1, y2)
	}

	_, by1, _, _ := b2.GetPosition()
	if by1 != 17 {
		t.Errorf("B2 Y coords wrong: expected 17, got %d", by1)
	}
}

// --- HBoxLayout Tests ---

func TestHBoxLayout_SpacingAndMargins(t *testing.T) {
	// Container height 1, X=0, W=100
	hbox := NewHBoxLayout(0, 0, 100, 1)
	hbox.Spacing = 5

	b1 := NewButton(0, 0, "B1") // width 6 "[ B1 ]"
	b2 := NewButton(0, 0, "B2") // width 6

	hbox.Add(b1, Margins{Left: 2, Right: 3}, AlignTop)
	hbox.Add(b2, Margins{Left: 1}, AlignTop)

	hbox.Apply()

	// B1: X = 0 + 2 (Margin) = 2. X2 = 2+6-1 = 7
	x1, _, x2, _ := b1.GetPosition()
	if x1 != 2 || x2 != 7 {
		t.Errorf("HBox B1 failed: %d..%d", x1, x2)
	}

	// B2: X = B1_X2 (7) + B1_Right (3) + Spacing (5) + B2_Left (1) + 1 = 17
	bx1, _, _, _ := b2.GetPosition()
	if bx1 != 17 {
		t.Errorf("HBox B2 failed: expected 17, got %d", bx1)
	}
}

func TestHBoxLayout_CrossAlignment(t *testing.T) {
	// Container: height 10, Y=10
	hbox := NewHBoxLayout(0, 10, 100, 10)

	top := NewButton(0, 0, "T") // height 1
	center := NewButton(0, 0, "C")
	bottom := NewButton(0, 0, "B")
	fill := NewEdit(0, 0, 10, "F")

	hbox.Add(top, Margins{Top: 1}, AlignTop)
	hbox.Add(center, Margins{}, AlignCenter)
	hbox.Add(bottom, Margins{Bottom: 1}, AlignBottom)
	hbox.Add(fill, Margins{Top: 2, Bottom: 2}, AlignFill)

	hbox.Apply()

	// 1. AlignTop: 10 (Y) + 1 (Margin) = 11
	_, y1, _, _ := top.GetPosition()
	if y1 != 11 {
		t.Errorf("HBox AlignTop failed: %d", y1)
	}

	// 2. AlignCenter: 10 + (10 - 1) / 2 = 14 (approx)
	_, y1, _, _ = center.GetPosition()
	if y1 != 14 && y1 != 15 {
		t.Errorf("HBox AlignCenter failed: %d", y1)
	}

	// 3. AlignBottom: 10 + 10 (Height) - 1 (Margin) - 1 (Width) = 18
	_, y1, _, _ = bottom.GetPosition()
	if y1 != 18 {
		t.Errorf("HBox AlignBottom failed: %d", y1)
	}

	// 4. AlignFill: Y1 = 10+2=12, Height = 10-2-2=6. Y2 = 12+6-1 = 17
	_, y1, _, y2 := fill.GetPosition()
	if y1 != 12 || y2 != 17 {
		t.Errorf("HBox AlignFill failed: %d..%d", y1, y2)
	}
}

func TestHBoxLayout_BlockAlignment(t *testing.T) {
	hbox := NewHBoxLayout(0, 0, 100, 1)
	hbox.Spacing = 0

	b1 := NewButton(0, 0, "B1") // width 6
	b2 := NewButton(0, 0, "B2") // width 6

	hbox.Add(b1, Margins{}, AlignTop)
	hbox.Add(b2, Margins{}, AlignTop)

	// --- 1. Right alignment of the whole block ---
	hbox.HorizontalAlign = AlignRight
	hbox.Apply()
	// Total width = 6 + 6 = 12. Block should start at 100 - 12 = 88
	x1, _, _, _ := b1.GetPosition()
	if x1 != 88 {
		t.Errorf("HBox AlignBlockRight failed: expected 88, got %d", x1)
	}

	// --- 2. Center alignment of the whole block ---
	hbox.HorizontalAlign = AlignCenter
	hbox.Apply()
	// Total width 12. Start at (100 - 12) / 2 = 44
	x1, _, _, _ = b1.GetPosition()
	if x1 != 44 {
		t.Errorf("HBox AlignBlockCenter failed: expected 44, got %d", x1)
	}
}

func TestLayout_NegativeMarginsSafety(t *testing.T) {
	// Проверка, что движок не падает при странных маргинах (хотя использовать их не стоит)
	vbox := NewVBoxLayout(0, 0, 10, 10)
	btn := NewButton(0, 0, "B")
	vbox.Add(btn, Margins{Top: -5, Left: -5}, AlignLeft)

	// Should not panic
	vbox.Apply()

	x1, y1, _, _ := btn.GetPosition()
	if x1 != -5 || y1 != -5 {
		t.Errorf("Negative margins should be respected if provided: (%d, %d)", x1, y1)
	}
}
func TestLayout_NestedBoxFill(t *testing.T) {
	// Tests that AlignFill correctly propagates through nested layouts.
	// Layouts only stretch elements on the CROSS axis.

	// Outer HBox (W=100, H=10)
	hbox := NewHBoxLayout(0, 0, 100, 10)
	hbox.Spacing = 0

	// VBox (W=20, H=1) as child.
	// AlignFill in HBox means "stretching vertically to fill H=10"
	vbox := NewVBoxLayout(0, 0, 20, 1)
	hbox.Add(vbox, Margins{}, AlignFill)

	// Inside VBox, a button.
	// AlignFill in VBox means "stretching horizontally to fill W=20"
	btn := NewButton(0, 0, "B") // width 5: "[ B ]"
	vbox.Add(btn, Margins{Left: 2, Right: 2}, AlignFill)

	hbox.Apply()

	// 1. Verify VBox stretched vertically
	_, vy1, _, vy2 := vbox.GetPosition()
	if vy1 != 0 || vy2 != 9 {
		t.Errorf("Nested VBox did not fill HBox height. Got Y: %d..%d, want 0..9", vy1, vy2)
	}

	// 2. Verify Button stretched horizontally inside VBox
	// VBox width is 20. Margins 2+2=4. Final width 16.
	// X1 = 0 + 2 = 2. X2 = 2 + 16 - 1 = 17.
	bx1, _, bx2, _ := btn.GetPosition()
	if bx1 != 2 || bx2 != 17 {
		t.Errorf("Nested button did not fill VBox width correctly. Got X: %d..%d, want 2..17", bx1, bx2)
	}
}
