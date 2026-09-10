package viewer

import (
	"github.com/unxed/f4/internal/semantic"
)

func (vv *ViewerView) ApplyNativeViewport(g semantic.NativeDocumentGeometry) {
	if g.Revision != 0 && g.Revision < vv.NativeViewportRevision {
		return
	}
	changed := vv.NativeViewportColumns != g.Columns
	vv.NativeViewportColumns, vv.NativeViewportRows = g.Columns, g.Rows
	vv.NativeViewportRevision = g.Revision
	if changed {
		vv.SemanticLayoutRevision++
		vv.SemanticNeedsReflow = true
		vv.SemanticWrapSeek = SemanticWrapSeekState{}
		vv.SemanticPendingScroll = false
		vv.SemanticPendingGeneration = 0
		vv.LineOffsets = nil
	}
}
