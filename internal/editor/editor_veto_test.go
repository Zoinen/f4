package editor

import (
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// While autocomplete is active, Esc must dismiss the autocomplete list,
// not close the editor: the frame vetoes hotkey dispatch and handles
// the key in its own ProcessKey.
func TestEditorView_AutocompleteEscVeto(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	Pt := piecetable.New([]byte("foo"))
	ev := NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.acEnabled = true
	ev.acMatches = []string{"foobar"}
	ev.acPrefix = "foo"

	testutil.PressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}, nil)

	if ev.acMatches != nil {
		t.Error("Esc should dismiss the autocomplete list")
	}
	if ev.IsDone() {
		t.Error("Editor must stay open after Esc dismissed autocomplete")
	}
}
