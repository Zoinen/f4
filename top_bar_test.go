package main

import (
	"github.com/unxed/vtui"
	"testing"
)

func TestTopBar_Show(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 5)

	// 1. Valid TopBar with value
	tb := NewTopBar(func() string {
		return "My Test Status"
	}, nil)
	tb.SetPosition(0, 0, 39, 0)
	tb.SetVisible(true)

	tb.Show(scr)

	// Verify that the background is filled and text is written
	attr := vtui.Palette[ColViewerStatus]
	for x := 0; x < 40; x++ {
		cell := scr.GetCell(x, 0)
		if cell.Attributes != attr {
			t.Errorf("Expected cell at x=%d to have attribute %016X, got %016X", x, attr, cell.Attributes)
		}
	}

	// Verify the text was printed starting at X1 (0)
	expectedText := "My Test Status"
	for i, r := range expectedText {
		cell := scr.GetCell(i, 0)
		if cell.Char != uint64(r) {
			t.Errorf("Expected char %q at x=%d, got %q", r, i, rune(cell.Char))
		}
	}
}

func TestTopBar_AttributeCallback(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 5)

	tb := NewTopBar(func() string { return "picked" }, nil)
	tb.SetPosition(0, 0, 39, 0)
	tb.SetVisible(true)

	tb.GetAttr = func() uint64 { return imageTilePickedAttr }
	tb.Show(scr)
	if got := scr.GetCell(0, 0).Attributes; got != imageTilePickedAttr {
		t.Errorf("the callback colour must win, got %016X", got)
	}

	// A callback with nothing to say leaves the palette in charge.
	tb.GetAttr = func() uint64 { return 0 }
	tb.Show(scr)
	if got, want := scr.GetCell(0, 0).Attributes, vtui.Palette[ColViewerStatus]; got != want {
		t.Errorf("expected the palette colour %016X, got %016X", want, got)
	}
}

func TestTopBar_NilCallbackAndInvisible(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 5)

	// 1. Nil callback - should not panic and should not write anything
	tbNil := NewTopBar(nil, nil)
	tbNil.SetPosition(0, 0, 39, 0)
	tbNil.SetVisible(true)

	tbNil.Show(scr) // should be a no-op except for parent Bar logic

	// 2. Invisible TopBar - should not write anything
	tbInvisible := NewTopBar(func() string {
		return "Should Not Be Seen"
	}, nil)
	tbInvisible.SetPosition(0, 0, 39, 0)
	tbInvisible.SetVisible(false)

	tbInvisible.Show(scr) // should be a no-op entirely
}
func TestTopBar_LeftRightAlignment(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 5)

	tb := NewTopBar(
		func() string { return "LeftPart" },
		func() string { return "RightPart" },
	)
	tb.SetPosition(0, 0, 39, 0)
	tb.SetVisible(true)

	tb.Show(scr)

	// Verify LeftPart is on the left
	leftText := "LeftPart"
	for i, r := range leftText {
		cell := scr.GetCell(i, 0)
		if cell.Char != uint64(r) {
			t.Errorf("Expected char %q at x=%d, got %q", r, i, rune(cell.Char))
		}
	}

	// Verify RightPart is on the right
	rightText := "RightPart"
	rightStart := 40 - len(rightText)
	for i, r := range rightText {
		cell := scr.GetCell(rightStart+i, 0)
		if cell.Char != uint64(r) {
			t.Errorf("Expected char %q at x=%d, got %q", r, rightStart+i, rune(cell.Char))
		}
	}
}

func TestTopBar_Truncation(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(20, 5) // Narrow screen

	tb := NewTopBar(
		func() string { return "VeryLongLeftPartName" },
		func() string { return "Right" },
	)
	tb.SetPosition(0, 0, 19, 0)
	tb.SetVisible(true)

	tb.Show(scr)

	// Combined length (20 + 5 = 25) exceeds width (20).
	// Right part ("Right" - 5 chars) should be preserved, and Left part should be truncated with "…".
	// Left part should become: runewidth.Truncate("VeryLongLeftPartName", 20 - 5 - 1 = 14, "…") -> "VeryLongLeftP…"
	expectedLeft := "VeryLongLeftP…"
	for i, r := range expectedLeft {
		cell := scr.GetCell(i, 0)
		if cell.Char != uint64(r) {
			t.Errorf("Expected truncated char %q at x=%d, got %q", r, i, rune(cell.Char))
		}
	}

	expectedRight := "Right"
	rightStart := 20 - len(expectedRight)
	for i, r := range expectedRight {
		cell := scr.GetCell(rightStart+i, 0)
		if cell.Char != uint64(r) {
			t.Errorf("Expected right char %q at x=%d, got %q", r, rightStart+i, rune(cell.Char))
		}
	}
}
