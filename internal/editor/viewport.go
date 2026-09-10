package editor

import (
	"github.com/unxed/f4/internal/semantic"
)

func (ev *EditorView) ApplyNativeViewport(g semantic.NativeDocumentGeometry) {
	if g.Revision != 0 && g.Revision < ev.NativeViewportRevision {
		return
	}
	ev.NativeViewportColumns, ev.NativeViewportRows = g.Columns, g.Rows
	ev.NativeViewportRevision = g.Revision
	ev.EnsureEngineWidth()
	if ev.ScrollBar != nil {
		ev.ScrollBar.PgStep = ev.ViewportHeight()
	}
}
