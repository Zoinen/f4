package app

import (
	"github.com/unxed/vtui"
	"testing"
)

func TestHandleSemanticActionNil(t *testing.T) {
	if HandleSemanticAction(nil) {
		t.Fatal("HandleSemanticAction(nil) = true, want false")
	}
}

func TestHandleSemanticElementActionWidgets(t *testing.T) {
	edit := vtui.NewEdit(0, 0, 20, "")
	if !handleSemanticElementAction(edit, map[string]any{"action": "set_text", "text": "alpha"}) {
		t.Fatal("set_text action was not handled")
	}
	if got := edit.GetText(); got != "alpha" {
		t.Fatalf("edit text = %q, want alpha", got)
	}
	if !handleSemanticElementAction(edit, map[string]any{"action": "insert_text", "text": " beta"}) {
		t.Fatal("insert_text action was not handled")
	}
	if got := edit.GetText(); got != "alpha beta" {
		t.Fatalf("edit text after insert = %q, want alpha beta", got)
	}
	if !handleSemanticElementAction(edit, map[string]any{"action": "focus"}) {
		t.Fatal("focus action was not handled")
	}
	if !edit.IsFocused() {
		t.Fatal("edit did not receive focus")
	}

	checkbox := vtui.NewCheckbox(0, 0, "enabled", false)
	if !handleSemanticElementAction(checkbox, map[string]any{"action": "toggle"}) {
		t.Fatal("toggle action was not handled")
	}
	if checkbox.State != 1 {
		t.Fatalf("checkbox state = %d, want checked state 1", checkbox.State)
	}

	radio := vtui.NewRadioGroup(0, 0, 1, []string{"one", "two"})
	if !handleSemanticElementAction(radio, map[string]any{"action": "select", "index": 1}) {
		t.Fatal("radio select action was not handled")
	}
	if radio.Selected != 1 {
		t.Fatalf("radio selection = %d, want 1", radio.Selected)
	}

	list := vtui.NewListBox(0, 0, 20, 3, []string{"one", "two", "three"})
	if !handleSemanticElementAction(list, map[string]any{"action": "select", "index": 2}) {
		t.Fatal("list select action was not handled")
	}
	if list.SelectPos != 2 {
		t.Fatalf("list selection = %d, want 2", list.SelectPos)
	}
	if handleSemanticElementAction(list, map[string]any{"action": "select", "index": 3}) {
		t.Fatal("out-of-range list select action was handled")
	}

	combo := vtui.NewComboBox(0, 0, 20, []string{"one", "two"})
	if !handleSemanticElementAction(combo, map[string]any{"action": "select", "index": 1}) {
		t.Fatal("combo select action was not handled")
	}
	if got := combo.Edit.GetText(); got != "two" {
		t.Fatalf("combo text = %q, want two", got)
	}
	if handleSemanticElementAction(edit, map[string]any{"action": "unknown"}) {
		t.Fatal("unknown element action was handled")
	}
}

func TestHandleSemanticChildrenActionFindsTarget(t *testing.T) {
	dlg := vtui.NewCenteredDialog(40, 8, "semantic")
	edit := vtui.NewEdit(0, 0, 20, "")
	dlg.AddItem(edit)

	if !handleSemanticChildrenAction(dlg.GetChildren(), vtui.SemanticID(edit), map[string]any{"action": "set_text", "text": "nested"}) {
		t.Fatal("child semantic target was not handled")
	}
	if got := edit.GetText(); got != "nested" {
		t.Fatalf("child edit text = %q, want nested", got)
	}
	if handleSemanticChildrenAction(dlg.GetChildren(), "missing-target", map[string]any{"action": "focus"}) {
		t.Fatal("missing child semantic target was handled")
	}
}

func TestHandleSemanticFrameAction(t *testing.T) {
	dlg := vtui.NewCenteredDialog(40, 8, "semantic")
	edit := vtui.NewEdit(0, 0, 20, "")
	dlg.AddItem(edit)

	if !handleSemanticFrameAction(dlg, vtui.SemanticID(edit), map[string]any{"action": "control.focus"}) {
		t.Fatal("frame child action was not handled")
	}
	if !edit.IsFocused() {
		t.Fatal("frame child did not receive focus")
	}

	frame := vtui.NewWindow(0, 0, 20, 6, "semantic")
	if !handleSemanticFrameAction(frame, vtui.SemanticID(frame), map[string]any{"action": "close"}) {
		t.Fatal("frame close action was not handled")
	}
}
