package editor

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"reflect"
	"testing"
)

func shiftKey(ev *EditorView, vk uint16) {
	ev.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk,
		ControlKeyState: vtinput.ShiftPressed,
	})
}

// extraCaretSelections is what each secondary caret has selected, as ordered
// pairs, for carets that have a selection at all.
func extraCaretSelections(ev *EditorView) [][2]int {
	out := make([][2]int, 0, len(ev.extraCursors))
	for _, caret := range ev.extraCursors {
		if start, end := caret.selRange(); end > start {
			out = append(out, [2]int{start, end})
		}
	}
	return out
}

func TestEditor_MultiCursor_ShiftMovementSelectsAtEveryCaret(t *testing.T) {
	ev := multiCursorEditor(t, "abcd\nabcd")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.ToggleCursorAt(6)

	shiftKey(ev, vtinput.VK_RIGHT)
	shiftKey(ev, vtinput.VK_RIGHT)

	if !ev.SelActive {
		t.Fatal("the primary caret has no selection")
	}
	if start, end := ev.GetSelectionRange(); start != 1 || end != 3 {
		t.Errorf("primary selection = [%d %d), want [1 3)", start, end)
	}
	if got, want := extraCaretSelections(ev), [][2]int{{6, 8}}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra selections = %v, want %v", got, want)
	}
}

// Moving without Shift lets go of the text, at every caret.
func TestEditor_MultiCursor_MovingWithoutShiftDropsSelections(t *testing.T) {
	ev := multiCursorEditor(t, "abcd\nabcd")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.ToggleCursorAt(6)
	shiftKey(ev, vtinput.VK_RIGHT)

	arrowKey(ev, vtinput.VK_RIGHT)

	if ev.SelActive {
		t.Error("the primary selection survived a plain arrow")
	}
	if got := extraCaretSelections(ev); len(got) != 0 {
		t.Errorf("extra selections = %v, want none", got)
	}
}

// Selecting back to where a caret started leaves nothing selected.
func TestEditor_MultiCursor_EmptySelectionIsNoSelection(t *testing.T) {
	ev := multiCursorEditor(t, "abcd\nabcd")
	ev.CursorLine = 0
	ev.CursorPos = 1
	ev.ToggleCursorAt(6)

	shiftKey(ev, vtinput.VK_RIGHT)
	shiftKey(ev, vtinput.VK_LEFT)

	if ev.SelActive {
		t.Error("the primary caret still reports a selection")
	}
	if got := extraCaretSelections(ev); len(got) != 0 {
		t.Errorf("extra selections = %v, want none", got)
	}
}

func TestEditor_MultiCursor_TypingReplacesEverySelection(t *testing.T) {
	ev := multiCursorEditor(t, "xxab\nxxab")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(5)

	shiftKey(ev, vtinput.VK_RIGHT)
	shiftKey(ev, vtinput.VK_RIGHT)
	typeCharAtCarets(ev, 'Z')

	if got, want := ev.Pt.String(), "Zab\nZab"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	// The carets come out of the edit as bare carets, sitting after what
	// they typed.
	if ev.SelActive {
		t.Error("a selection survived the edit that replaced it")
	}
	if got := extraCaretSelections(ev); len(got) != 0 {
		t.Errorf("extra selections = %v, want none", got)
	}
	if ev.CursorPos != 1 {
		t.Errorf("primary caret pos = %d, want 1", ev.CursorPos)
	}
	if got, want := extraCaretOffsets(ev), []int{5}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra carets = %v, want %v", got, want)
	}
}

// With a selection, Backspace and Del each remove exactly that and no more.
func TestEditor_MultiCursor_DeletingRemovesEverySelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  uint16
	}{
		{"backspace", vtinput.VK_BACK},
		{"delete", vtinput.VK_DELETE},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ev := multiCursorEditor(t, "xxab\nxxab")
			ev.CursorLine = 0
			ev.CursorPos = 0
			ev.ToggleCursorAt(5)

			shiftKey(ev, vtinput.VK_RIGHT)
			shiftKey(ev, vtinput.VK_RIGHT)
			pressVKAtCarets(ev, tc.key)

			if got, want := ev.Pt.String(), "ab\nab"; got != want {
				t.Fatalf("buffer = %q, want %q", got, want)
			}
			if got, want := extraCaretOffsets(ev), []int{3}; !reflect.DeepEqual(got, want) {
				t.Errorf("extra carets = %v, want %v", got, want)
			}
		})
	}
}

func TestEditor_MultiCursor_EnterReplacesEverySelection(t *testing.T) {
	ev := multiCursorEditor(t, "ab\nab")
	ev.AutoIndent = false
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(3)

	shiftKey(ev, vtinput.VK_RIGHT)
	pressVKAtCarets(ev, vtinput.VK_RETURN)

	if got, want := ev.Pt.String(), "\nb\n\nb"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

// One Undo takes back the whole replacement, and brings back the carets that
// made it along with what they had selected.
func TestEditor_MultiCursor_UndoRestoresSelections(t *testing.T) {
	ev := multiCursorEditor(t, "xxab\nxxab")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(5)
	shiftKey(ev, vtinput.VK_RIGHT)
	shiftKey(ev, vtinput.VK_RIGHT)

	typeCharAtCarets(ev, 'Z')
	ev.Undo()

	if got, want := ev.Pt.String(), "xxab\nxxab"; got != want {
		t.Fatalf("buffer after undo = %q, want %q", got, want)
	}
	if got, want := extraCaretSelections(ev), [][2]int{{5, 7}}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra selections after undo = %v, want %v", got, want)
	}
}

// Two carets on one line both select back to its start and so end up on the
// same offset; the caret that remains keeps the longer reach, not the shorter.
func TestEditor_MultiCursor_MergingKeepsTheWiderSelection(t *testing.T) {
	ev := multiCursorEditor(t, "abcdef")
	ev.CursorLine = 0
	ev.CursorPos = 3
	ev.ToggleCursorAt(4)

	shiftKey(ev, vtinput.VK_HOME)

	if ev.MultiCursor() {
		t.Fatalf("extra carets = %v, want the two to have merged", extraCaretOffsets(ev))
	}
	if !ev.SelActive {
		t.Fatal("the merged caret lost its selection")
	}
	if start, end := ev.GetSelectionRange(); start != 0 || end != 4 {
		t.Errorf("merged selection = [%d %d), want [0 4)", start, end)
	}
}

func TestEditor_MultiCursor_SelectionsArePainted(t *testing.T) {
	ev := multiCursorEditor(t, "abcd\nabcd")
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.ToggleCursorAt(5)
	shiftKey(ev, vtinput.VK_RIGHT)
	shiftKey(ev, vtinput.VK_RIGHT)
	ev.SetFocus(true)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 13)
	ev.Show(scr)

	selAttr := vtui.Palette[vtui.ColDialogEditSelected]
	// The second line's selection covers its first two cells; the third is
	// ordinary text.
	for x := 0; x < 2; x++ {
		if got := scr.GetCell(x, 2).Attributes; got != selAttr {
			t.Errorf("cell (%d,2) = %x, want the selection colour %x", x, got, selAttr)
		}
	}
	// Column 2 carries the caret itself, which is marked the same way, so
	// the first ordinary cell is the one after it.
	if got := scr.GetCell(3, 2).Attributes; got == selAttr {
		t.Error("the selection spilled past the end of the range")
	}
}
