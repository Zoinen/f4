package vtui

import (
	"testing"
	"time"

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

	// 3. PgDn on a menu shorter than a page still lands on the last item
	m.SetPosition(0, 0, 20, 4) // ViewHeight 3 > 2 items
	m.SetSelectPos(0)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if m.SelectPos != 1 {
		t.Error("PgDn failed to reach the last item")
	}

	// 3. Left/Right in standalone menu should return false (boundary exit)
	if m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}) {
		t.Error("Left in standalone menu should return false")
	}
}

func TestVMenu_PageNavigation(t *testing.T) {
	key := func(vk uint16) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
	}

	m := NewVMenu("Page")
	for i := 0; i < 12; i++ {
		m.AddItem(MenuItem{Text: "Item"})
	}
	// 7 rows minus top/bottom margins -> ViewHeight = 5
	m.SetPosition(0, 0, 20, 6)
	if m.ViewHeight != 5 {
		t.Fatalf("test setup: expected ViewHeight 5, got %d", m.ViewHeight)
	}

	m.SetSelectPos(2)

	// PgDn advances selection and view by one page, keeping the cursor row
	if !m.ProcessKey(key(vtinput.VK_NEXT)) {
		t.Fatal("PgDn should be handled")
	}
	if m.SelectPos != 7 || m.TopPos != 5 {
		t.Fatalf("after PgDn expected SelectPos=7 TopPos=5, got %d/%d", m.SelectPos, m.TopPos)
	}

	// Second PgDn clamps at the last item; TopPos clamps at ItemCount-ViewHeight
	m.ProcessKey(key(vtinput.VK_NEXT))
	if m.SelectPos != 11 || m.TopPos != 7 {
		t.Fatalf("after 2nd PgDn expected SelectPos=11 TopPos=7, got %d/%d", m.SelectPos, m.TopPos)
	}

	// PgDn at the end must stay put, not wrap to the top (Wrap is on by default)
	m.ProcessKey(key(vtinput.VK_NEXT))
	if m.SelectPos != 11 || m.TopPos != 7 {
		t.Fatalf("PgDn at end must clamp, got SelectPos=%d TopPos=%d", m.SelectPos, m.TopPos)
	}

	// PgUp mirrors the paging back...
	m.ProcessKey(key(vtinput.VK_PRIOR))
	if m.SelectPos != 6 || m.TopPos != 2 {
		t.Fatalf("after PgUp expected SelectPos=6 TopPos=2, got %d/%d", m.SelectPos, m.TopPos)
	}
	m.ProcessKey(key(vtinput.VK_PRIOR))
	if m.SelectPos != 1 || m.TopPos != 0 {
		t.Fatalf("after 2nd PgUp expected SelectPos=1 TopPos=0, got %d/%d", m.SelectPos, m.TopPos)
	}

	// ...and clamps at item 0 without wrapping to the end
	m.ProcessKey(key(vtinput.VK_PRIOR))
	if m.SelectPos != 0 || m.TopPos != 0 {
		t.Fatalf("PgUp at start must clamp, got SelectPos=%d TopPos=%d", m.SelectPos, m.TopPos)
	}

	// Virtual list: ItemCount set directly with no Items populated (f4-style
	// Find All frame). Paging must not touch len(m.Items).
	v := NewVMenu("Virtual")
	v.ItemCount = 40
	v.SetPosition(0, 0, 20, 6)
	v.SetSelectPos(0)
	v.ProcessKey(key(vtinput.VK_NEXT))
	if v.SelectPos != 5 || v.TopPos != 5 {
		t.Fatalf("virtual PgDn expected SelectPos=5 TopPos=5, got %d/%d", v.SelectPos, v.TopPos)
	}
	v.ProcessKey(key(vtinput.VK_PRIOR))
	if v.SelectPos != 0 || v.TopPos != 0 {
		t.Fatalf("virtual PgUp expected SelectPos=0 TopPos=0, got %d/%d", v.SelectPos, v.TopPos)
	}
}

func TestVMenu_PageSkipsSeparator(t *testing.T) {
	m := NewVMenu("Sep")
	for i := 0; i < 5; i++ {
		m.AddItem(MenuItem{Text: "Item"})
	}
	m.AddSeparator() // index 5
	for i := 0; i < 6; i++ {
		m.AddItem(MenuItem{Text: "Item"})
	}
	m.SetPosition(0, 0, 20, 6) // ViewHeight 5
	m.SetSelectPos(0)

	// PgDn lands on the separator at index 5 and must step past it
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if m.SelectPos != 6 {
		t.Fatalf("PgDn must skip the separator, expected SelectPos=6, got %d", m.SelectPos)
	}

	// PgUp from 6 lands on index 1; on the way back the separator is not hit,
	// but paging up from 6+... exercise the reverse nudge too: from index 10,
	// PgUp lands on 5 (separator) and must continue upward to 4.
	m.SetSelectPos(10)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
	if m.SelectPos != 4 {
		t.Fatalf("PgUp must skip the separator, expected SelectPos=4, got %d", m.SelectPos)
	}
}

func TestVMenu_WheelAtEdgeClampsInsteadOfWrapping(t *testing.T) {
	m := NewVMenu("Wheel")
	for i := 0; i < 12; i++ {
		m.AddItem(MenuItem{Text: "Item"})
	}
	m.SetPosition(0, 0, 20, 6) // ViewHeight 5
	m.SetSelectPos(11)         // bottom: TopPos 7

	handled := m.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	if !handled {
		t.Fatal("wheel scroll should be handled")
	}
	if m.SelectPos != 11 || m.TopPos != 7 {
		t.Fatalf("wheel down at bottom must clamp, not wrap to top: SelectPos=%d TopPos=%d", m.SelectPos, m.TopPos)
	}

	m.SetSelectPos(0)
	m.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	if m.SelectPos != 0 || m.TopPos != 0 {
		t.Fatalf("wheel up at top must clamp, not wrap to bottom: SelectPos=%d TopPos=%d", m.SelectPos, m.TopPos)
	}
}

func TestVMenu_VirtualListEnterAndHoverDoNotPanic(t *testing.T) {
	m := NewVMenu("Virtual")
	m.ItemCount = 40 // no Items populated
	m.SetPosition(0, 0, 20, 6)
	m.SetSelectPos(3)

	confirmed := -1
	m.OnAction = func(i int) { confirmed = i }

	// Enter on a virtual row: no item command to fire, but the selection is
	// confirmed and the menu closes with it.
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if confirmed != 3 {
		t.Fatalf("Enter on a virtual row should confirm via OnAction, got %d", confirmed)
	}
	if !m.IsDone() || m.exitCode != 3 {
		t.Fatalf("Enter on a virtual row should close with its index, done=%v exit=%d", m.IsDone(), m.exitCode)
	}

	// Hover over a virtual row selects it
	m.ClearDone()
	handled := m.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          5,
		MouseY:          2, // row 1 of content -> index TopPos+1
		MouseEventFlags: vtinput.MouseMoved,
	})
	if !handled {
		t.Fatal("hover over a virtual row should be handled")
	}
	if m.SelectPos != m.TopPos+1 {
		t.Fatalf("hover should select the virtual row, expected %d, got %d", m.TopPos+1, m.SelectPos)
	}

	// Click on a virtual row confirms it
	confirmed = -1
	clickIdx := m.TopPos + 2
	m.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      5,
		MouseY:      3,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	})
	if confirmed != clickIdx {
		t.Fatalf("click on a virtual row should confirm via OnAction, expected %d, got %d", clickIdx, confirmed)
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

func TestVMenuNestedKeyboardOpenCloseAndEdgePlacement(t *testing.T) {
	previous := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 12)
	fm.Init(scr)
	FrameManager = fm
	defer func() { FrameManager = previous }()
	fm.Push(NewDesktop())

	root := NewVMenu("Root")
	root.SetPosition(30, 8, 39, 11)
	root.AddItem(MenuItem{
		ID: "locations", Text: "Locations",
		Submenu: func() *VMenu {
			child := NewVMenu("Child")
			child.AddItem(MenuItem{ID: "header", Text: "Favorites", Header: true})
			child.AddItem(MenuItem{ID: "home", Text: "Home"})
			return child
		},
	})
	fm.PushMenu(root)

	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_RIGHT,
	}, false)
	if len(fm.frames) != 3 {
		t.Fatalf("frame stack after opening child = %d, want 3", len(fm.frames))
	}
	child, ok := fm.GetTopFrame().(*VMenu)
	if !ok || child.ParentMenu() != root || child.ParentIndex() != 0 {
		t.Fatalf("nested relationship was not retained: %#v", fm.GetTopFrame())
	}
	if child.SelectPos != 1 {
		t.Fatalf("nonselectable header received focus; selected = %d", child.SelectPos)
	}
	x1, y1, x2, y2 := child.GetPosition()
	if x1 < 0 || y1 < 0 || x2 >= 40 || y2 >= 12 || x1 >= root.X1 {
		t.Fatalf("edge-flipped child position = (%d,%d)-(%d,%d)", x1, y1, x2, y2)
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_LEFT,
	}, false)
	if fm.GetTopFrame() != root || root.childMenu != nil {
		t.Fatalf("Left did not restore the parent menu; top=%T child=%p",
			fm.GetTopFrame(), root.childMenu)
	}
}

func TestVMenuReplaceItemsPreservesStableSelection(t *testing.T) {
	menu := NewVMenu("Dynamic")
	menu.ReplaceItems([]MenuItem{
		{ID: "header", Text: "Locations", Header: true},
		{ID: "home", Text: "Home"},
		{ID: "volume", Text: "Volume"},
	})
	menu.SetSelectPos(2)
	menu.TopPos = 1
	menu.ReplaceItems([]MenuItem{
		{ID: "header", Text: "Locations", Header: true},
		{ID: "cloud", Text: "Cloud"},
		{ID: "volume", Text: "Renamed volume"},
		{ID: "home", Text: "Home"},
	})
	if menu.SelectPos != 2 || menu.Items[menu.SelectPos].ID != "volume" {
		t.Fatalf("selection was not preserved by ID: pos=%d item=%#v",
			menu.SelectPos, menu.Items[menu.SelectPos])
	}
	menu.ReplaceItems([]MenuItem{{ID: "header", Text: "Tags", Header: true},
		{ID: "all", Text: "All Tags"}})
	if menu.SelectPos != 1 {
		t.Fatalf("missing selection did not fall back to first selectable row: %d",
			menu.SelectPos)
	}
}

func TestVMenuHoverOpensSubmenuAfterDelay(t *testing.T) {
	previous := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	defer func() { FrameManager = previous }()
	fm.Push(NewDesktop())

	root := NewVMenu("Root")
	root.SetPosition(10, 5, 30, 9)
	root.AddItem(MenuItem{
		ID: "locations", Text: "Locations",
		Submenu: func() *VMenu {
			child := NewVMenu("Child")
			child.AddItem(MenuItem{ID: "home", Text: "Home"})
			return child
		},
	})
	fm.PushMenu(root)

	if !root.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          15,
		MouseY:          6,
		MouseEventFlags: vtinput.MouseMoved,
	}) {
		t.Fatal("hover over submenu row was not handled")
	}

	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(time.Second):
		t.Fatal("submenu hover did not post an opening task")
	}
	if root.childMenu == nil || fm.GetTopFrame() != root.childMenu {
		t.Fatalf("hover did not open the anchored child: child=%p top=%T",
			root.childMenu, fm.GetTopFrame())
	}
	root.CloseChain()
}

func TestVMenuReplaceItemsRetainsOpenChildByStableID(t *testing.T) {
	previous := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	defer func() { FrameManager = previous }()
	fm.Push(NewDesktop())

	submenuFactory := func() *VMenu {
		child := NewVMenu("Child")
		child.AddItem(MenuItem{ID: "home", Text: "Home"})
		return child
	}
	root := NewVMenu("Root")
	root.SetPosition(10, 4, 30, 12)
	root.ReplaceItems([]MenuItem{
		{ID: "first", Text: "First"},
		{ID: "locations", Text: "Locations", Submenu: submenuFactory},
	})
	fm.PushMenu(root)
	if !root.OpenSubmenu(1) {
		t.Fatal("failed to open initial child")
	}
	child := root.childMenu
	_, oldY, _, _ := child.GetPosition()

	root.ReplaceItems([]MenuItem{
		{ID: "header", Text: "Places", Header: true},
		{ID: "first", Text: "First"},
		{ID: "locations", Text: "Locations", Submenu: submenuFactory},
	})
	_, newY, _, _ := child.GetPosition()
	if root.childMenu != child || fm.GetTopFrame() != child {
		t.Fatal("asynchronous replacement discarded the open child")
	}
	if root.childIndex != 2 || child.ParentIndex() != 2 {
		t.Fatalf("child anchor after replacement = root %d / child %d, want 2",
			root.childIndex, child.ParentIndex())
	}
	if newY != oldY+1 {
		t.Fatalf("child was not repositioned with its stable row: y %d -> %d",
			oldY, newY)
	}
	root.CloseChain()
}

func TestVMenuOutsideClickClosesNestedChain(t *testing.T) {
	previous := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	defer func() { FrameManager = previous }()
	fm.Push(NewDesktop())

	root := NewVMenu("Root")
	root.SetPosition(10, 5, 30, 12)
	root.AddItem(MenuItem{
		ID: "locations", Text: "Locations",
		Submenu: func() *VMenu {
			child := NewVMenu("Child")
			child.AddItem(MenuItem{ID: "home", Text: "Home"})
			return child
		},
	})
	fm.PushMenu(root)
	if !root.OpenSubmenu(0) {
		t.Fatal("failed to open nested menu")
	}
	child := root.childMenu

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      2,
		MouseY:      2,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)
	if !root.IsDone() || !child.IsDone() {
		t.Fatalf("outside click left menu chain open: root=%v child=%v",
			root.IsDone(), child.IsDone())
	}
	if root.childMenu != nil {
		t.Fatal("outside click retained the nested relationship")
	}
}

func TestVMenuCloseChainNotifiesEachMenuExactlyOnce(t *testing.T) {
	previous := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	defer func() { FrameManager = previous }()
	fm.Push(NewDesktop())

	rootClosed := 0
	childClosed := 0
	root := NewVMenu("Root")
	root.OnClose = func() { rootClosed++ }
	root.AddItem(MenuItem{
		ID: "dynamic", Text: "Dynamic",
		Submenu: func() *VMenu {
			child := NewVMenu("Child")
			child.OnClose = func() { childClosed++ }
			child.AddItem(MenuItem{ID: "loading", Text: "Loading", Disabled: true})
			return child
		},
	})
	fm.PushMenu(root)
	if !root.OpenSubmenu(0) {
		t.Fatal("failed to open child")
	}
	root.CloseChain()
	root.CloseChain()
	if rootClosed != 1 || childClosed != 1 {
		t.Fatalf("close callbacks = root %d, child %d; want one each",
			rootClosed, childClosed)
	}
}
