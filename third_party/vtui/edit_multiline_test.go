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

func TestEditMultilineArgumentBreaksAreVisual(t *testing.T) {
	e := NewEdit(0,0,80,`app -a "inside -quoted" --next`)
	e.Multiline, e.WordWrap = true, true
	lines := e.multilineLines(80)
	if len(lines) != 3 { t.Fatalf("argument lines=%+v", lines) }
	if got := string(e.text[lines[1].start:lines[1].end]); got != `-a "inside -quoted" ` {
		t.Fatalf("quoted argument split: %q",got)
	}
	e.WordWrap = false
	if len(e.multilineLines(80)) != 1 { t.Fatal("disabled argument wrapping still splits") }
}
