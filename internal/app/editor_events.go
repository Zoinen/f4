package app

import (
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/viewer"
)

// The editor and the viewer report what happens to it through editor.OnEvent; the Lua macros
// hear it as their EditorEvent and ViewerEvent (f4#1686, Step 7).
func init() {
	viewer.OnEvent = func(vv *viewer.ViewerView, event int) {
		macro.MacroMgr.RaiseViewerEvent(vv.MacroID, event)
	}
	editor.OnEvent = func(ev *editor.EditorView, event int) {
		macro.MacroMgr.RaiseEditorEvent(ev.MacroID, event)
	}
}
