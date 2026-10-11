package editor

import (
	"testing"

	"github.com/unxed/vtinput"
)

func pressEditorTab(ev *EditorView, shift bool) {
	var modifiers vtinput.ControlKeyState
	if shift {
		modifiers = vtinput.ShiftPressed
	}
	ev.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_TAB,
		ControlKeyState: modifiers,
	})
}

func TestEditor_TabIndentsMultilineSelectionWithoutReplacingIt(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree")
	ev.ExpandTabs = 0
	ev.SelActive = true
	ev.SelAnchorOffset = 0
	ev.CursorLine = 2
	ev.CursorPos = len("three")

	pressEditorTab(ev, false)

	if got, want := ev.Pt.String(), "\tone\n\ttwo\n\tthree"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if !ev.SelActive || ev.SelAnchorOffset != 0 {
		t.Fatalf("selection = active %v anchor %d, want active at 0", ev.SelActive, ev.SelAnchorOffset)
	}
	if ev.CursorLine != 2 || ev.CursorPos != len("\tthree") {
		t.Errorf("cursor = line %d pos %d, want line 2 pos 6", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditor_ShiftTabUnindentsMultilineSelection(t *testing.T) {
	ev := newDuplicateLineEditor(t, "\tone\n  two\n    three")
	ev.TabSize = 4
	ev.SelActive = true
	ev.SelAnchorOffset = 0
	ev.CursorLine = 2
	ev.CursorPos = len("    three")

	pressEditorTab(ev, true)

	if got, want := ev.Pt.String(), "one\ntwo\nthree"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if !ev.SelActive || ev.SelAnchorOffset != 0 {
		t.Fatalf("selection = active %v anchor %d, want active at 0", ev.SelActive, ev.SelAnchorOffset)
	}
}

func TestEditor_TabIndentsRectangularMultilineSelection(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree")
	ev.ExpandTabs = 1
	ev.TabSize = 2
	ev.RectSelActive = true
	ev.rectSelStartLine = 0
	ev.rectSelStartCol = 1
	ev.CursorLine = 2
	ev.CursorPos = 2

	pressEditorTab(ev, false)

	if got, want := ev.Pt.String(), "  one\n  two\n  three"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if !ev.RectSelActive || ev.rectSelStartLine != 0 || ev.rectSelStartCol != 1 {
		t.Fatalf("rectangular selection changed: active=%v start=%d,%d", ev.RectSelActive, ev.rectSelStartLine, ev.rectSelStartCol)
	}
}
