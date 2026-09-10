package navtrace

import (
	"github.com/unxed/f4/sdk/extui"
)

func TraceDocumentWindow(surface *extui.SurfaceModel) {
	if !NavigationBenchmarkIsEnabled() {
		return
	}
	contentKey := surface.WindowContentKey
	if contentKey == "" {
		contentKey = extui.WindowRowsContentKey(surface.WindowRows)
	}
	NavigationBenchmarkUIEvent("document.window.projected",
		"documentKey", surface.DocumentKey, "kind", surface.Kind,
		"layoutRevision", surface.LayoutRevision, "geometryRevision", surface.GeometryRevision,
		"generation", surface.WindowGeneration, "requestGeneration", surface.WindowRequestGeneration,
		"columns", surface.ViewportColumns, "viewportRows", surface.ViewportRows,
		"rows", len(surface.WindowRows), "ready", !surface.LayoutPending,
		"windowStart", surface.WindowStart, "windowEnd", surface.WindowEnd,
		"windowContentKey", contentKey)
}
