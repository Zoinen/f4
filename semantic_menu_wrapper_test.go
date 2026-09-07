package main

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type semanticMenuControlTestFrame struct {
	*vtui.VMenu
	returnActivations int
	selectedOnReturn  int
	inputSource       string
}

func (f *semanticMenuControlTestFrame) ProcessKey(event *vtinput.InputEvent) bool {
	if event != nil && event.Type == vtinput.KeyEventType && event.KeyDown &&
		event.VirtualKeyCode == vtinput.VK_RETURN {
		f.returnActivations++
		f.selectedOnReturn = f.SelectPos
		f.inputSource = event.InputSource
		return true
	}
	return f.VMenu.ProcessKey(event)
}

func TestAppFrameVMenuUsesSharedMenuControlProvider(t *testing.T) {
	control := vtui.NewVMenu("Wrapped")
	control.AddItem(vtui.MenuItem{
		ID:   "nested",
		Text: "Nested",
		Submenu: func() *vtui.VMenu {
			return vtui.NewVMenu("Child")
		},
	})
	frame := &semanticMenuControlTestFrame{VMenu: control}

	got, bottomHint := appFrameVMenu(frame)
	if got != control || bottomHint != "" {
		t.Fatalf("appFrameVMenu(wrapper) = (%p, %q), want (%p, empty)",
			got, bottomHint, control)
	}

	model := (appVMenu{frame: frame, menu: got}).model().ToMap()
	items := appMapSlice(model["items"])
	if len(items) != 1 || items[0]["hasSubmenu"] != true {
		t.Fatalf("wrapped menu submenu model = %#v, want hasSubmenu", items)
	}
}

func TestAppVMenuModelUsesActualParentFrameIdentity(t *testing.T) {
	oldFrameManager := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFrameManager }()

	screen := vtui.NewScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)

	root := vtui.NewVMenu("Root")
	parentControl := vtui.NewVMenu("Parent")
	parentFrame := &semanticMenuControlTestFrame{VMenu: parentControl}
	childControl := vtui.NewVMenu("Child")
	childFrame := &semanticMenuControlTestFrame{VMenu: childControl}
	parentControl.AddItem(vtui.MenuItem{
		ID: "child", Text: "Child",
		SubmenuFrame: func() vtui.Frame { return childFrame },
	})
	root.AddItem(vtui.MenuItem{
		ID: "parent", Text: "Parent",
		SubmenuFrame: func() vtui.Frame { return parentFrame },
	})
	vtui.FrameManager.PushMenu(root)
	if !root.OpenSubmenu(0) || !parentControl.OpenSubmenu(0) {
		t.Fatal("custom nested menu frames did not open")
	}

	model := (appVMenu{frame: childFrame, menu: childControl}).model().ToMap()
	if got, want := model["parentId"], vtui.SemanticID(parentFrame); got != want {
		t.Fatalf("child parentId = %#v, want actual wrapper identity %#v", got, want)
	}
	if got := semanticInt(model["anchorIndex"]); got != 0 {
		t.Fatalf("child anchorIndex = %d, want 0", got)
	}

	state, supported := BuildAppMenuState(nil)
	if !supported {
		t.Fatal("menu-only projection rejected custom menu frames")
	}
	menus := appMapSlice(state["menus"])
	if len(menus) != 3 {
		t.Fatalf("menu-only projection contains %d menus, want root and two wrappers: %#v",
			len(menus), menus)
	}
	if got, want := menus[2]["id"], vtui.SemanticID(childFrame); got != want {
		t.Fatalf("projected child id = %#v, want %#v", got, want)
	}
	if got, want := menus[2]["parentId"], vtui.SemanticID(parentFrame); got != want {
		t.Fatalf("projected child parentId = %#v, want %#v", got, want)
	}
}

func TestSemanticMenuActivationUsesActualProviderFrame(t *testing.T) {
	control := vtui.NewVMenu("Wrapped")
	control.AddItem(vtui.MenuItem{ID: "first", Text: "First"})
	control.AddItem(vtui.MenuItem{ID: "second", Text: "Second"})
	frame := &semanticMenuControlTestFrame{VMenu: control}

	if !handleSemanticFrameAction(frame, vtui.SemanticID(frame), map[string]any{
		"action": "menu.activate",
		"index":  1,
	}) {
		t.Fatal("wrapped menu activation was not handled")
	}
	if frame.returnActivations != 1 || frame.selectedOnReturn != 1 {
		t.Fatalf("wrapped activation count/selection = %d/%d, want 1/1",
			frame.returnActivations, frame.selectedOnReturn)
	}
	if frame.inputSource != "qt_semantic" {
		t.Fatalf("wrapped activation input source = %q, want qt_semantic",
			frame.inputSource)
	}
}

func TestSemanticMenuScrollAcceptsAbsoluteNativeTop(t *testing.T) {
	oldFrameManager := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFrameManager }()

	screen := vtui.NewScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)

	control := vtui.NewVMenu("Wrapped")
	for i := 0; i < 20; i++ {
		control.AddItem(vtui.MenuItem{Text: "Row"})
	}
	control.SetPosition(0, 0, 20, 5)
	frame := &semanticMenuControlTestFrame{VMenu: control}
	target := vtui.SemanticID(frame)

	if !handleSemanticFrameAction(frame, target, map[string]any{
		"action": "menu.scroll",
		"top":    7,
	}) {
		t.Fatal("absolute native menu scroll was not handled")
	}
	if control.TopPos != 7 || control.SelectPos != 7 {
		t.Fatalf("absolute scroll top/selection = %d/%d, want 7/7",
			control.TopPos, control.SelectPos)
	}

	if !handleSemanticFrameAction(frame, target, map[string]any{
		"action": "menu.scroll",
		"delta":  -2,
	}) {
		t.Fatal("relative menu scroll was not handled")
	}
	if control.TopPos != 5 || control.SelectPos != 5 {
		t.Fatalf("relative scroll top/selection = %d/%d, want 5/5",
			control.TopPos, control.SelectPos)
	}
}
