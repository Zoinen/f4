package editor

import (
	"testing"
)

func TestEditor_MoveLine_UpAndDown(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree")
	ev.CursorLine = 1
	ev.CursorPos = 2

	ev.MoveLines(-1)
	if got, want := ev.Pt.String(), "two\none\nthree"; got != want {
		t.Fatalf("buffer after move up = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 2 {
		t.Errorf("cursor after move up = line %d pos %d, want line 0 pos 2", ev.CursorLine, ev.CursorPos)
	}
	if !ev.Modified {
		t.Error("buffer not marked modified")
	}

	// Back down again restores the original order, one undo step per move.
	ev.MoveLines(1)
	if got, want := ev.Pt.String(), "one\ntwo\nthree"; got != want {
		t.Fatalf("buffer after move down = %q, want %q", got, want)
	}
	if ev.CursorLine != 1 || ev.CursorPos != 2 {
		t.Errorf("cursor after move down = line %d pos %d, want line 1 pos 2", ev.CursorLine, ev.CursorPos)
	}

	ev.Undo()
	if got, want := ev.Pt.String(), "two\none\nthree"; got != want {
		t.Errorf("buffer after undo = %q, want %q", got, want)
	}
}

func TestEditor_MoveLine_StopsAtFileEdges(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo")

	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.MoveLines(-1)
	if got, want := ev.Pt.String(), "one\ntwo"; got != want {
		t.Fatalf("buffer after move up at top = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 1 {
		t.Errorf("cursor moved at top of file: line %d pos %d", ev.CursorLine, ev.CursorPos)
	}
	if ev.Modified {
		t.Error("no-op move marked the buffer modified")
	}

	ev.CursorLine = 1
	ev.MoveLines(1)
	if got, want := ev.Pt.String(), "one\ntwo"; got != want {
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
	if got, want := ev.Pt.String(), "one\nlast\ntwo"; got != want {
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
	if got, want := ev2.Pt.String(), "one\nlast\ntwo"; got != want {
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
	if got, want := ev.Pt.String(), "two\r\none\r\nthree\r\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}

	// The synthesised terminator at the unterminated end of a CRLF file has
	// to be a CRLF as well.
	ev2 := newDuplicateLineEditor(t, "one\r\nlast")
	ev2.CursorLine = 0
	ev2.CursorPos = 0
	ev2.MoveLines(1)
	if got, want := ev2.Pt.String(), "last\r\none"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_MoveLine_SelectedBlock(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	// "two" and "three" selected, from the middle of one to the middle of
	// the other.
	ev.SelActive = true
	ev.SelAnchorOffset = 5 // line 1, pos 1
	ev.CursorLine = 2
	ev.CursorPos = 2

	ev.MoveLines(1)

	if got, want := ev.Pt.String(), "one\nfour\ntwo\nthree"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 3 || ev.CursorPos != 2 {
		t.Errorf("cursor = line %d pos %d, want line 3 pos 2", ev.CursorLine, ev.CursorPos)
	}
	// The selection still covers the same two lines, now one line lower.
	if want := ev.Li.GetLineOffset(2) + 1; ev.SelAnchorOffset != want {
		t.Errorf("selection anchor = %d, want %d", ev.SelAnchorOffset, want)
	}
}

// A selection made with Shift+Down stops at the start of the line after the
// block; that line is the end of the selection, not a selected line.
func TestEditor_MoveLine_SelectionEndingAtLineStart(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	ev.SelActive = true
	ev.SelAnchorOffset = 4 // start of "two"
	ev.CursorLine = 2      // start of "three"
	ev.CursorPos = 0

	ev.MoveLines(1)

	if got, want := ev.Pt.String(), "one\nthree\ntwo\nfour"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.SelAnchorOffset != ev.Li.GetLineOffset(2) {
		t.Errorf("selection anchor = %d, want %d", ev.SelAnchorOffset, ev.Li.GetLineOffset(2))
	}
	if ev.CursorLine != 3 || ev.CursorPos != 0 {
		t.Errorf("cursor = line %d pos %d, want line 3 pos 0", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditor_MoveLine_RectangularSelection(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	ev.RectSelActive = true
	ev.rectSelStartLine = 1
	ev.rectSelStartCol = 1
	ev.CursorLine = 2
	ev.CursorPos = 2

	ev.MoveLines(-1)

	if got, want := ev.Pt.String(), "two\nthree\none\nfour"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.rectSelStartLine != 0 || ev.CursorLine != 1 {
		t.Errorf("rect selection = lines %d..%d, want 0..1", ev.rectSelStartLine, ev.CursorLine)
	}
}
