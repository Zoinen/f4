package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

func undoKey(e *Edit, key uint16, mods vtinput.ControlKeyState) bool {
	return e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key, ControlKeyState: mods,
	})
}

func TestEditUndoTypingAndRedo(t *testing.T) {
	e := NewEdit(0, 0, 30, "")
	for _, char := range "hé🙂" {
		e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char})
	}
	if !undoKey(e, vtinput.VK_Z, vtinput.LeftCtrlPressed) || e.GetText() != "" {
		t.Fatalf("undo typed run: %q", e.GetText())
	}
	if !undoKey(e, vtinput.VK_Z, vtinput.LeftCtrlPressed|vtinput.ShiftPressed) || e.GetText() != "hé🙂" {
		t.Fatalf("redo typed run: %q", e.GetText())
	}
	undoKey(e, vtinput.VK_Z, vtinput.LeftCtrlPressed)
	e.InsertString("other")
	undoKey(e, vtinput.VK_Y, vtinput.LeftCtrlPressed)
	if e.GetText() != "other" {
		t.Fatalf("new edit retained obsolete redo: %q", e.GetText())
	}
}

func TestEditUndoSelectionAndPaste(t *testing.T) {
	e := NewEdit(0, 0, 30, "hello🙂")
	e.HandleSemanticAction(map[string]any{"action": "control.select", "anchor": 1, "cursor": 5})
	e.InsertString("X\nY")
	undoKey(e, vtinput.VK_Z, vtinput.LeftCtrlPressed)
	if e.GetText() != "hello🙂" || e.curPos != 5 || e.selStart != 1 || e.selEnd != 5 {
		t.Fatalf("undo selection replacement: %q cursor=%d selection=%d:%d", e.GetText(), e.curPos, e.selStart, e.selEnd)
	}
	undoKey(e, vtinput.VK_Y, vtinput.LeftCtrlPressed)
	if e.GetText() != "hX\nY🙂" {
		t.Fatalf("redo selection replacement: %q", e.GetText())
	}
	e.SetText("baseline")
	e.ClearSelection()
	e.Multiline = true
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	for _, char := range "\r\nnext" {
		e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char})
	}
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType})
	undoKey(e, vtinput.VK_Z, vtinput.LeftCtrlPressed)
	if e.GetText() != "baseline" {
		t.Fatalf("paste must undo as one transaction: %q", e.GetText())
	}
	undoKey(e, vtinput.VK_Y, vtinput.LeftCtrlPressed)
	if e.GetText() != "baseline\nnext" {
		t.Fatalf("redo multiline paste: %q", e.GetText())
	}
}

func TestEditUndoMutations(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Edit)
	}{
		{name: "backspace", edit: func(e *Edit) { undoKey(e, vtinput.VK_BACK, 0) }},
		{name: "word deletion", edit: func(e *Edit) { undoKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed) }},
		{name: "delete", edit: func(e *Edit) {
			undoKey(e, vtinput.VK_HOME, 0)
			undoKey(e, vtinput.VK_DELETE, 0)
		}},
		{name: "overtype", edit: func(e *Edit) {
			undoKey(e, vtinput.VK_HOME, 0)
			undoKey(e, vtinput.VK_INSERT, 0)
			e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'X'})
		}},
		{name: "clear selection", edit: func(e *Edit) { e.SelectAll(); undoKey(e, vtinput.VK_DELETE, 0) }},
		{name: "history", edit: func(e *Edit) { e.History = []string{"previous"}; e.HistoryUp() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := NewEdit(0, 0, 30, "hello world")
			e.ClearSelection()
			test.edit(e)
			after := e.GetText()
			if after == "hello world" || !e.CanUndo() {
				t.Fatal("mutation was not recorded")
			}
			e.Undo()
			if e.GetText() != "hello world" || !e.CanRedo() {
				t.Fatal("undo failed")
			}
			e.Redo()
			if e.GetText() != after {
				t.Fatal("redo failed")
			}
		})
	}
}

func TestEditUndoBlockAndNotifications(t *testing.T) {
	e := NewEdit(0, 0, 39, "abcdefghi")
	e.Multiline = true
	e.SetMultilineBlockSelectionAt(1, 8, MultilineBlockSelectionGeometry{
		AnchorRow: 0, AnchorColumn: 1, FocusRow: 1, FocusColumn: 4, WrapWidth: 5,
	})
	e.DeleteBlock()
	if e.GetText() != "aef" {
		t.Fatal(e.GetText())
	}
	var notified []string
	e.OnTextChange = func(text string) { notified = append(notified, text) }
	e.HandleSemanticAction(map[string]any{"action": "control.undo"})
	if e.GetText() != "abcdefghi" || !e.blockSelection || e.blockWrapWidth != 5 {
		t.Fatal("block not restored")
	}
	e.HandleSemanticAction(map[string]any{"action": "control.redo"})
	if len(notified) != 2 || notified[0] != "abcdefghi" || notified[1] != "aef" {
		t.Fatal(notified)
	}
	if e.SemanticNode(nil)["canUndo"] != true || e.SemanticNode(nil)["canRedo"] != false {
		t.Fatal("wrong semantic state")
	}
}

func TestEditUndoBoundariesAndLimits(t *testing.T) {
	e := NewEdit(0, 0, 30, "")
	undoKey(e, vtinput.VK_BACK, 0)
	if e.CanUndo() {
		t.Fatal("no-op edit created undo")
	}
	for range editUndoLimit + 5 {
		e.InsertString("a")
	}
	undos := 0
	for e.Undo() {
		undos++
	}
	if undos != editUndoLimit {
		t.Fatalf("undo limit: %d", undos)
	}
	e.SetText("") // A new baseline also clears redo when text is unchanged.
	if e.CanRedo() || e.CanUndo() {
		t.Fatal("new baseline kept history")
	}
	e.InsertString(strings.Repeat("x", editUndoBytes+1))
	if e.CanUndo() {
		t.Fatal("unbounded undo storage")
	}
	e.SetText("")
	e.PasswordMode = true
	e.InsertString("secret")
	if e.CanUndo() || len(e.undoChanges) != 0 {
		t.Fatal("password retained history")
	}
	e.PasswordMode = false
	e.SetText("")
	e.InsertString("draft")
	e.SetDisabled(true)
	if e.CanUndo() || e.Undo() {
		t.Fatal("disabled edit accepts undo")
	}
}

func TestEditUndoCompletionPreview(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	e := NewEdit(0, 0, 30, "")
	e.InsertString("he")
	e.History = []string{"hello"}
	e.AutoCompletePreview = true
	ac := NewAutoCompleteMenu(e)
	ac.Preview(1)
	if e.GetText() != "hello" || !e.CanUndo() {
		t.Fatal("preview lost undo")
	}
	ac.Preview(0)
	if e.GetText() != "he" {
		t.Fatal("preview did not restore draft")
	}
	ac.accept(1, false)
	if e.GetText() != "hello" {
		t.Fatal("completion not accepted")
	}
	e.Undo()
	if e.GetText() != "he" {
		t.Fatal("completion did not undo as one replacement")
	}
	e.Undo()
	if e.GetText() != "" {
		t.Fatal("completion lost earlier undo")
	}
}
