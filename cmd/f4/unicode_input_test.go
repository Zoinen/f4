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
	ev.ProcessKey(unicodeKey(vtinput.VK_RIGHT, 0))
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
