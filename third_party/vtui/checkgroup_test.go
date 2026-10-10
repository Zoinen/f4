package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestCheckGroup_GetSetDataBitmask(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"A", "B", "C", "D"})
	cg.States[0] = true
	cg.States[2] = true

	got := cg.GetData()
	mask, ok := got.(uint32)
	if !ok {
		t.Fatalf("GetData returned %T, want uint32", got)
	}
	if mask != 0b0101 {
		t.Errorf("GetData() = %b, want %b", mask, 0b0101)
	}

	cg.SetData(uint32(0b1010))
	want := []bool{false, true, false, true}
	for i, w := range want {
		if cg.States[i] != w {
			t.Errorf("State[%d] = %v, want %v", i, cg.States[i], w)
		}
	}

	// SetData with the wrong type must be a no-op rather than panic.
	before := append([]bool(nil), cg.States...)
	cg.SetData("not a mask")
	for i := range before {
		if cg.States[i] != before[i] {
			t.Errorf("SetData with wrong type mutated State[%d]", i)
		}
	}
}

func TestCheckGroup_ColumnsDefaultsToOne(t *testing.T) {
	cg := NewCheckGroup(0, 0, 0, []string{"A"})
	if cg.Columns != 1 {
		t.Errorf("Columns = %d, want 1 for cols<1 input", cg.Columns)
	}
}

func TestCheckGroup_ProcessKey_IgnoresKeyUpAndDisabled(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"A", "B"})
	if cg.ProcessKey(&vtinput.InputEvent{KeyDown: false, VirtualKeyCode: vtinput.VK_SPACE}) {
		t.Error("key-up event should not be handled")
	}
	cg.SetDisabled(true)
	if cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE}) {
		t.Error("disabled group should not handle keys")
	}
}

func TestCheckGroup_ProcessKey_SpaceTogglesFocusedItem(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"A", "B"})
	if cg.States[0] {
		t.Fatal("precondition: item 0 should start unchecked")
	}
	handled := cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if !handled {
		t.Fatal("space should be handled")
	}
	if !cg.States[0] {
		t.Error("space should toggle the focused item on")
	}
	cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if cg.States[0] {
		t.Error("second space should toggle the focused item back off")
	}
}

func TestCheckGroup_ProcessKey_GridNavMovesFocus(t *testing.T) {
	cg := NewCheckGroup(0, 0, 2, []string{"A", "B", "C", "D"})
	if !cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) {
		t.Fatal("right arrow should move focus within the grid")
	}
	// Toggling now should affect item 1 (moved focus), not item 0.
	cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if cg.States[1] != true || cg.States[0] != false {
		t.Errorf("expected only item 1 toggled, got States=%v", cg.States)
	}
}

func TestCheckGroup_ProcessKey_BoundaryNavExitsGroup(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"A", "B"})
	// Focus starts at index 0; moving Up at the top boundary must be
	// reported as unhandled so it can bubble out to the enclosing dialog.
	if cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("Up at the first item should not be swallowed by the group")
	}
}

func TestCheckGroup_ProcessKey_HotkeyTogglesMatchingItem(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"&Alpha", "&Beta"})
	handled := cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, Char: 'b'})
	if !handled {
		t.Fatal("matching hotkey should be handled")
	}
	if !cg.States[1] {
		t.Error("hotkey 'b' should toggle 'Beta' on")
	}
	if cg.focusIdx != 1 {
		t.Errorf("hotkey should move focus to the matched item, got focusIdx=%d", cg.focusIdx)
	}

	// A character that matches no hotkey must be reported as unhandled.
	handled = cg.ProcessKey(&vtinput.InputEvent{KeyDown: true, Char: 'z'})
	if handled {
		t.Error("non-matching character should not be handled")
	}
}

func TestCheckGroup_ProcessMouse_DisabledIgnoresClicks(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"Alpha"})
	cg.SetDisabled(true)
	handled := cg.ProcessMouse(&vtinput.InputEvent{
		ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: true,
		MouseX: 0, MouseY: 0,
	})
	if handled {
		t.Error("disabled group should ignore mouse clicks")
	}
}

func TestCheckGroup_ProcessMouse_OutOfBoundsIgnored(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"Alpha"})
	handled := cg.ProcessMouse(&vtinput.InputEvent{
		ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: true,
		MouseX: 500, MouseY: 500,
	})
	if handled {
		t.Error("out-of-bounds click should not be handled")
	}
}

func TestCheckGroup_ProcessMouse_ClickTogglesItemAndMovesFocus(t *testing.T) {
	cg := NewCheckGroup(0, 0, 2, []string{"Alpha", "Beta", "Gamma"})
	// Column 1 (Beta), row 0: x is somewhere inside colWidths[0]..colWidths[0]+colWidths[1]-1.
	x := cg.X1 + cg.colWidths[0]
	handled := cg.ProcessMouse(&vtinput.InputEvent{
		ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: true,
		MouseX: int16(x), MouseY: int16(cg.Y1), // #nosec G115 -- test-fixture coordinates, always well within int16
	})
	if !handled {
		t.Fatal("click on item 1 should be handled")
	}
	if !cg.States[1] {
		t.Error("click should toggle item 1 on")
	}
	if cg.focusIdx != 1 {
		t.Errorf("click should move focus to item 1, got focusIdx=%d", cg.focusIdx)
	}

	// A click that only presses without KeyDown (button release) must not toggle.
	handled = cg.ProcessMouse(&vtinput.InputEvent{
		ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: false,
		MouseX: int16(x), MouseY: int16(cg.Y1), // #nosec G115 -- test-fixture coordinates, always well within int16
	})
	if handled {
		t.Error("button-up event should not be handled")
	}
}
