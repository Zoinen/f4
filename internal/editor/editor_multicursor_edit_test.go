package editor

import (
	"github.com/unxed/vtinput"
	"reflect"
	"testing"
)

func typeCharAtCarets(ev *EditorView, ch rune) {
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ch})
}

func pressVKAtCarets(ev *EditorView, vk uint16) {
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
}

func TestEditor_MultiCursor_TypingInsertsAtEveryCaret(t *testing.T) {
	ev := multiCursorEditor(t, "one\ntwo\nsix")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(4)
	ev.ToggleCursorAt(8)

	typeCharAtCarets(ev, 'X')

	if got, want := ev.Pt.String(), "Xone\nXtwo\nXsix"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	// Every caret moved past what it typed, so the next keystroke continues
	// the word rather than typing over it.
	if ev.CursorLine != 0 || ev.CursorPos != 1 {
		t.Errorf("primary caret = line %d pos %d, want line 0 pos 1", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{6, 11}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}

	typeCharAtCarets(ev, 'Y')
	if got, want := ev.Pt.String(), "XYone\nXYtwo\nXYsix"; got != want {
		t.Fatalf("buffer after the second keystroke = %q, want %q", got, want)
	}
}

// The whole set of insertions is one change: one Undo takes all of it back.
func TestEditor_MultiCursor_TypingIsOneUndoStep(t *testing.T) {
	ev := multiCursorEditor(t, "one\ntwo")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(4)

	typeCharAtCarets(ev, 'X')
	if got, want := ev.Pt.String(), "Xone\nXtwo"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}

	ev.Undo()
	if got, want := ev.Pt.String(), "one\ntwo"; got != want {
		t.Fatalf("buffer after undo = %q, want %q", got, want)
	}
	// And the set that made the edit comes back with it, so it can be redone
	// or retyped.
	if got, want := extraCaretOffsets(ev), []int{4}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets after undo = %v, want %v", got, want)
	}

	ev.Redo()
	if got, want := ev.Pt.String(), "Xone\nXtwo"; got != want {
		t.Errorf("buffer after redo = %q, want %q", got, want)
	}
}

func TestEditor_MultiCursor_Backspace(t *testing.T) {
	ev := multiCursorEditor(t, "abc\nabc")
	ev.CursorLine = 0
	ev.CursorPos = 2 // offset 2, after "ab"
	ev.ToggleCursorAt(6)

	pressVKAtCarets(ev, vtinput.VK_BACK)

	if got, want := ev.Pt.String(), "ac\nac"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 1 {
		t.Errorf("primary caret = line %d pos %d, want line 0 pos 1", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{4}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

// A caret at the start of the buffer has nothing to delete, and must not be
// lost because of it.
func TestEditor_MultiCursor_BackspaceAtStartKeepsCaret(t *testing.T) {
	ev := multiCursorEditor(t, "ab\ncd")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(4)

	pressVKAtCarets(ev, vtinput.VK_BACK)

	if got, want := ev.Pt.String(), "ab\nd"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 0 {
		t.Errorf("primary caret = line %d pos %d, want line 0 pos 0", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{3}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

func TestEditor_MultiCursor_Delete(t *testing.T) {
	ev := multiCursorEditor(t, "abc\nabc")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.ToggleCursorAt(5)

	pressVKAtCarets(ev, vtinput.VK_DELETE)

	if got, want := ev.Pt.String(), "ac\nac"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 1 {
		t.Errorf("primary caret = line %d pos %d, want line 0 pos 1", ev.CursorLine, ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{4}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

// Two carets one character apart both reach for the same byte; it may only be
// deleted once, and the carets that meet become one.
func TestEditor_MultiCursor_AdjacentDeletionsDoNotOverlap(t *testing.T) {
	ev := multiCursorEditor(t, "abcd")
	ev.CursorLine = 0
	ev.CursorPos = 2
	ev.ToggleCursorAt(3)

	pressVKAtCarets(ev, vtinput.VK_BACK)

	if got, want := ev.Pt.String(), "ad"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorPos != 1 {
		t.Errorf("primary caret pos = %d, want 1", ev.CursorPos)
	}
	if len(extraCaretOffsets(ev)) != 0 {
		t.Errorf("extra carets = %v, want the two carets to have merged", extraCaretOffsets(ev))
	}
}

func TestEditor_MultiCursor_EnterKeepsIndentPerCaret(t *testing.T) {
	ev := multiCursorEditor(t, "  ab\n\tcd")
	ev.AutoIndent = true
	ev.CursorLine = 0
	ev.CursorPos = 4 // end of "  ab"
	ev.ToggleCursorAt(8)

	pressVKAtCarets(ev, vtinput.VK_RETURN)

	if got, want := ev.Pt.String(), "  ab\n  \n\tcd\n\t"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_MultiCursor_TabUsesEachCaretsColumn(t *testing.T) {
	ev := multiCursorEditor(t, "ab\nabcd")
	ev.ExpandTabs = 1
	ev.TabSize = 4
	ev.CursorLine = 0
	ev.CursorPos = 2 // column 2 -> two spaces
	ev.ToggleCursorAt(7)

	pressVKAtCarets(ev, vtinput.VK_TAB)

	// The second caret sits at column 4, a tab stop, so it gets a full tab
	// width rather than the two spaces the first one needed.
	if got, want := ev.Pt.String(), "ab  \nabcd    "; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func TestEditor_MultiCursor_OvertypeReplacesAtEveryCaret(t *testing.T) {
	ev := multiCursorEditor(t, "abc\nabc")
	ev.Overtype = true
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(4)

	typeCharAtCarets(ev, 'X')

	if got, want := ev.Pt.String(), "Xbc\nXbc"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

// Overtype at the end of a line types over nothing: the line break survives.
func TestEditor_MultiCursor_OvertypeKeepsLineBreaks(t *testing.T) {
	ev := multiCursorEditor(t, "ab\nab")
	ev.Overtype = true
	ev.CursorLine = 0
	ev.CursorPos = 2
	ev.ToggleCursorAt(5)

	typeCharAtCarets(ev, 'X')

	if got, want := ev.Pt.String(), "abX\nabX"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

// Keys that are still single-caret collapse the set rather than acting through
// half of it.
func TestEditor_MultiCursor_UnsupportedKeysStillCollapse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event vtinput.InputEvent
	}{
		{"page down", vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT}},
		{"shift+page down", vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT,
			ControlKeyState: vtinput.ShiftPressed,
		}},
		{"ctrl+left", vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT,
			ControlKeyState: vtinput.LeftCtrlPressed,
		}},
		{"ctrl+backspace", vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK,
			ControlKeyState: vtinput.LeftCtrlPressed,
		}},
		{"shift+tab", vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB,
			ControlKeyState: vtinput.ShiftPressed,
		}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ev := multiCursorEditor(t, "one\ntwo")
			ev.CursorLine = 0
			ev.CursorPos = 1
			ev.ToggleCursorAt(5)

			event := tc.event
			ev.ProcessKey(&event)

			if ev.MultiCursor() {
				t.Errorf("extra carets survived %s: %v", tc.name, extraCaretOffsets(ev))
			}
		})
	}
}

func TestEditor_MultiCursor_PlainClickCollapsesTheSet(t *testing.T) {
	ev := multiCursorEditor(t, "one\ntwo\nthree")
	ev.ToggleCursorAt(4)

	ev.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      2,
		MouseY:      3,
	})

	if ev.MultiCursor() {
		t.Errorf("a plain click left extra carets behind: %v", extraCaretOffsets(ev))
	}
}

// Typing through carets on the same line has to leave them spread out the way
// they were, not piled up by the shifting offsets of their neighbours.
func TestEditor_MultiCursor_SameLineCaretsStayApart(t *testing.T) {
	ev := multiCursorEditor(t, "a.b.c")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.ToggleCursorAt(3)

	typeCharAtCarets(ev, '!')

	if got, want := ev.Pt.String(), "a!.b!.c"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if ev.CursorPos != 2 {
		t.Errorf("primary caret pos = %d, want 2", ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{5}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}
