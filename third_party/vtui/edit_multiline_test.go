package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestEditMultilinePasteAndWrapping(t *testing.T) {
	e := NewEdit(0, 0, 9, "")
	e.Multiline, e.WordWrap = true, true
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	for _, r := range "app -argument\r\nnext\n" {
		e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType})
	if got := e.GetText(); got != "app -argument\nnext\n" {
		t.Fatalf("paste=%q", got)
	}
	lines := e.multilineLines(9)
	if len(lines) != 5 || lines[0].end != 4 || !lines[0].soft || lines[2].soft {
		t.Fatalf("wrapped lines=%+v", lines)
	}
	e.WordWrap = false
	if got := e.multilineLines(9)[0].end; got != 8 {
		t.Fatalf("character wrap=%d", got)
	}
	if got := e.GetText(); got != "app -argument\nnext\n" {
		t.Fatal("wrapping changed command")
	}
}

func TestEditMultilineCaretAndHitTest(t *testing.T) {
	SetDefaultPalette()
	e := NewEdit(2, 1, 9, "first\nsecond\nthird")
	e.Multiline = true
	e.SetPosition(2, 1, 10, 2)
	e.SetFocus(true)
	e.ClearSelection()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)
	e.Show(scr)
	if e.multilineTop != 1 {
		t.Fatalf("top=%d", e.multilineTop)
	}
	if got := e.cursorPositionAtPoint(3, 1); got != 7 {
		t.Fatalf("second line hit=%d", got)
	}
	checkCell(t, scr, 2, 1, 's', Palette[e.ColorTextIdx])
	checkCell(t, scr, 2, 2, 't', Palette[e.ColorTextIdx])
}

func TestEditMultilineAltDragUsesRectangularSelection(t *testing.T) {
	e := NewEdit(0, 0, 8, "abcdef\nuvwxyz\n123456")
	e.Multiline = true
	e.SetPosition(0, 0, 7, 2)
	e.ClearSelection()
	press := func(x, y int, flags uint32, modifiers vtinput.ControlKeyState) {
		t.Helper()
		if !e.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX:      int16(x), MouseY: int16(y), MouseEventFlags: flags,
			ControlKeyState: modifiers,
		}) {
			t.Fatal("mouse gesture was not handled")
		}
	}
	press(1, 0, 0, vtinput.LeftAltPressed)
	press(4, 2, vtinput.MouseMoved, 0) // Alt may be released before the drag ends.
	e.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if !e.blockSelection {
		t.Fatal("Alt drag did not remain a block selection")
	}
	if got := e.multilineBlockText(); got != "bcd\nvwx\n234" {
		t.Fatalf("block text=%q", got)
	}
	e.DeleteBlock()
	if got := e.GetText(); got != "aef\nuyz\n156" {
		t.Fatalf("block delete=%q", got)
	}
}

func TestEditNativeBlockSelectionUsesRenderedWrapWidth(t *testing.T) {
	e := NewEdit(0, 0, 39, "abcdefghi")
	e.Multiline, e.WordWrap = true, true
	e.SetPosition(0, 0, 39, 2)
	e.SetMultilineBlockSelectionAt(1, 8, MultilineBlockSelectionGeometry{
		AnchorRow: 0, AnchorColumn: 1,
		FocusRow: 1, FocusColumn: 4,
		WrapWidth: 5,
	})
	if got, want := e.multilineBlockText(), "bcd\nghi"; got != want {
		t.Fatalf("native wrapped block copy=%q, want %q", got, want)
	}
	e.DeleteBlock()
	if got, want := e.GetText(), "aef"; got != want {
		t.Fatalf("native wrapped block delete=%q, want %q", got, want)
	}
}

func TestEditMultilineBlockSelectionYieldsToKeyboardSelection(t *testing.T) {
	e := NewEdit(0, 0, 19, "abcdef\nghijkl\nmnopqr")
	e.Multiline = true
	e.SetPosition(0, 0, 19, 2)
	e.SetMultilineBlockSelection(1, 10)
	if !e.MoveCursorVerticalSelection(-1, true) {
		t.Fatal("Shift+Up did not move from the block selection")
	}
	if e.blockSelection || e.selStart != 3 || e.selEnd != 10 {
		t.Fatalf("keyboard selection block=%v range=[%d,%d], want linear [3,10]", e.blockSelection, e.selStart, e.selEnd)
	}

	e.SetMultilineBlockSelection(1, 10)
	e.SelectAll()
	if e.blockSelection || e.selStart != 0 || e.selEnd != len(e.text) {
		t.Fatalf("SelectAll retained block state: block=%v range=[%d,%d]", e.blockSelection, e.selStart, e.selEnd)
	}

	e.SetMultilineBlockSelection(1, 10)
	e.SetText("replacement")
	if e.blockSelection {
		t.Fatal("SetText retained stale block selection")
	}
}

func TestEditMultilineAltClickWithoutDragDoesNotLeaveEmptyBlock(t *testing.T) {
	e := NewEdit(0, 0, 7, "first\nsecond")
	e.Multiline = true
	e.SetPosition(0, 0, 7, 1)
	if !e.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		ControlKeyState: vtinput.LeftAltPressed,
	}) {
		t.Fatal("Alt-click was not handled")
	}
	e.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if e.blockSelection {
		t.Fatal("Alt-click without a drag left an empty block selection")
	}
}

func TestEditMultilineArgumentBreaksAreVisual(t *testing.T) {
	e := NewEdit(0, 0, 80, `app -a "inside -quoted" --next`)
	e.Multiline, e.WordWrap = true, true
	lines := e.multilineLines(80)
	if len(lines) != 3 {
		t.Fatalf("argument lines=%+v", lines)
	}
	if got := string(e.text[lines[1].start:lines[1].end]); got != `-a "inside -quoted" ` {
		t.Fatalf("quoted argument split: %q", got)
	}
	e.WordWrap = false
	if len(e.multilineLines(80)) != 1 {
		t.Fatal("disabled argument wrapping still splits")
	}
}

func TestEditMultilineVerticalNavigation(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		text                          string
		width, start, direction, want int
		moved                         bool
	}{
		{"literal up", "abc\nx\nabcdef", 80, 9, -1, 5, true},
		{"literal down", "abc\nx\nabcdef", 80, 2, 1, 5, true},
		{"first row", "abc\nx", 80, 2, -1, 2, false},
		{"last row", "abc\nx", 80, 4, 1, 4, false},
		{"wrapped", "abcdefghijk", 5, 10, -1, 6, true},
		{"wide grapheme", "a界b\n1234", 80, 6, -1, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEdit(0, 0, tc.width, tc.text)
			e.Multiline = true
			e.ClearSelection()
			e.curPos = tc.start
			if moved := e.MoveCursorVertical(tc.direction); moved != tc.moved || e.curPos != tc.want {
				t.Fatalf("moved=%v cursor=%d, want %v/%d", moved, e.curPos, tc.moved, tc.want)
			}
			if e.GetText() != tc.text {
				t.Fatal("vertical navigation modified text")
			}
		})
	}
}

func TestEditMultilineShiftVerticalSelection(t *testing.T) {
	e := NewEdit(0, 0, 80, "one\ntwo\nthree")
	e.Multiline = true
	e.ClearSelection()
	e.curPos = 5
	if !e.MoveCursorVerticalSelection(1, true) {
		t.Fatal("Shift+Down did not move to next line")
	}
	if e.selStart != 5 || e.selEnd != 9 || e.curPos != 9 {
		t.Fatalf("selection [%d,%d] cursor=%d, want [5,9]", e.selStart, e.selEnd, e.curPos)
	}
	if !e.MoveCursorVerticalSelection(-1, true) || e.curPos != 5 {
		t.Fatal("Shift+Up did not return to anchor")
	}
}
