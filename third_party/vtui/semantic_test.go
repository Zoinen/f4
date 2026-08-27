package vtui

import (
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

	if children[1]["kind"] != "checkbox" || children[1]["text"] != "Option" {
		t.Errorf("unexpected checkbox node: %#v", children[1])
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
