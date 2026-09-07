package main

import (
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// While autocomplete is active, Esc must dismiss the autocomplete list,
// not close the editor: the frame vetoes hotkey dispatch and handles
// the key in its own ProcessKey.
func TestEditorView_AutocompleteEscVeto(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pt := piecetable.New([]byte("foo"))
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.acEnabled = true
	ev.acMatches = []string{"foobar"}
	ev.acPrefix = "foo"

	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	})

	if ev.acMatches != nil {
		t.Error("Esc should dismiss the autocomplete list")
	}
	if ev.IsDone() {
		t.Error("Editor must stay open after Esc dismissed autocomplete")
	}
}

// Without an active modal input state, Esc reaches the hotkey
// dispatcher and runs the Editor.Quit action.
func TestEditorView_EscRunsQuitAction(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pt := piecetable.New([]byte("foo"))
	ev := NewEditorView(pt, nil, "test.txt")

	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	})

	if !ev.IsDone() {
		t.Error("Editor should be closed by the Editor.Quit action")
	}
}
