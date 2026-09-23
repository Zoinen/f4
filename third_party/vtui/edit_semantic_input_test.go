package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestEditWordDeletion(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		cursor     int
		key        uint16
		want       string
		wantCursor int
	}{
		{"backward", "alpha beta", 10, vtinput.VK_BACK, "alpha ", 6},
		{"forward", "alpha beta", 0, vtinput.VK_DELETE, "beta", 0},
		{"middle backward", "alpha beta", 8, vtinput.VK_BACK, "alpha ta", 6},
		{"middle forward", "alpha beta", 8, vtinput.VK_DELETE, "alpha be", 8},
		{"start", "alpha", 0, vtinput.VK_BACK, "alpha", 0},
		{"end", "alpha", 5, vtinput.VK_DELETE, "alpha", 5},
		{"unicode", "one снимок", 10, vtinput.VK_BACK, "one ", 4},
		{"cluster", "one e\u0301clair", 4, vtinput.VK_DELETE, "one ", 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEdit(0, 0, 30, tc.text)
			e.ClearSelection()
			e.curPos = tc.cursor
			if !e.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tc.key, ControlKeyState: vtinput.LeftCtrlPressed,
			}) {
				t.Fatal("word deletion was not handled")
			}
			if e.GetText() != tc.want || e.curPos != tc.wantCursor {
				t.Fatalf("got %q at %d, want %q at %d", e.GetText(), e.curPos, tc.want, tc.wantCursor)
			}
		})
	}
}

func TestEditSemanticSelectionOwnsKeyboard(t *testing.T) {
	e := NewEdit(0, 0, 30, "alpha снимок 😀")
	e.SetFocus(true)
	if !e.HandleSemanticAction(map[string]any{
		"action": "control.select", "anchor": 6, "cursor": 12,
	}) {
		t.Fatal("native selection did not reach Go")
	}
	if e.selStart != 6 || e.selEnd != 12 || e.curPos != 12 {
		t.Fatalf("selection was not preserved: %#v", e.SemanticNode(nil))
	}
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})
	if e.GetText() != "alpha  😀" {
		t.Fatalf("Backspace did not remove the native selection: %q", e.GetText())
	}
	e.HandleSemanticAction(map[string]any{"action": "control.select", "anchor": 6, "cursor": 6})
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})
	if e.curPos != 0 || e.selStart != 0 || e.selEnd != 6 {
		t.Fatalf("Ctrl+Shift+Left did not extend selection from native caret: %#v", e.SemanticNode(nil))
	}
}

func TestEditSemanticSelectionClampsAndPreservesDirection(t *testing.T) {
	e := NewEdit(0, 0, 20, "a😀bc")
	e.HandleSemanticAction(map[string]any{"action": "control.select", "anchor": 100, "cursor": -4})
	if e.selAnchor != 4 || e.curPos != 0 || e.selStart != 0 || e.selEnd != 4 {
		t.Fatalf("invalid or reversed selection: %#v", e.SemanticNode(nil))
	}
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE})
	if e.GetText() != "" {
		t.Fatalf("reverse selection deletion = %q", e.GetText())
	}
}

func TestEditSemanticMultilineCursorPreservesLineBoundaries(t *testing.T) {
	for _, text := range []string{"abc\nx\nabcdef", "a\n\nb", "e\u0301\nx"} {
		e := NewEdit(0, 0, 30, text)
		e.Multiline = true
		for offset := 0; offset <= len(e.text); offset++ {
			if offset > 0 && offset < len(e.text) && e.text[offset] == '\u0301' {
				continue
			}
			e.HandleSemanticAction(map[string]any{"action": "control.select", "anchor": offset, "cursor": offset})
			if e.curPos != offset {
				t.Errorf("text %q: cursor=%d, want %d", text, e.curPos, offset)
			}
		}
	}
}
