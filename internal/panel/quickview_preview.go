package panel

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// Quick View presentation requests never mutate the source panel's cursor.
// The native host owns its preferences; these fields last only for this frame.
func (pf *PanelsFrame) handleQuickViewPresentation(action map[string]any) bool {
	name := semantic.String(action["action"])
	if name == "quickView.configure" {
		if semantic.String(action["target"]) != vtui.SemanticID(pf) {
			return false
		}
		pf.quickViewNativeImages = semantic.Bool(action["nativeImages"])
		for _, alt := range pf.AltPanels {
			if q, ok := alt.(*QuickViewPanel); ok {
				q.setNativeImages(pf.quickViewNativeImages)
			}
		}
		return true
	}
	for _, alt := range pf.AltPanels {
		q, ok := alt.(*QuickViewPanel)
		if !ok || semantic.String(action["target"]) != vtui.SemanticID(q) {
			continue
		}
		return q.applyPreviewRequest(action)
	}
	return false
}

func (q *QuickViewPanel) setNativeImages(native bool) {
	if q.nativeImages == native {
		return
	}
	q.nativeImages = native
	q.cacheValid = false
	q.imageLoadGen++
	q.imageSurf = nil
	q.clearSemanticImage()
	vtui.DebugLog("QUICKVIEW: native images=%v", native)
}

func (q *QuickViewPanel) applyPreviewRequest(action map[string]any) bool {
	if q.src == nil || q.src.Vfs == nil ||
		semantic.String(action["sourcePanelId"]) != vtui.SemanticID(q.src) ||
		semantic.Int64(action["catalogRevision"]) != q.src.catalogRevision {
		return false
	}
	generation := semantic.Int64(action["generation"])
	if generation <= q.previewRequestGeneration {
		return false
	}
	entryID := semantic.String(action["entryId"])
	if entryID != "" {
		if _, ok := q.src.semanticEntryIndex(action); !ok {
			return false
		}
	}
	q.previewRequestGeneration = generation
	q.previewEntryID = entryID
	q.previewCatalogRevision = q.src.catalogRevision
	q.previewCursorIndex = q.src.GetCursorIndex()
	q.prepareSelection()
	vtui.DebugLog("QUICKVIEW: preview generation=%d entry=%q", generation, entryID)
	return true
}

func (q *QuickViewPanel) previewIndex() int {
	index := q.src.GetCursorIndex()
	if q.previewEntryID == "" {
		return index
	}
	if q.previewCatalogRevision != q.src.catalogRevision || index != q.previewCursorIndex {
		q.previewEntryID = ""
		return index
	}
	preview, ok := q.src.semanticEntryIndex(map[string]any{
		"entryId": q.previewEntryID, "catalogRevision": q.previewCatalogRevision,
	})
	if !ok {
		q.previewEntryID = ""
		return index
	}
	return preview
}
