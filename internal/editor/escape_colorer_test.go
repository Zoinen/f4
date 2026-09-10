package editor

import (
	"github.com/unxed/vtinput"
	"testing"
)

// Escape cancels a running colorer pass. The test builds the view by
// literal because the two flags it needs are set by that pass, not by a
// constructor — which is also why it belongs in this package.

func TestEditorEscapeCancelsColorer(t *testing.T) {
	ch := &ColorerHighlighter{}
	ev := &EditorView{
		Highlighter:     ch,
		colorerIndexing: true,
		colorerCancel:   ch.Cancel,
	}
	e := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}

	if !ev.VetoActionKey(e) {
		t.Fatal("Escape was not reserved for the editor while Colorer is working")
	}
	ev.cancelColorer()
	if !ch.disabled || ev.colorerIndexing || ev.colorerCancel != nil {
		t.Fatalf("Colorer was not cancelled: disabled=%v indexing=%v cancel=%v", ch.disabled, ev.colorerIndexing, ev.colorerCancel != nil)
	}
}
