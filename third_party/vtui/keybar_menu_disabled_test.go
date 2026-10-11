package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// TestVMenu_ItemDisabled_Dims verifies that MenuItem.Disabled dims a row's
// colors relative to an otherwise identical, non-disabled row, the same way
// FrameManager.DisabledCommands already dims a Command-keyed item. This is
// the mechanism f4#1356 relies on for menu items built from an OnClick
// closure rather than a TV-style Command id.
func TestVMenu_ItemDisabled_Dims(t *testing.T) {
	SetDefaultPalette()

	m := NewVMenu("Menu")
	m.AddItem(MenuItem{Text: "First"})
	m.AddItem(MenuItem{Text: "Second", Disabled: true})
	m.AddItem(MenuItem{Text: "Third"})
	m.SetPosition(0, 0, 20, 5)
	// Select neither row under test, so both draw with the plain (unselected)
	// palette entry and the only difference left is Disabled.
	m.SetSelectPos(2)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(21, 6)
	m.Show(scr)

	// Items start one row below the top border; the label starts two columns
	// in (one for the border, one for the leading padding space).
	activeCell := scr.GetCell(2, 1)
	disabledCell := scr.GetCell(2, 2)
	if activeCell.Attributes == disabledCell.Attributes {
		t.Error("a Disabled menu item should be dimmed relative to an enabled one")
	}
}

// TestVMenu_ItemDisabled_BlocksActivation verifies that neither Enter nor a
// mouse click fires OnClick for a Disabled item, and that the input is still
// swallowed (no fall-through to whatever is behind the menu).
func TestVMenu_ItemDisabled_BlocksActivation(t *testing.T) {
	SetDefaultPalette()
	oldFm := FrameManager
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	clicked := false
	m := NewVMenu("Menu")
	m.AddItem(MenuItem{Text: "Active"})
	m.AddItem(MenuItem{Text: "Disabled", Disabled: true, OnClick: func() { clicked = true }})
	m.SetPosition(0, 0, 20, 5)
	m.SetSelectPos(1)

	if !m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Error("Enter on a disabled item should still be swallowed by the menu")
	}
	if clicked {
		t.Error("Enter on a disabled item must not fire OnClick")
	}

	// Clicking it with the mouse must not fire OnClick either. Item 1
	// ("Disabled") sits one row below item 0, right under the menu's top
	// border (see TestVMenu_MouseMoveSelectsItemWithoutActivatingIt for the
	// same m.Y1+1+index row mapping).
	y := m.Y1 + 1 + 1
	if idx := m.GetClickIndex(y); idx != 1 {
		t.Fatalf("test setup: expected click index 1 at row %d, got %d", y, idx)
	}
	m.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      int16(m.X1 + 2), //nolint:gosec // test-fixed small coordinate
		MouseY:      int16(y),        //nolint:gosec // test-fixed small coordinate
	})
	if clicked {
		t.Error("mouse click on a disabled item must not fire OnClick")
	}
}

// TestKeyBar_DisabledSlot_Dims verifies that a KeyBarDisabled slot dims both
// the F-number and the label text for the modifier row currently on screen,
// without affecting a neighboring enabled slot.
func TestKeyBar_DisabledSlot_Dims(t *testing.T) {
	SetDefaultPalette()

	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	kb.SetVisible(true)

	// F5 (index 4) stays enabled, F6 (index 5) is disabled -- mirrors
	// Copy/Move on the Shell keybar with no selection to act on.
	kb.Normal[4] = "Copy"
	kb.Normal[5] = "Move"
	kb.NormalDisabled[5] = true

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	kb.Show(scr)

	slotWidth := 80 / 12
	activeNumX := 0 + 4*slotWidth
	disabledNumX := 0 + 5*slotWidth

	activeNum := scr.GetCell(activeNumX, 24)
	disabledNum := scr.GetCell(disabledNumX, 24)
	if activeNum.Attributes == disabledNum.Attributes {
		t.Error("a disabled keybar slot's number should be dimmed relative to an enabled one")
	}

	activeLabel := scr.GetCell(activeNumX+1, 24)
	disabledLabel := scr.GetCell(disabledNumX+1, 24)
	if activeLabel.Attributes == disabledLabel.Attributes {
		t.Error("a disabled keybar slot's label should be dimmed relative to an enabled one")
	}
}

// TestKeyBar_DisabledSlot_FollowsModifierRow verifies that the Disabled mask
// applied is the one matching the modifier row currently latched, not always
// Normal's -- a Shift row dims independently of Normal.
func TestKeyBar_DisabledSlot_FollowsModifierRow(t *testing.T) {
	SetDefaultPalette()

	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	kb.SetVisible(true)

	kb.Normal[4] = "Copy"
	kb.Shift[4] = "CopyHere"
	kb.ShiftDisabled[4] = true
	kb.LatchModifiers(true, false, false)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	kb.Show(scr)

	slotWidth := 80 / 12
	numX := 0 + 4*slotWidth
	numCell := scr.GetCell(numX, 24)

	// Compare against the same slot drawn for the Normal row (not disabled)
	// to make sure the dimming really came from ShiftDisabled.
	kb.LatchModifiers(false, false, false)
	scr2 := NewSilentScreenBuf()
	scr2.AllocBuf(80, 25)
	kb.Show(scr2)
	normalCell := scr2.GetCell(numX, 24)

	if numCell.Attributes == normalCell.Attributes {
		t.Error("Shift row should be dimmed while Normal row (not disabled) should not")
	}
}
