package main

import (
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newDuplicateLineEditor(t *testing.T, text string) *EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "test.txt")
	t.Cleanup(ev.Close)
	ev.SetPosition(0, 0, 79, 24)
	return ev
}

func TestEditor_DuplicateLine_CurrentLine(t *testing.T) {
	ev := newDuplicateLineEditor(t, "line1\nline2\nline3")
	ev.CursorLine = 1
	ev.CursorPos = 2

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "line1\nline2\nline2\nline3"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	// The cursor follows the copy, so holding the key makes a run of copies
	// rather than repeatedly duplicating the same original line.
	if ev.CursorLine != 2 || ev.CursorPos != 2 {
		t.Errorf("cursor = line %d pos %d, want line 2 pos 2", ev.CursorLine, ev.CursorPos)
	}
	if !ev.modified {
		t.Error("buffer not marked modified")
	}

	ev.Undo()
	if got, want := ev.pt.String(), "line1\nline2\nline3"; got != want {
		t.Errorf("buffer after undo = %q, want %q", got, want)
	}
}

func TestEditor_DuplicateLine_LastLineWithoutTerminator(t *testing.T) {
	ev := newDuplicateLineEditor(t, "line1\nlast")
	ev.CursorLine = 1
	ev.CursorPos = 4

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "line1\nlast\nlast"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 2 || ev.CursorPos != 4 {
		t.Errorf("cursor = line %d pos %d, want line 2 pos 4", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditor_DuplicateLine_KeepsCRLF(t *testing.T) {
	ev := newDuplicateLineEditor(t, "line1\r\nline2\r\n")
	ev.CursorLine = 1
	ev.CursorPos = 0

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "line1\r\nline2\r\nline2\r\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}

	// The unterminated final line has to borrow the file's own line ending
	// instead of introducing a lone LF into a CRLF file.
	ev2 := newDuplicateLineEditor(t, "line1\r\nlast")
	ev2.CursorLine = 1
	ev2.CursorPos = 0
	ev2.DuplicateLines()
	if got, want := ev2.pt.String(), "line1\r\nlast\r\nlast"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_DuplicateLine_Selection(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree\nfour")
	// Select from the middle of "one" to the middle of "two".
	ev.selActive = true
	ev.selAnchorOffset = 1
	ev.CursorLine = 1
	ev.CursorPos = 1

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "one\ntwo\none\ntwo\nthree\nfour"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 3 || ev.CursorPos != 1 {
		t.Errorf("cursor = line %d pos %d, want line 3 pos 1", ev.CursorLine, ev.CursorPos)
	}
	// The selection moves onto the copy, keeping the same two lines covered.
	if !ev.selActive || ev.selAnchorOffset != 9 {
		t.Errorf("selection = active %v anchor %d, want active anchor 9", ev.selActive, ev.selAnchorOffset)
	}
}

// A selection that stops exactly at the start of a line must not drag that
// line into the copy: Shift+Down over one line duplicates one line.
func TestEditor_DuplicateLine_SelectionEndingAtLineStart(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree")
	ev.selActive = true
	ev.selAnchorOffset = 0
	ev.CursorLine = 1
	ev.CursorPos = 0

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "one\none\ntwo\nthree"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_DuplicateLine_RectangularSelection(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\ntwo\nthree")
	ev.rectSelActive = true
	ev.rectSelStartLine = 0
	ev.rectSelStartCol = 1
	ev.CursorLine = 1
	ev.CursorPos = 2

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "one\ntwo\none\ntwo\nthree"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.rectSelStartLine != 2 {
		t.Errorf("rect selection start line = %d, want 2", ev.rectSelStartLine)
	}
	if ev.CursorLine != 3 || ev.CursorPos != 2 {
		t.Errorf("cursor = line %d pos %d, want line 3 pos 2", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditor_DuplicateLine_EmptyLine(t *testing.T) {
	ev := newDuplicateLineEditor(t, "one\n\ntwo")
	ev.CursorLine = 1
	ev.CursorPos = 0

	ev.DuplicateLines()

	if got, want := ev.pt.String(), "one\n\n\ntwo"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_DuplicateLine_Hotkey(t *testing.T) {
	action, ok := GetAction("Editor.DuplicateLine")
	if !ok {
		t.Fatal("Editor.DuplicateLine is not registered")
	}
	if action.Area != "Editor" || action.MenuPath != "Edit" ||
		len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "CtrlShiftD" {
		t.Fatalf("registration = %+v; want Editor/Edit bound to CtrlShiftD", action)
	}

	ev := newDuplicateLineEditor(t, "line1\nline2")
	ev.CursorLine = 0
	ev.CursorPos = 0

	pressKey(ev, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_D,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})

	if got, want := ev.pt.String(), "line1\nline1\nline2"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}
