package main

import (
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func unicodeKey(vk uint16, ch rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vk,
		Char:           ch,
	}
}

func TestConfigureUnicodeInputEnablesFullCaretMode(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	defer func() { vtui.DefaultBidiMode = oldMode }()

	vtui.DefaultBidiMode = vtui.BidiDisplay
	configureUnicodeInput()
	if vtui.DefaultBidiMode != vtui.BidiFull {
		t.Fatalf("f4 input mode = %v, want BidiFull", vtui.DefaultBidiMode)
	}
}

func TestCommandLineUnicodeInputFollowsVisualCaret(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	defer func() { vtui.DefaultBidiMode = oldMode }()
	configureUnicodeInput()

	line := NewCommandLine("")
	line.Edit.SetText("שלום")
	line.Edit.ClearSelection()
	line.ProcessKey(unicodeKey(vtinput.VK_RIGHT, 0))
	line.ProcessKey(unicodeKey(0, 'X'))
	if got := line.Edit.GetText(); got != "שלוXם" {
		t.Fatalf("visual caret inserted at logical text %q, want %q", got, "שלוXם")
	}
}

func TestEditorUnicodeInputFollowsVisualCaret(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	defer func() { vtui.DefaultBidiMode = oldMode }()
	configureUnicodeInput()

	ev := NewEditorView(piecetable.New([]byte("שלום")), nil, "unicode.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)
	ev.CursorPos = len([]byte("שלום"))
	ev.SetFocus(true)
	// The editor moves the caret logically: Left from the end of the line
	// steps back over the last letter, wherever it is drawn.
	ev.ProcessKey(unicodeKey(vtinput.VK_LEFT, 0))
	ev.ProcessKey(unicodeKey(0, 'X'))
	data, _ := ev.pt.Bytes()
	if got := string(data); got != "שלוXם" {
		t.Fatalf("editor inserted at logical text %q, want %q", got, "שלוXם")
	}
}

func TestEditorUnicodeInputBackspaceRemovesGraphemeCluster(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	defer func() { vtui.DefaultBidiMode = oldMode }()
	configureUnicodeInput()

	text := "e\u0301x"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "combining.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)
	ev.CursorPos = len([]byte(text))
	ev.SetFocus(true)
	ev.ProcessKey(unicodeKey(vtinput.VK_BACK, 0))
	data, _ := ev.pt.Bytes()
	if got := string(data); got != "e\u0301" {
		t.Fatalf("backspace split the grapheme input: %q, want %q", got, "e\u0301")
	}
	ev.ProcessKey(unicodeKey(vtinput.VK_BACK, 0))
	data, _ = ev.pt.Bytes()
	if got := string(data); got != "" {
		t.Fatalf("second backspace left the combining cluster: %q", got)
	}
}

func TestEditorUnicodeInputBackspaceKeepsTerminalModifierRuleInBidiMode(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })

	text := "ދިވެހިބަސް"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "divehi.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)
	ev.CursorPos = len([]byte(text))
	ev.SetFocus(true)
	ev.ProcessKey(unicodeKey(vtinput.VK_BACK, 0))
	data, _ := ev.pt.Bytes()
	if got, want := string(data), "ދިވެހިބަސ"; got != want {
		t.Fatalf("backspace in BidiFull mode = %q, want %q", got, want)
	}
}

func TestEditorUnicodeInputDeleteKeepsLogicalDirectionInBidiMode(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })

	text := "ދިވެހިބަސް"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "divehi.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)
	ev.CursorPos = 0
	ev.SetFocus(true)
	ev.ProcessKey(unicodeKey(vtinput.VK_DELETE, 0))
	data, _ := ev.pt.Bytes()
	if got, want := string(data), "ވެހިބަސް"; got != want {
		t.Fatalf("delete in BidiFull mode = %q, want %q", got, want)
	}
}

func TestEditorBidiArrowKeysMoveLogically(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })

	// Left and Right walk the text in logical order, as in Notepad
	// (unxed/f4#546): entering the Hebrew word the caret jumps to its right
	// edge and then moves left through it while the byte offset grows.
	text := "abc אבג def"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "mixed-bidi.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)
	ev.CursorPos = len([]byte("abc "))
	ev.SetFocus(true)

	type step struct{ pos, col int }
	want := []step{
		{len([]byte("abc א")), 6},
		{len([]byte("abc אב")), 5},
		{len([]byte("abc אבג")), 4},
		{len([]byte("abc אבג ")), 8},
	}
	for i, w := range want {
		ev.ProcessKey(unicodeKey(vtinput.VK_RIGHT, 0))
		if ev.CursorPos != w.pos {
			t.Fatalf("right %d: cursor at byte %d, want %d", i, ev.CursorPos, w.pos)
		}
		if _, col := ev.engine.LogicalToVisual(ev.CursorPos); col != w.col {
			t.Fatalf("right %d: caret drawn at column %d, want %d", i, col, w.col)
		}
	}
	for i := len(want) - 2; i >= 0; i-- {
		ev.ProcessKey(unicodeKey(vtinput.VK_LEFT, 0))
		if ev.CursorPos != want[i].pos {
			t.Fatalf("left to %d: cursor at byte %d, want %d", i, ev.CursorPos, want[i].pos)
		}
	}
	ev.ProcessKey(unicodeKey(vtinput.VK_LEFT, 0))
	if got, w := ev.CursorPos, len([]byte("abc ")); got != w {
		t.Fatalf("left out of the RTL run: cursor at byte %d, want %d", got, w)
	}
}
