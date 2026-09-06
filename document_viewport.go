package main

import (
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// Opt-in timing records identify ready windows without recording file text.
func traceDocumentWindow(surface *extui.SurfaceModel) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	contentKey := surface.WindowContentKey
	if contentKey == "" {
		contentKey = extui.WindowRowsContentKey(surface.WindowRows)
	}
	navigationBenchmarkUIEvent("document.window.projected",
		"documentKey", surface.DocumentKey, "kind", surface.Kind,
		"layoutRevision", surface.LayoutRevision, "geometryRevision", surface.GeometryRevision,
		"generation", surface.WindowGeneration, "requestGeneration", surface.WindowRequestGeneration,
		"columns", surface.ViewportColumns, "viewportRows", surface.ViewportRows,
		"rows", len(surface.WindowRows), "ready", !surface.LayoutPending,
		"windowStart", surface.WindowStart, "windowEnd", surface.WindowEnd,
		"windowContentKey", contentKey)
}

// This is negotiated presentation state, not document data. Qt reports its
// prospective standalone body geometry even while the commander is visible.
// All access after connection setup runs on the UI thread.
type nativeDocumentGeometry struct {
	columns, rows int
	revision      uint64
}

var nativeDocumentViewport struct {
	enabled  bool
	geometry nativeDocumentGeometry
}

func seedNativeDocumentViewport(document any) {
	if !nativeDocumentViewport.enabled || AppConfig.GuiPresentation == GuiPresentationText {
		return
	}
	g := nativeDocumentViewport.geometry
	if g.revision != 0 {
		applyNativeDocumentViewport(document, g)
		navigationBenchmarkUIEvent("document.viewport.seeded", "columns", g.columns, "rows", g.rows, "geometryRevision", g.revision)
	}
}

func nativeDocumentLayoutPending(revision uint64) bool {
	return nativeDocumentViewport.enabled && AppConfig.GuiPresentation != GuiPresentationText && revision == 0
}

func applyNativeDocumentViewport(document any, g nativeDocumentGeometry) {
	switch view := document.(type) {
	case *ViewerView:
		if g.revision != 0 && g.revision < view.nativeViewportRevision {
			return
		}
		changed := view.nativeViewportColumns != g.columns
		view.nativeViewportColumns, view.nativeViewportRows = g.columns, g.rows
		view.nativeViewportRevision = g.revision
		if changed {
			view.semanticLayoutRevision++
			view.semanticNeedsReflow = true
			view.semanticWrapSeek = semanticWrapSeekState{}
			view.semanticPendingScroll = false
			view.semanticPendingGeneration = 0
			view.lineOffsets = nil
		}
	case *EditorView:
		if g.revision != 0 && g.revision < view.nativeViewportRevision {
			return
		}
		view.nativeViewportColumns, view.nativeViewportRows = g.columns, g.rows
		view.nativeViewportRevision = g.revision
		view.ensureEngineWidth()
		if view.scrollBar != nil {
			view.scrollBar.PgStep = view.viewportHeight()
		}
	}
}

func handleStandaloneDocumentViewport(action map[string]any) bool {
	if semanticString(action["action"]) != "document.viewport" ||
		semanticString(action["scope"]) != "standalone" {
		return false
	}
	if !nativeDocumentViewport.enabled {
		return true
	}
	columns, rows := semanticInt(action["columns"]), semanticInt(action["rows"])
	revision := semanticInt64(action["geometryRevision"])
	// Reject malformed geometry rather than allocating unbounded surfaces.
	if columns <= 0 || columns > 16384 || rows <= 0 || rows > 4096 || revision <= 0 {
		return true
	}
	if uint64(revision) <= nativeDocumentViewport.geometry.revision {
		return true
	}
	g := nativeDocumentGeometry{columns: columns, rows: rows, revision: uint64(revision)}
	nativeDocumentViewport.geometry = g
	navigationBenchmarkUIEvent("document.viewport.negotiated", "columns", columns, "rows", rows, "geometryRevision", revision)
	if vtui.FrameManager != nil && AppConfig.GuiPresentation != GuiPresentationText {
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				applyNativeDocumentViewport(frame, g)
			}
		}
	}
	return true
}

// Never build the invisible compatibility document a second time. Offscreen
// styling surfaces and unsupported/modal frame stacks still paint normally.
func nativeDocumentCellPaintOwned(scr *vtui.ScreenBuf) bool {
	if !nativeDocumentViewport.enabled || AppConfig.GuiPresentation == GuiPresentationText ||
		vtui.FrameManager == nil || scr != vtui.FrameManager.Screen() {
		return false
	}
	renderer, ok := scr.Renderer.(*ExtUiRenderer)
	if !ok || !renderer.nativeSemanticSurfaceEnabled {
		return false
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		switch frame.(type) {
		case *ViewerView, *EditorView:
		default:
			if frame.GetType() != vtui.TypeDesktop {
				return false
			}
		}
	}
	navigationBenchmarkUIEvent("document.hidden_cell_paint.skipped")
	return true
}

func targetedDocumentGeometry(action map[string]any, columns int) nativeDocumentGeometry {
	if raw, exists := action["columns"]; exists {
		columns = max(0, min(16384, semanticInt(raw)))
	}
	return nativeDocumentGeometry{
		columns:  columns,
		rows:     max(0, min(4096, semanticInt(action["rows"]))),
		revision: uint64(max(int64(0), semanticInt64(action["geometryRevision"]))),
	}
}

func semanticDocumentLayoutMatches(action map[string]any, documentKey string, revision uint64) bool {
	if key := semanticString(action["documentKey"]); key != "" && key != documentKey {
		return false
	}
	if raw, exists := action["layoutRevision"]; exists {
		return semanticInt64(raw) == int64(revision)
	}
	return true // console and pre-extension clients
}
