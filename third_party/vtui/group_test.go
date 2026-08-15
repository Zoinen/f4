package vtui

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestGroup_FocusCycle(t *testing.T) {
	g := NewGroup(0, 0, 20, 10)
	g.WrapFocus = true
	b1 := NewButton(1, 1, "B1")
	txt := NewText(1, 2, "Static", 0) // Not focusable
	e1 := NewEdit(1, 3, 10, "E1")
	b2 := NewButton(1, 4, "B2")

	g.AddItem(b1)
	g.AddItem(txt)
	g.AddItem(e1)
	g.AddItem(b2)

	if g.focusIdx != 0 {
		t.Errorf("Initial focus: expected 0, got %d", g.focusIdx)
	}

	// 1. Tab -> skips txt (1), goes to e1 (index 2)
	g.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if g.focusIdx != 2 {
		t.Errorf("Tab 1: expected index 2 (e1), got %d", g.focusIdx)
	}
	if !e1.IsFocused() || b1.IsFocused() {
		t.Error("Focus state not updated correctly after Tab 1")
	}

	// 2. Tab -> goes to b2 (index 3)
	g.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if g.focusIdx != 3 {
		t.Errorf("Tab 2: expected index 3 (b2), got %d", g.focusIdx)
	}

	// 3. Tab -> cycles back to b1 (index 0)
	g.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if g.focusIdx != 0 {
		t.Errorf("Tab 3 (cycle): expected index 0 (b1), got %d", g.focusIdx)
	}
}

func TestGroup_Nested(t *testing.T) {
	// Window -> GroupBox -> Button
	dlg := NewDialog(0, 0, 40, 20, "Nested Test")
	outerEdit := NewEdit(1, 1, 10, "Outer")
	dlg.AddItem(outerEdit)

	gb := NewGroupBox(1, 3, 38, 10, "Inner Group")
	innerBtn1 := NewButton(gb.X1+1, gb.Y1+1, "&Inner1")
	innerBtn2 := NewButton(gb.X1+1, gb.Y1+2, "I&nner2")
	gb.AddItem(innerBtn1)
	gb.AddItem(innerBtn2)
	dlg.AddItem(gb)

	outerBtn := NewButton(1, 12, "Outer")
	dlg.AddItem(outerBtn)

	// Focus initially on outerEdit (index 0)
	if dlg.rootGroup.focusIdx != 0 {
		t.Fatalf("Initial focus should be on outerEdit, got %d", dlg.rootGroup.focusIdx)
	}

	// 1. Tab into the GroupBox. It's the next focusable element.
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if dlg.rootGroup.focusIdx != 1 { // GroupBox is item 1
		t.Fatalf("Tab 1: Focus should be on GroupBox. Got %d", dlg.rootGroup.focusIdx)
	}
	if !gb.IsFocused() {
		t.Fatal("GroupBox should be focused")
	}
	if gb.focusIdx != 0 || !innerBtn1.IsFocused() {
		t.Fatal("GroupBox did not pass focus to its first child (innerBtn1)")
	}

	// 2. Tab inside the GroupBox.
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if dlg.rootGroup.focusIdx != 1 { // Focus remains on GroupBox
		t.Fatal("Tab 2: Focus should remain on GroupBox.")
	}
	if gb.focusIdx != 1 || !innerBtn2.IsFocused() {
		t.Fatal("Tab 2: Focus did not move to innerBtn2.")
	}

	// 3. Tab out of the GroupBox.
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if dlg.rootGroup.focusIdx != 2 { // Focus moves to outerBtn (index 2)
		t.Fatalf("Tab 3: Focus should move to outerBtn. Got %d", dlg.rootGroup.focusIdx)
	}
	if !outerBtn.IsFocused() || gb.IsFocused() {
		t.Fatal("Tab 3: Focus state is incorrect.")
	}
	// 4. Test XLat hotkey: press 'т' (Russian 't' -> QWERTY 'n') to trigger Alt+N for innerBtn2
	GlobalXlator.Track('т') // Устанавливаем русский контекст
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'т',
		ControlKeyState: vtinput.LeftAltPressed,
	})
	if gb.focusIdx != 1 || !innerBtn2.IsFocused() {
		t.Error("XLat hotkey (Alt+т -> Alt+N) failed to focus button")
	}

	// 4. Test hotkey (Alt+I for innerBtn1)
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'i', ControlKeyState: vtinput.LeftAltPressed})
	if dlg.rootGroup.focusIdx != 1 || gb.focusIdx != 0 || !innerBtn1.IsFocused() {
		t.Fatal("Hotkey did not correctly focus nested button.")
	}
}

func TestGroup_FocusMemory(t *testing.T) {
	g := NewGroup(0, 0, 20, 10)
	b1 := NewButton(0, 0, "B1")
	b2 := NewButton(0, 1, "B2")
	g.AddItem(b1)
	g.AddItem(b2)

	// Focus b2
	g.changeFocus(1)
	g.changeFocus(1)

	if !b2.IsFocused() {
		t.Fatal("b2 should be focused")
	}

	// Lose focus
	g.SetFocus(false)
	if b2.IsFocused() {
		t.Error("b2 should lose focus when group loses focus")
	}

	// Regain focus
	g.SetFocus(true)
	if !b2.IsFocused() {
		t.Error("b2 should regain focus because group remembered it")
	}
}
func TestGroup_NoFocusableItems(t *testing.T) {
	g := NewGroup(0, 0, 10, 10)
	g.AddItem(NewText(1, 1, "Static Text", 0))

	done := make(chan bool, 1)
	go func() {
		g.changeFocus(1)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Deadlock in changeFocus with no focusable items")
	}

	if g.focusIdx != -1 {
		t.Errorf("Expected focusIdx to be -1, got %d", g.focusIdx)
	}
}

func TestGroup_SetFocusedItem(t *testing.T) {
	g := NewGroup(0, 0, 20, 10)
	b1 := NewButton(0, 0, "B1")
	b2 := NewButton(0, 1, "B2")
	g.AddItem(b1)
	g.AddItem(b2)

	// Initial focus is on b1
	if g.GetFocusedItem() != b1 {
		t.Fatal("Initial focus fail")
	}

	// Set focus to b2
	g.SetFocusedItem(b2)
	if g.GetFocusedItem() != b2 {
		t.Error("SetFocusedItem failed to move focus to b2")
	}
	if !b2.IsFocused() || b1.IsFocused() {
		t.Error("Focus flags not updated after SetFocusedItem")
	}
}
func TestGroup_ActivateHotkey_FocusLinkRecursion(t *testing.T) {
	// Tests that hotkey on a Label focuses the linked element even if nested
	dlg := NewDialog(0, 0, 40, 10, "Hotkey Test")

	group := NewGroupBox(1, 1, 38, 8, "Inner")
	edit := NewEdit(2, 4, 10, "")
	label := NewLabel(2, 2, "&Target:", edit)

	group.AddItem(label)
	group.AddItem(edit)
	dlg.AddItem(group)

	// Focus initially elsewhere
	dummy := NewButton(1, 1, "Dummy")
	dlg.AddItem(dummy)
	dlg.SetFocusedItem(dummy)

	// Activate hotkey 't' (from &Target)
	dlg.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 't',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !edit.IsFocused() {
		t.Error("Hotkey failed to focus linked element through nested group")
	}
	if !group.IsFocused() {
		t.Error("Parent group of the focused element should also be marked as focused")
	}
}
func TestGroup_FocusAllDisabled(t *testing.T) {
	// Verifies that group doesn't enter infinite loop or crash if no items can be focused
	g := NewGroup(0, 0, 10, 10)
	b1 := NewButton(1, 1, "B1")
	b1.SetDisabled(true)
	b2 := NewButton(1, 2, "B2")
	b2.SetDisabled(true)

	g.AddItem(b1)
	g.AddItem(b2)

	// Should safely return false
	res := g.changeFocus(1)
	if res {
		t.Error("changeFocus should return false when no items are enabled")
	}
	if g.focusIdx != -1 {
		t.Errorf("focusIdx should be -1, got %d", g.focusIdx)
	}
}

func TestNestedGroupFocusCycle(t *testing.T) {
	SetDefaultPalette()

	// 1. Create a parent group (like dialog root)
	parent := NewGroup(0, 0, 40, 20)
	parent.WrapFocus = true

	// 2. Create nested group A
	groupA := NewGroup(2, 2, 10, 5)
	editA1 := NewEdit(0, 0, 5, "")
	editA2 := NewEdit(0, 0, 5, "")
	groupA.AddItem(editA1)
	groupA.AddItem(editA2)

	// 3. Create nested group B
	groupB := NewGroup(15, 2, 10, 5)
	editB1 := NewEdit(0, 0, 5, "")
	editB2 := NewEdit(0, 0, 5, "")
	groupB.AddItem(editB1)
	groupB.AddItem(editB2)

	parent.AddItem(groupA)
	parent.AddItem(groupB)

	// Set initial focus to parent
	parent.SetFocus(true)

	// Initially, editA1 should be focused
	if !editA1.IsFocused() {
		t.Errorf("Expected editA1 to be focused initially")
	}

	// Move forward with Tab
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if !editA2.IsFocused() {
		t.Errorf("Expected editA2 to be focused after first Tab")
	}

	// Move forward with Tab (enters groupB, should focus first element editB1)
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if !editB1.IsFocused() {
		t.Errorf("Expected editB1 to be focused after entering groupB")
	}

	// Move forward with Tab
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if !editB2.IsFocused() {
		t.Errorf("Expected editB2 to be focused inside groupB")
	}

	// Move forward with Tab (should wrap back to groupA, focusing editA1)
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if !editA1.IsFocused() {
		t.Errorf("Expected editA1 to be focused after wrapping back to groupA")
	}

	// Move backward with Shift+Tab (should wrap to groupB, focusing the last element editB2)
	parent.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed,
	})
	if !editB2.IsFocused() {
		t.Errorf("Expected editB2 to be focused after wrapping backward to groupB")
	}
}

func TestGroup_ProcessMouse_EmptySpacePropagation(t *testing.T) {
	SetDefaultPalette()

	// 1. Создаем родительское MDI окно
	win := NewWindow(10, 5, 50, 15, "Test Window")

	// 2. Добавляем декоративную группу GroupBox
	gb := NewGroupBox(0, 0, 30, 8, "Container Group")
	win.AddItem(gb)

	// 3. Добавляем внутрь нее интерактивную кнопку
	btn := NewButton(2, 2, "Btn")
	gb.AddItem(btn)

	// Настраиваем точные координаты элементов на экране
	gb.SetPosition(12, 7, 42, 13)
	btn.SetPosition(15, 9, 25, 9)

	// Тест 1: Клик по интерактивной кнопке должен быть поглощен ею самой
	evBtn := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      18,
		MouseY:      9,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	if !win.ProcessMouse(evBtn) {
		t.Error("Expected mouse click on button to be consumed")
	}
	if win.isDragging {
		t.Error("Click on button should not initiate window dragging")
	}
	win.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseX: 18, MouseY: 9,
		ButtonState: 0, KeyDown: false,
	})

	// Тест 2: Клик по пустому пространству GroupBox должен провалиться глубже
	// и инициировать перетаскивание родительского BaseWindow
	evEmpty := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      35,
		MouseY:      11,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	if !win.ProcessMouse(evEmpty) {
		t.Error("Expected click on empty space in GroupBox to propagate to window operations")
	}
	if !win.isDragging {
		t.Error("Expected empty space click to successfully trigger window dragging")
	}
}
func TestGroup_GetElementAt(t *testing.T) {
	g := NewGroup(0, 0, 50, 50)

	gb := NewGroupBox(5, 5, 30, 20, "Box")
	g.AddItem(gb)

	btn := NewButton(10, 10, "ClickMe")
	gb.AddItem(btn)

	// 1. Hit test exact button coordinates
	if el := g.GetElementAt(12, 10); el != btn {
		t.Errorf("Expected button at (12,10), got %T", el)
	}

	// 2. Hit test empty space in GroupBox
	if el := g.GetElementAt(6, 6); el != gb {
		t.Errorf("Expected GroupBox at (6,6), got %T", el)
	}

	// 3. Hit test outside
	if el := g.GetElementAt(1, 1); el != nil {
		t.Errorf("Expected nil at (1,1), got %T", el)
	}
}
func TestGroup_DoubleFocusRegression(t *testing.T) {
	SetDefaultPalette()

	parent := NewGroup(0, 0, 40, 20)
	parent.WrapFocus = true

	gb := NewGroupBox(2, 2, 20, 10, "Group")
	chk1 := NewCheckbox(0, 0, "C1", false)
	chk2 := NewCheckbox(0, 1, "C2", false)
	gb.AddItem(chk1)
	gb.AddItem(chk2)

	edit := NewEdit(2, 12, 10, "")

	parent.AddItem(gb)
	parent.AddItem(edit)

	// Set initial focus to parent
	parent.SetFocus(true)

	// Initially, parent focus should be on gb, and gb focus on chk1.
	// Since parent is focused, chk1 should be focused. chk2 should not.
	if !chk1.IsFocused() {
		t.Error("chk1 should be focused initially")
	}
	if chk2.IsFocused() {
		t.Error("chk2 should not be focused initially")
	}

	// Move forward to chk2 (Tab 1)
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if !chk2.IsFocused() {
		t.Error("chk2 should be focused after Tab 1")
	}
	if chk1.IsFocused() {
		t.Error("chk1 should lose focus after Tab 1")
	}

	// Move forward to edit (Tab 2) -> gb loses focus completely, so chk2 must lose focus too!
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if !edit.IsFocused() {
		t.Error("edit should be focused after Tab 2")
	}
	if chk2.IsFocused() {
		t.Error("chk2 should lose focus when parent focus moves to edit")
	}

	// Move focus backward with Up arrow (VK_UP) -> enters gb from the bottom, should focus chk2.
	// Both chk1 and chk2 must NOT be focused at the same time!
	parent.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if !chk2.IsFocused() {
		t.Error("chk2 should be focused when entering gb from the bottom")
	}
	if chk1.IsFocused() {
		t.Error("chk1 must NOT be focused when chk2 gets focused (double focus regression!)")
	}
}
