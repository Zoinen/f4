package vtui

import "testing"

func TestDialogTextAndLabelFollowRuntimePaletteChanges(t *testing.T) {
	SetDefaultPalette()
	original := Palette[ColDialogText]
	defer func() { Palette[ColDialogText] = original }()

	Palette[ColDialogText] = SetRGBBoth(0, 0xAAAAAA, 0x111111)
	text := NewText(1, 1, "Text", Palette[ColDialogText])
	label := NewLabel(1, 2, "Label", nil)

	Palette[ColDialogText] = SetRGBBoth(0, 0xBBBBBB, 0x222222)
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)
	text.Show(scr)
	label.Show(scr)

	for _, y := range []int{1, 2} {
		cell := scr.GetCell(1, y)
		if fg, bg := GetRGBFore(cell.Attributes), GetRGBBack(cell.Attributes); fg != 0xBBBBBB || bg != 0x222222 {
			t.Fatalf("runtime-themed text at y=%d = %06X on %06X, want BBBBBB on 222222", y, fg, bg)
		}
	}
}

func TestTextExplicitCustomColorStaysFixedAcrossPaletteChanges(t *testing.T) {
	SetDefaultPalette()
	original := Palette[ColDialogText]
	defer func() { Palette[ColDialogText] = original }()

	custom := SetRGBBoth(0, 0x123456, 0x654321)
	text := NewText(1, 1, "Custom", custom)
	Palette[ColDialogText] = SetRGBBoth(0, 0xBBBBBB, 0x222222)
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 4)
	text.Show(scr)

	cell := scr.GetCell(1, 1)
	if fg, bg := GetRGBFore(cell.Attributes), GetRGBBack(cell.Attributes); fg != 0x123456 || bg != 0x654321 {
		t.Fatalf("custom text changed to %06X on %06X", fg, bg)
	}
}

func TestText_Truncation(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	// Width is 5 (0 to 4)
	txt := NewText(0, 0, "Longer than five", 0)
	txt.X2 = 4
	txt.SetVisible(true)

	txt.Show(scr)

	// Check that only "Longe" is written
	checkCell(t, scr, 0, 0, 'L', Palette[ColDialogText])
	checkCell(t, scr, 4, 0, 'e', Palette[ColDialogText])

	// Cell at X=5 should be empty (zero-char)
	if cell := scr.GetCell(5, 0); cell.Char != 0 {
		t.Errorf("Text overflow: expected empty cell at X=5, got %c", rune(cell.Char))
	}
}
