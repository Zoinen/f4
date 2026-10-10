package editor

import (
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
)

func TestEditorReportsReadAndClose(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	var events []int
	var ids []int
	OnEvent = func(ev *EditorView, event int) {
		events = append(events, event)
		ids = append(ids, ev.MacroID)
	}
	t.Cleanup(func() { OnEvent = nil })

	ev := NewEditorView(piecetable.New([]byte("text")), nil, "a.txt")
	ev.Close()
	ev.Close() // a second Close reports nothing more

	if len(events) != 2 || events[0] != EventRead || events[1] != EventClose {
		t.Fatalf("events = %v, want [read close]", events)
	}
	if ids[0] == 0 || ids[0] != ids[1] {
		t.Errorf("editor ids = %v, want one non-zero id", ids)
	}
	other := NewEditorView(piecetable.New(nil), nil, "b.txt")
	defer other.Close()
	if other.MacroID == ev.MacroID {
		t.Error("two editors share an id")
	}
	OnEvent = nil
	ev.notify(EventSave) // no hook: nothing happens
}
