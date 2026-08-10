package vtui

import (
	"testing"
)

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
