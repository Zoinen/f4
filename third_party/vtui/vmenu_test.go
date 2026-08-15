package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestVMenu_BoundaryNavigation(t *testing.T) {
	m := NewVMenu("Standalone")
	m.AddItem(MenuItem{Text: "1"})
	m.AddItem(MenuItem{Text: "2"})

	// 1. Default (Wrap=true): Up at top should WRAP, returning true
	m.SetSelectPos(0)
	if !m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("Up at index 0 should wrap and return true by default")
	}
	if m.SelectPos != 1 {
		t.Errorf("Expected wrap to 1, got %d", m.SelectPos)
	}

	// 2. Disable Wrap: Up at top should return false (exit focus)
	m.Wrap = false
	m.SetSelectPos(0)
	if m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("Up at index 0 should return false when Wrap=false")
	}

	// 3. Test PgUp/PgDn jumps
	m.SetSelectPos(0)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if m.SelectPos != 1 {
		t.Error("PgDn failed to jump to end")
	}

	// 3. Left/Right in standalone menu should return false (boundary exit)
	if m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}) {
		t.Error("Left in standalone menu should return false")
	}
}

func TestVMenu_FocusVisualization(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)
	m := NewVMenu("Menu")
	m.SetPosition(0, 0, 10, 4)

	// 1. Inactive state
	m.SetFocus(false)
	m.Show(scr)
	// Title " Menu " should use ColMenuTitle
	checkCell(t, scr, 3, 0, 'M', Palette[ColMenuTitle])

	// 2. Focused state: the title stays on ColMenuTitle, as in far2l
	m.SetFocus(true)
	m.Show(scr)
	checkCell(t, scr, 3, 0, 'M', Palette[ColMenuTitle])
}

func TestVMenu_OnKeyDownHook(t *testing.T) {
	m := NewVMenu("Hook Test")
	m.AddItem(MenuItem{Text: "Item 1"})

	m.AddItem(MenuItem{Text: "Item 2"})

	hookCalled := false
	m.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if e.VirtualKeyCode == vtinput.VK_F5 {
			hookCalled = true
			return true // Swallowed
		}
		return false
	}

	// 1. Test intercepting key
	handled := m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5})
	if !handled || !hookCalled {
		t.Error("OnKeyDown hook was not called or did not swallow the event")
	}

	// 2. Test falling through for other keys
	handled = m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if !handled {
		t.Error("Standard navigation should still work if hook returns false")
	}
}

func TestVMenu_MouseMoveSelectsItemWithoutActivatingIt(t *testing.T) {
	m := NewVMenu("Menu")
	m.AddItem(MenuItem{Text: "First"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "Third"})
	m.SetPosition(10, 5, 30, 9)
	m.SetSelectPos(0)

	actions := 0
	m.OnAction = func(int) { actions++ }

	handled := m.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          15,
		MouseY:          8,
		MouseEventFlags: vtinput.MouseMoved,
	})
	if !handled {
		t.Fatal("mouse move over a menu item should be handled")
	}
	if m.SelectPos != 2 {
		t.Fatalf("expected hovered item 2 to be selected, got %d", m.SelectPos)
	}
	if actions != 0 || m.IsDone() {
		t.Fatal("hovering must not activate an item or close the menu")
	}
}

func TestVMenu_MouseMoveIgnoresSeparatorAndOutside(t *testing.T) {
	m := NewVMenu("Menu")
	m.AddItem(MenuItem{Text: "First"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "Third"})
	m.SetPosition(10, 5, 30, 9)
	m.SetSelectPos(2)

	separatorHandled := m.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          15,
		MouseY:          7,
		MouseEventFlags: vtinput.MouseMoved,
	})
	if !separatorHandled {
		t.Fatal("mouse move over a separator inside the menu should be consumed")
	}
	if m.SelectPos != 2 {
		t.Fatalf("separator must not become selected, got %d", m.SelectPos)
	}

	outsideHandled := m.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          9,
		MouseY:          6,
		MouseEventFlags: vtinput.MouseMoved,
	})
	if outsideHandled {
		t.Fatal("mouse move outside the menu content should not be handled")
	}
	if m.SelectPos != 2 {
		t.Fatalf("outside movement must not change selection, got %d", m.SelectPos)
	}
}
