package editor

import "sync/atomic"

// Editor events, numbered as Far's EE_* constants so a macro's EditorEvent
// action sees the values it expects (f4#1686, Step 7).
const (
	EventRead  = 0 // the editor has opened its file
	EventSave  = 1 // the file has been saved
	EventClose = 3 // the editor is closing
)

// OnEvent, when set, is told about the editor events above. The application
// wires it to its macro engine; the editor itself knows nothing of macros.
var OnEvent func(ev *EditorView, event int)

var lastEditorID atomic.Int64

// notify passes an event to OnEvent.
func (ev *EditorView) notify(event int) {
	if OnEvent != nil {
		OnEvent(ev, event)
	}
}
