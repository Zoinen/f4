package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func newSubMenuTestBar(t *testing.T, width, height int, items []MenuItem) *MenuBar {
	t.Helper()

	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(width, height)
	fm.Init(scr)
	fm.Push(NewDesktop())

	old := FrameManager
	FrameManager = fm
	t.Cleanup(func() { FrameManager = old })

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{{Label: "Commands", SubItems: items}}
	mb.SetPosition(0, 0, width-1, 0)
	mb.Active = true
	return mb
}

func key(vk uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
}

func TestVMenuOpensAndClosesNestedMenu(t *testing.T) {
	mb := newSubMenuTestBar(t, 80, 25, []MenuItem{
		{Text: "Find file"},
		{Text: "History", SubItems: []MenuItem{
			{Text: "Command history"},
			{Text: "Folders history"},
		}},
	})
	mb.ActivateSubMenu(0)

	dropdown, ok := mb.activeSubMenu.(*VMenu)
	if !ok {
		t.Fatal("menu bar did not open a VMenu")
	}
	dropdown.SetSelectPos(1)

	if !dropdown.ProcessKey(key(vtinput.VK_RIGHT)) {
		t.Fatal("Right on a submenu heading was not handled")
	}
	nested := dropdown.activeSub
	if nested == nil {
		t.Fatal("Right did not open the nested menu")
	}
	if FrameManager.GetTopFrame() != Frame(nested) {
		t.Fatal("the nested menu is not on top of the frame stack")
	}
	if got := nested.GetTitle(); got != "History" {
		t.Fatalf("nested menu title = %q, want History", got)
	}
	if nested.GetItemCount() != 2 {
		t.Fatalf("nested menu holds %d items, want 2", nested.GetItemCount())
	}

	if !nested.ProcessKey(key(vtinput.VK_LEFT)) {
		t.Fatal("Left in a nested menu was not handled")
	}
	if dropdown.activeSub != nil || !nested.IsDone() {
		t.Error("Left did not close the nested menu")
	}
	if dropdown.IsDone() {
		t.Error("Left closed the parent menu as well, it should step back one level only")
	}
	if !mb.Active {
		t.Error("Left in a nested menu deactivated the whole menu bar")
	}
}

func TestVMenuNestedEscapeClosesOneLevel(t *testing.T) {
	mb := newSubMenuTestBar(t, 80, 25, []MenuItem{
		{Text: "History", SubItems: []MenuItem{{Text: "Command history"}}},
	})
	mb.ActivateSubMenu(0)
	dropdown := mb.activeSubMenu.(*VMenu)

	dropdown.ProcessKey(key(vtinput.VK_RETURN))
	nested := dropdown.activeSub
	if nested == nil {
		t.Fatal("Enter on a submenu heading did not open the nested menu")
	}

	nested.ProcessKey(key(vtinput.VK_ESCAPE))
	if !nested.IsDone() || dropdown.activeSub != nil {
		t.Error("Esc did not close the nested menu")
	}
	if dropdown.IsDone() {
		t.Error("Esc in a nested menu closed the parent dropdown too")
	}
}

func TestVMenuNestedActionClosesWholeChain(t *testing.T) {
	ran := 0
	mb := newSubMenuTestBar(t, 80, 25, []MenuItem{
		{Text: "History", SubItems: []MenuItem{
			{Text: "Command history", OnClick: func() { ran++ }},
		}},
	})
	mb.ActivateSubMenu(0)
	dropdown := mb.activeSubMenu.(*VMenu)

	dropdown.ProcessKey(key(vtinput.VK_RIGHT))
	nested := dropdown.activeSub
	if nested == nil {
		t.Fatal("Right did not open the nested menu")
	}

	nested.ProcessKey(key(vtinput.VK_RETURN))
	if ran != 1 {
		t.Fatalf("nested item ran %d times, want once", ran)
	}
	if !nested.IsDone() || !dropdown.IsDone() {
		t.Error("choosing a nested item left a menu open")
	}
	if mb.Active {
		t.Error("choosing a nested item did not deactivate the menu bar")
	}
	for _, frame := range FrameManager.GetActiveFrames(0) {
		if frame == Frame(dropdown) {
			t.Error("the parent dropdown is still on the frame stack")
		}
	}
}

func TestVMenuNestedMenuStaysOnScreen(t *testing.T) {
	nestedItems := []MenuItem{
		{Text: "Copy path"},
		{Text: "Copy name"},
		{Text: "Copy selected names"},
	}
	mb := newSubMenuTestBar(t, 40, 8, []MenuItem{
		{Text: "Copy path or name", SubItems: nestedItems},
	})
	mb.ActivateSubMenu(0)
	dropdown := mb.activeSubMenu.(*VMenu)
	dropdown.ProcessKey(key(vtinput.VK_RIGHT))

	nested := dropdown.activeSub
	if nested == nil {
		t.Fatal("Right did not open the nested menu")
	}
	x1, y1, x2, y2 := nested.GetPosition()
	if x1 < 0 || y1 < 0 || x2 > 39 || y2 > 7 {
		t.Errorf("nested menu at (%d,%d)-(%d,%d) hangs off a 40x8 screen", x1, y1, x2, y2)
	}
	if x1 >= dropdown.X2 {
		t.Errorf("nested menu opened at x=%d, want it folded back left of the parent ending at %d", x1, dropdown.X2)
	}
}

func TestMenuItemsWidthCountsSubMenuMarker(t *testing.T) {
	plain := menuItemsWidth([]MenuItem{{Text: "History"}}, 0)
	nested := menuItemsWidth([]MenuItem{{Text: "History", SubItems: []MenuItem{{Text: "Command history"}}}}, 0)
	if nested <= plain {
		t.Errorf("width with the submenu marker = %d, want more than %d", nested, plain)
	}
}
