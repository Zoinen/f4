package main

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestEditor_MoveLine_UpAndDown(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree")
	ev.CursorLine = 1
	ev.CursorPos = 2

	ev.MoveLines(-1)
	if got, want := ev.pt.String(), "two\none\nthree"; got != want {
		t.Fatalf("buffer after move up = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 2 {
		t.Errorf("cursor after move up = line %d pos %d, want line 0 pos 2", ev.CursorLine, ev.CursorPos)
	}
	if !ev.modified {
		t.Error("buffer not marked modified")
	}

	// Back down again restores the original order, one undo step per move.
	ev.MoveLines(1)
	if got, want := ev.pt.String(), "one\ntwo\nthree"; got != want {
		t.Fatalf("buffer after move down = %q, want %q", got, want)
	}
	if ev.CursorLine != 1 || ev.CursorPos != 2 {
		t.Errorf("cursor after move down = line %d pos %d, want line 1 pos 2", ev.CursorLine, ev.CursorPos)
	}

	ev.Undo()
	if got, want := ev.pt.String(), "two\none\nthree"; got != want {
		t.Errorf("buffer after undo = %q, want %q", got, want)
	}
}

func TestEditor_MoveLine_StopsAtFileEdges(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo")

	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.MoveLines(-1)
	if got, want := ev.pt.String(), "one\ntwo"; got != want {
		t.Fatalf("buffer after move up at top = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 1 {
		t.Errorf("cursor moved at top of file: line %d pos %d", ev.CursorLine, ev.CursorPos)
	}
	if ev.modified {
		t.Error("no-op move marked the buffer modified")
	}

	ev.CursorLine = 1
	ev.MoveLines(1)
	if got, want := ev.pt.String(), "one\ntwo"; got != want {
		t.Fatalf("buffer after move down at bottom = %q, want %q", got, want)
	}
}

// The last line of a file need not end with a line break, and moving lines
// across that boundary must not add one or lose one.
func TestEditor_MoveLine_UnterminatedLastLine(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nlast")
	ev.CursorLine = 2
	ev.CursorPos = 4

	ev.MoveLines(-1)
	if got, want := ev.pt.String(), "one\nlast\ntwo"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 1 || ev.CursorPos != 4 {
		t.Errorf("cursor = line %d pos %d, want line 1 pos 4", ev.CursorLine, ev.CursorPos)
	}

	// And the other way round: pushing a line into the last slot leaves it
	// without a terminator too.
	ev2 := newDuplicateLineEditor(t, "one\ntwo\nlast")
	ev2.CursorLine = 1
	ev2.CursorPos = 3
	ev2.MoveLines(1)
	if got, want := ev2.pt.String(), "one\nlast\ntwo"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev2.CursorLine != 2 || ev2.CursorPos != 3 {
		t.Errorf("cursor = line %d pos %d, want line 2 pos 3", ev2.CursorLine, ev2.CursorPos)
	}
}

func TestEditor_MoveLine_KeepsCRLF(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\r\ntwo\r\nthree\r\n")
	ev.CursorLine = 0
	ev.CursorPos = 0

	ev.MoveLines(1)
	if got, want := ev.pt.String(), "two\r\none\r\nthree\r\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}

	// The synthesised terminator at the unterminated end of a CRLF file has
	// to be a CRLF as well.
	ev2 := newDuplicateLineEditor(t, "one\r\nlast")
	ev2.CursorLine = 0
	ev2.CursorPos = 0
	ev2.MoveLines(1)
	if got, want := ev2.pt.String(), "last\r\none"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_MoveLine_SelectedBlock(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	// "two" and "three" selected, from the middle of one to the middle of
	// the other.
	ev.selActive = true
	ev.selAnchorOffset = 5 // line 1, pos 1
	ev.CursorLine = 2
	ev.CursorPos = 2

	ev.MoveLines(1)

	if got, want := ev.pt.String(), "one\nfour\ntwo\nthree"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 3 || ev.CursorPos != 2 {
		t.Errorf("cursor = line %d pos %d, want line 3 pos 2", ev.CursorLine, ev.CursorPos)
	}
	// The selection still covers the same two lines, now one line lower.
	if want := ev.li.GetLineOffset(2) + 1; ev.selAnchorOffset != want {
		t.Errorf("selection anchor = %d, want %d", ev.selAnchorOffset, want)
	}
}

// A selection made with Shift+Down stops at the start of the line after the
// block; that line is the end of the selection, not a selected line.
func TestEditor_MoveLine_SelectionEndingAtLineStart(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	ev.selActive = true
	ev.selAnchorOffset = 4 // start of "two"
	ev.CursorLine = 2      // start of "three"
	ev.CursorPos = 0

	ev.MoveLines(1)

	if got, want := ev.pt.String(), "one\nthree\ntwo\nfour"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.selAnchorOffset != ev.li.GetLineOffset(2) {
		t.Errorf("selection anchor = %d, want %d", ev.selAnchorOffset, ev.li.GetLineOffset(2))
	}
	if ev.CursorLine != 3 || ev.CursorPos != 0 {
		t.Errorf("cursor = line %d pos %d, want line 3 pos 0", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditor_MoveLine_RectangularSelection(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	ev.rectSelActive = true
	ev.rectSelStartLine = 1
	ev.rectSelStartCol = 1
	ev.CursorLine = 2
	ev.CursorPos = 2

	ev.MoveLines(-1)

	if got, want := ev.pt.String(), "two\nthree\none\nfour"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.rectSelStartLine != 0 || ev.CursorLine != 1 {
		t.Errorf("rect selection = lines %d..%d, want 0..1", ev.rectSelStartLine, ev.CursorLine)
	}
}

func TestEditor_MoveLine_Hotkeys(t *testing.T) {
	for _, tc := range []struct {
		name    string
		action  string
		key     uint16
		defKey  string
		initial string
		want    string
		line    int
	}{
		{"up", "Editor.MoveLineUp", vtinput.VK_UP, "CtrlShiftUp", "one\ntwo", "two\none", 1},
		{"down", "Editor.MoveLineDown", vtinput.VK_DOWN, "CtrlShiftDown", "one\ntwo", "two\none", 0},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			action, ok := GetAction(tc.action)
			if !ok {
				t.Fatalf("%s is not registered", tc.action)
			}
			if action.Area != "Editor" || action.MenuPath != "Edit" ||
				len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != tc.defKey {
				t.Fatalf("registration = %+v; want Editor/Edit bound to %s", action, tc.defKey)
			}

			ev := newDuplicateLineEditor(t, tc.initial)
			ev.CursorLine = tc.line
			ev.CursorPos = 0

			pressKey(ev, &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				VirtualKeyCode:  tc.key,
				ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
			})

			if got := ev.pt.String(); got != tc.want {
				t.Fatalf("buffer = %q, want %q", got, tc.want)
			}
		})
	}
}
