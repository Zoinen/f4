package main

import (
	"reflect"
	"testing"

	"github.com/unxed/vtinput"
)

func arrowKey(ev *EditorView, vk uint16) {
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
}

func TestEditor_MultiCursor_HorizontalMovement(t *testing.T) {
	ev := multiCursorEditor(t, "abc\nabc")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.toggleCursorAt(5)

	arrowKey(ev, vtinput.VK_RIGHT)
	if ev.CursorPos != 2 {
		t.Errorf("primary caret pos = %d, want 2", ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{6}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}

	arrowKey(ev, vtinput.VK_LEFT)
	arrowKey(ev, vtinput.VK_LEFT)
	if ev.CursorPos != 0 {
		t.Errorf("primary caret pos = %d, want 0", ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{4}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

// Moving along the text steps between lines rather than stopping at the ends,
// and a line break counts as one step however many bytes it takes.
func TestEditor_MultiCursor_HorizontalMovementCrossesLines(t *testing.T) {
	ev := multiCursorEditor(t, "ab\r\ncd\r\nef")
	ev.CursorLine = 0
	ev.CursorPos = 2 // end of "ab"
	ev.toggleCursorAt(8)

	// The primary caret steps over the two-byte break in one go; the other
	// one just moves along its own line.
	arrowKey(ev, vtinput.VK_RIGHT)
	if ev.CursorLine != 1 || ev.CursorPos != 0 {
		t.Errorf("primary caret = line %d pos %d, want line 1 pos 0", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{9}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}

	arrowKey(ev, vtinput.VK_LEFT)
	if ev.CursorLine != 0 || ev.CursorPos != 2 {
		t.Errorf("primary caret = line %d pos %d, want line 0 pos 2", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{8}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

func TestEditor_MultiCursor_VerticalMovement(t *testing.T) {
	ev := multiCursorEditor(t, "aaaa\nbbbb\ncccc\ndddd")
	ev.CursorLine = 0
	ev.CursorPos = 2
	ev.updateDesiredVisualCol()
	ev.toggleCursorAt(12) // line 2, column 2

	arrowKey(ev, vtinput.VK_DOWN)

	if ev.CursorLine != 1 || ev.CursorPos != 2 {
		t.Errorf("primary caret = line %d pos %d, want line 1 pos 2", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{17}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

// Passing a short line must not cost a caret the column it started from.
func TestEditor_MultiCursor_VerticalMovementKeepsColumns(t *testing.T) {
	ev := multiCursorEditor(t, "aaaaaa\n\naaaaaa\nbbbbbb\n\nbbbbbb")
	ev.CursorLine = 0
	ev.CursorPos = 5
	ev.updateDesiredVisualCol()
	ev.toggleCursorAt(20) // line 3, column 5

	arrowKey(ev, vtinput.VK_DOWN) // both land on an empty line
	if ev.CursorPos != 0 {
		t.Fatalf("primary caret pos on the empty line = %d, want 0", ev.CursorPos)
	}

	arrowKey(ev, vtinput.VK_DOWN)
	if ev.CursorLine != 2 || ev.CursorPos != 5 {
		t.Errorf("primary caret = line %d pos %d, want line 2 pos 5", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{28}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v (column 5 of the last line)", got, want)
	}
}

// A caret already at the edge of the text stays put instead of dragging the
// set out of shape.
func TestEditor_MultiCursor_VerticalMovementStopsAtEdges(t *testing.T) {
	ev := multiCursorEditor(t, "aaa\nbbb\nccc")
	ev.CursorLine = 2
	ev.CursorPos = 1
	ev.updateDesiredVisualCol()
	ev.toggleCursorAt(2) // line 0, column 2

	arrowKey(ev, vtinput.VK_DOWN)

	if ev.CursorLine != 2 || ev.CursorPos != 1 {
		t.Errorf("primary caret = line %d pos %d, want it to have stayed on the last line", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{6}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

func TestEditor_MultiCursor_HomeAndEnd(t *testing.T) {
	ev := multiCursorEditor(t, "aaaa\nbb")
	ev.CursorLine = 0
	ev.CursorPos = 2
	ev.toggleCursorAt(6)

	arrowKey(ev, vtinput.VK_END)
	if ev.CursorPos != 4 {
		t.Errorf("primary caret pos = %d, want the end of its own line", ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{7}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}

	arrowKey(ev, vtinput.VK_HOME)
	if ev.CursorPos != 0 {
		t.Errorf("primary caret pos = %d, want 0", ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{5}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

// Two carets on the same line both go to its start, which makes them one.
func TestEditor_MultiCursor_MovementMergesCaretsThatMeet(t *testing.T) {
	ev := multiCursorEditor(t, "abcd\nefgh")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.toggleCursorAt(3)

	arrowKey(ev, vtinput.VK_HOME)

	if ev.CursorPos != 0 {
		t.Errorf("primary caret pos = %d, want 0", ev.CursorPos)
	}
	if ev.multiCursor() {
		t.Errorf("extra carets = %v, want the two to have merged", extraCaretOffsets(ev))
	}
}

// Ctrl+Up and Ctrl+Down scroll the text under the carets, so the set survives.
func TestEditor_MultiCursor_ScrollKeepsTheSet(t *testing.T) {
	ev := multiCursorEditor(t, "a\nb\nc\nd\ne\nf")
	ev.CursorLine = 2
	ev.CursorPos = 0
	ev.toggleCursorAt(8)

	ev.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	// Scrolling can nudge the primary caret to keep it on screen, but the
	// set itself is none of its business.
	if got, want := extraCaretOffsets(ev), []int{8}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v: scrolling should leave them alone", got, want)
	}
}

// Movement is not an edit: nothing lands on the undo stack and the file is not
// marked modified.
func TestEditor_MultiCursor_MovementIsNotAnEdit(t *testing.T) {
	ev := multiCursorEditor(t, "abc\nabc")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.toggleCursorAt(4)

	arrowKey(ev, vtinput.VK_RIGHT)
	arrowKey(ev, vtinput.VK_DOWN)

	if ev.modified {
		t.Error("moving the carets marked the buffer modified")
	}
	if len(ev.undoStack) != 0 {
		t.Errorf("undo stack has %d entries after moving carets", len(ev.undoStack))
	}
}
