package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestMultiLineSemanticEditing(t *testing.T) {
	m := NewMultiLineEdit(1, 2, 40, 4, "echo 😀\necho second")
	p, ok := any(m).(interface {
		SemanticNode(*SemanticContext) map[string]any
	})
	if !ok {
		t.Fatal("multiline editor has no semantic projection")
	}
	h, ok := any(m).(SemanticActionHandler)
	if !ok {
		t.Fatal("multiline editor has no semantic actions")
	}
	m.SetFocus(true)
	n := p.SemanticNode(nil)
	if n["kind"] != "multiLineEdit" || n["text"] != m.GetText() || n["h"] != 4 {
		t.Fatalf("projection: %#v", n)
	}
	if !h.HandleSemanticAction(map[string]any{"action": "control.select", "anchor": 5, "cursor": 8}) {
		t.Fatal("selection rejected")
	}
	if got := m.CopySelection(); got != "😀\ne" {
		t.Fatalf("selection = %q", got)
	}
	h.HandleSemanticAction(map[string]any{"action": "control.insertText", "text": "X\nY"})
	if got := m.GetText(); got != "echo X\nYcho second" {
		t.Fatalf("replacement = %q", got)
	}
	n = p.SemanticNode(nil)
	if n["cursor"] != 8 {
		t.Fatalf("cursor = %v", n["cursor"])
	}
	m.SetDisabled(true)
	if h.HandleSemanticAction(map[string]any{"action": "control.setText", "text": "bad"}) {
		t.Fatal("disabled mutation")
	}
}

func TestMultiLineSemanticSelectionOwnsKeyboard(t *testing.T) {
	root := NewGroup(0, 0, 60, 20)
	root.AddItem(NewCheckbox(1, 1, "Other", false))
	nested := NewGroup(1, 3, 50, 12)
	edit := NewMultiLineEdit(2, 4, 40, 4, "first\nsecond")
	nested.AddItem(edit)
	root.AddItem(nested)
	root.SetFocus(true)
	if !root.HandleSemanticAction(map[string]any{"target": SemanticID(edit), "action": "control.select", "anchor": 6, "cursor": 12}) {
		t.Fatal("selection not routed")
	}
	if !edit.IsFocused() {
		t.Fatal("editor did not acquire focus")
	}
	root.ProcessKey(mleKey(0, 'X', 0))
	root.ProcessKey(mleKey(vtinput.VK_RETURN, 0, 0))
	if got := edit.GetText(); got != "first\nX\n" {
		t.Fatalf("keyboard after selection: %q", got)
	}
	changes := 0
	edit.OnTextChange = func(string) { changes++ }
	edit.HandleSemanticAction(map[string]any{"action": "control.setText", "text": "replacement"})
	if changes != 1 {
		t.Fatalf("got %d text callbacks", changes)
	}
}
