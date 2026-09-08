package vtui

import (
	"fmt"
	"github.com/unxed/vtinput"
	"testing"
)

func TestSemanticMenuBarExportsOptionalItemIcons(t *testing.T) {
	menuBar := NewMenuBar(nil)
	menuBar.Items = []MenuBarItem{{
		Label: "&Files",
		SubItems: []MenuItem{
			{Text: "&Open", Icon: "folder"},
			{Text: "Close"},
		},
	}}

	node := semanticMenuBar(menuBar)
	items := node["items"].([]map[string]any)
	subItems := items[0]["items"].([]map[string]any)
	if subItems[0]["icon"] != "folder" {
		t.Fatalf("menu icon was not exported: %#v", subItems[0])
	}
	if _, exists := subItems[1]["icon"]; exists {
		t.Fatalf("icon-less menu item exported an empty icon: %#v", subItems[1])
	}
}

func TestSemantic_DialogHierarchyExport(t *testing.T) {
	SetDefaultPalette()
	dlg := NewCenteredDialog(40, 10, "Test Dlg")
	btn := NewButton(2, 2, "Ok")
	chk := NewCheckbox(2, 4, "Option", false)
	dlg.AddItem(btn)
	dlg.AddItem(chk)

	node := dlg.SemanticNode(&SemanticContext{Width: 80, Height: 25})
	if node["kind"] != "dialog" || node["title"] != "Test Dlg" {
		t.Fatalf("unexpected dialog node: %#v", node)
	}

	children := node["children"].([]map[string]any)
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	if children[0]["kind"] != "button" || children[0]["text"] != "Ok" {
		t.Errorf("unexpected button node: %#v", children[0])
	}
	if children[0]["visible"] != true {
		t.Errorf("new dialog button was hidden before its first render: %#v", children[0])
	}

	if children[1]["kind"] != "checkbox" || children[1]["text"] != "Option" {
		t.Errorf("unexpected checkbox node: %#v", children[1])
	}
}

func TestSemantic_GroupBoxExportsVisualFrameAndChildren(t *testing.T) {
	dlg := NewDialog(0, 0, 39, 14, "Groups")
	group := NewGroupBox(3, 4, 30, 9, " &Flags ")
	child := NewCheckbox(5, 5, "Read only", false)
	group.AddItem(child)
	dlg.AddItem(group)

	dialogNode := dlg.SemanticNode(&SemanticContext{Width: 80, Height: 25})
	groups := dialogNode["children"].([]map[string]any)
	if len(groups) != 1 {
		t.Fatalf("expected one visual group, got %#v", groups)
	}
	node := groups[0]
	if node["kind"] != "group" || node["title"] != "Flags" {
		t.Fatalf("unexpected group box node: %#v", node)
	}
	if node["hotkey"] != "f" || node["bordered"] != true {
		t.Fatalf("group box lost visual metadata: %#v", node)
	}
	children := node["children"].([]map[string]any)
	if len(children) != 1 || children[0]["kind"] != "checkbox" ||
		children[0]["text"] != "Read only" {
		t.Fatalf("group box children were not preserved: %#v", children)
	}

	plain := NewGroup(0, 0, 10, 2).SemanticNode(nil)
	if _, exists := plain["bordered"]; exists {
		t.Fatalf("plain group was incorrectly marked as bordered: %#v", plain)
	}
}

func TestSemantic_EditSelectionActivationFollowsFocus(t *testing.T) {
	e := NewEdit(0, 0, 20, "initial path")

	node := e.SemanticNode(nil)
	if node["selectionActive"] != false {
		t.Fatalf("dormant initial selection was exported as active: %#v", node)
	}
	if node["selectionStart"] != 0 || node["selectionEnd"] != len(e.text) {
		t.Fatalf("initial replacement range was lost: %#v", node)
	}

	e.SetFocus(true)
	node = e.SemanticNode(nil)
	if node["selectionActive"] != true {
		t.Fatalf("focused initial selection was not activated: %#v", node)
	}

	e.ClearSelection()
	node = e.SemanticNode(nil)
	if node["selectionActive"] != false {
		t.Fatalf("cleared selection remained active: %#v", node)
	}
}

func TestSemantic_DialogPreservesExplicitlyHiddenChildBeforeFirstRender(t *testing.T) {
	SetDefaultPalette()
	dlg := NewCenteredDialog(40, 10, "Test Dlg")
	visible := NewButton(2, 2, "Visible")
	hidden := NewButton(2, 4, "Hidden")
	hidden.SetVisible(false)
	dlg.AddItem(visible)
	dlg.AddItem(hidden)

	node := dlg.SemanticNode(&SemanticContext{Width: 80, Height: 25})
	children := node["children"].([]map[string]any)
	if children[0]["visible"] != true {
		t.Fatalf("untouched child should be logically visible: %#v", children[0])
	}
	if children[1]["visible"] != false {
		t.Fatalf("explicitly hidden child became visible: %#v", children[1])
	}
}

func TestSemantic_ActionHandling(t *testing.T) {
	SetDefaultPalette()
	dlg := NewCenteredDialog(40, 10, "Test Dlg")
	btnClicked := false
	btn := NewButton(2, 2, "Ok")
	btn.OnClick = func() { btnClicked = true }

	chk := NewCheckbox(2, 4, "Option", false)
	dlg.AddItem(btn)
	dlg.AddItem(chk)

	// 1. Test Button activation via action routing
	actionBtn := map[string]any{
		"target": SemanticID(btn),
		"action": "activate",
	}
	if !dlg.HandleSemanticAction(actionBtn) {
		t.Error("button action not handled by dialog router")
	}
	if !btnClicked {
		t.Error("button OnClick was not triggered")
	}

	// 2. Test Checkbox toggle via action routing
	if chk.State != 0 {
		t.Fatal("checkbox should be unchecked")
	}
	actionChk := map[string]any{
		"target": SemanticID(chk),
		"action": "toggle",
	}
	if !dlg.HandleSemanticAction(actionChk) {
		t.Error("checkbox action not handled by dialog router")
	}
	if chk.State != 1 {
		t.Error("checkbox state was not toggled to 1")
	}
}

func TestSemantic_DialogGeometryActionMovesAndResizesWindow(t *testing.T) {
	dlg := NewDialog(5, 4, 44, 13, "Resizable")
	action := map[string]any{
		"target": SemanticID(dlg),
		"action": "dialog.geometry",
		"x":      float64(12),
		"y":      float64(7),
		"w":      float64(52),
		"h":      float64(18),
	}

	if !dlg.HandleSemanticAction(action) {
		t.Fatal("dialog geometry action was not handled")
	}
	x1, y1, x2, y2 := dlg.GetPosition()
	if x1 != 12 || y1 != 7 || x2-x1+1 != 52 || y2-y1+1 != 18 {
		t.Fatalf("unexpected dialog geometry: (%d,%d)-(%d,%d)",
			x1, y1, x2, y2)
	}

	if dlg.HandleSemanticAction(map[string]any{
		"target": SemanticID(dlg),
		"action": "dialog.geometry",
		"x":      1,
		"y":      1,
		"w":      0,
		"h":      10,
	}) {
		t.Fatal("invalid zero-width geometry action was accepted")
	}
}

func TestSemanticClickActionsTransferKeyboardFocus(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, kind := range []string{"checkbox", "radio", "button"} {
			t.Run(fmt.Sprintf("%s/nested=%v", kind, nested), func(t *testing.T) {
				root := NewGroup(0, 0, 60, 20)
				old := NewCheckbox(1, 1, "Old", false)
				root.AddItem(old)
				owner := root
				if nested {
					owner = NewGroup(1, 3, 50, 12)
					owner.AddItem(NewCheckbox(1, 3, "Other", false))
					root.AddItem(owner)
				}
				var control UIElement
				action := map[string]any{}
				switch kind {
				case "checkbox":
					control = NewCheckbox(2, 5, "Clicked", false)
					action["action"] = "control.toggle"
				case "radio":
					control = NewRadioGroup(2, 5, 1, []string{"First", "Second"})
					action["action"] = "control.select"
					action["index"] = 1
				case "button":
					button := NewButton(2, 5, "Clicked")
					button.OnClick = func() {}
					control = button
					action["action"] = "control.activate"
				}
				owner.AddItem(control)
				root.SetFocus(true)
				root.SetFocusedItem(old)
				action["target"] = SemanticID(control)
				if !root.HandleSemanticAction(action) {
					t.Fatal("click not handled")
				}
				if owner.GetFocusedItem() != control || !control.IsFocused() || old.IsFocused() {
					t.Fatal("click did not transfer keyboard focus")
				}
				if nested && root.GetFocusedItem() != owner {
					t.Fatal("ancestor focus not transferred")
				}
				if kind == "checkbox" {
					root.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
					if control.(*Checkbox).State != 0 {
						t.Fatal("Space did not reach clicked checkbox")
					}
				}
				if kind == "radio" {
					root.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
					node := control.(*RadioGroup).SemanticNode(nil)
					if node["focusIndex"] != 0 || node["selected"] != 1 {
						t.Fatal("Up did not reach clicked radio group")
					}
				}
			})
		}
	}
}

func TestSemantic_ListBoxExportsRowsAndRoutesSelection(t *testing.T) {
	dlg := NewDialog(0, 0, 59, 14, "Deletion Errors")
	lb := NewListBox(2, 2, 56, 3, []string{"Skipped 'photo-1.png':", "unlinkat", "The file is used by another process.", "last error"})
	dlg.AddItem(lb)
	node := dlg.SemanticNode(&SemanticContext{Width: 80, Height: 25})
	list := node["children"].([]map[string]any)[0]
	if list["kind"] != "listBox" || list["id"] != SemanticID(lb) || list["visible"] != true {
		t.Fatalf("unexpected list export: %#v", list)
	}
	items := list["items"].([]string)
	if len(items) != len(lb.Items) || items[2] != lb.Items[2] {
		t.Fatalf("lost error rows: %#v", items)
	}
	selected := -1
	lb.OnSelect = func(index int) { selected = index }
	if !dlg.HandleSemanticAction(map[string]any{"target": SemanticID(lb), "action": "control.select", "index": 3}) {
		t.Fatal("list selection was not routed")
	}
	if selected != 3 || lb.SelectPos != 3 || lb.TopPos != 1 || !lb.IsFocused() {
		t.Fatalf("selection did not update callback/focus/viewport: %d, %#v", selected, lb)
	}
	if lb.HandleSemanticAction(map[string]any{"action": "control.select", "index": 4}) {
		t.Fatal("out-of-range selection accepted")
	}
}
