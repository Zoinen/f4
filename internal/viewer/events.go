package viewer

import "sync/atomic"

// Viewer events, numbered as Far's VE_* constants so a macro's ViewerEvent
// action sees the values it expects (f4#1686, Step 7).
const (
	EventRead  = 0 // the viewer has opened its file
	EventClose = 1 // the viewer is closing
)

// OnEvent, when set, is told about the viewer events above. The application
// wires it to its macro engine; the viewer itself knows nothing of macros.
var OnEvent func(vv *ViewerView, event int)

var lastViewerID atomic.Int64

// notify passes an event to OnEvent.
func (vv *ViewerView) notify(event int) {
	if OnEvent != nil {
		OnEvent(vv, event)
	}
}
