package panel

import (
	"context"
	"github.com/unxed/f4/vfs"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestQuickViewHoverPreservesCursorAndRejectsStaleRequests(t *testing.T) {
	q, source, _ := newQuickViewProviderFixture(t, map[string][]byte{
		"a.txt": []byte("first"), "b.txt": []byte("second"),
	})
	source.updateSemanticRevisions()
	source.catalogRevision = 7
	before := source.GetCursorIndex()
	other := 1 - before
	kind, _ := source.semanticSourceInfo()
	id, _ := source.semanticEntryMetadata(source.Entries[other], kind)
	action := map[string]any{"sourcePanelId": vtui.SemanticID(source),
		"catalogRevision": int64(7), "generation": int64(1), "entryId": id}
	if !q.applyPreviewRequest(action) {
		t.Fatal("preview rejected")
	}
	waitForQuickView(t, func() bool { return !q.cacheLoading })
	model := q.semanticModel(1, 0, false)
	if model.EntryID != id || source.GetCursorIndex() != before {
		t.Fatalf("preview changed cursor or target: %+v", model)
	}
	for _, entry := range source.Entries {
		if entry.Selected {
			t.Fatal("hover selected a file")
		}
	}
	if q.applyPreviewRequest(action) {
		t.Fatal("accepted repeated generation")
	}
	action["generation"] = int64(2)
	action["catalogRevision"] = int64(6)
	if q.applyPreviewRequest(action) {
		t.Fatal("accepted stale catalog")
	}
	action["catalogRevision"] = int64(7)
	action["entryId"] = ""
	if !q.applyPreviewRequest(action) {
		t.Fatal("clear rejected")
	}
	if q.semanticModel(1, 0, false).Name != source.Entries[before].Name {
		t.Fatal("clear did not restore cursor preview")
	}
}

func TestQuickViewNativeImageDoesNotDecodeOrSerialize(t *testing.T) {
	q, _, _ := newQuickViewProviderFixture(t, map[string][]byte{"broken.png": []byte("not a PNG")})
	q.setNativeImages(true)
	model := q.semanticModel(1, 0, false)
	if model.PreviewKind != "image" || model.ImageRenderer != "gallery" ||
		model.Loading || model.ImageSource != "" || q.imageSurf != nil || q.cacheReadErr != nil {
		t.Fatalf("native preview entered Go decoding: %+v", model)
	}
	q.setNativeImages(false)
	q.semanticModel(1, 0, false)
	waitForQuickView(t, func() bool { return q.cacheReadErr != nil })
	if q.semanticModel(1, 0, false).PreviewKind != "error" {
		t.Fatal("built-in mode did not decode the original file")
	}
}

func TestQuickViewHoverSupersedesAsyncPreview(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	provider := &quickViewTestProvider{name: "hover-cancel", priority: 1000,
		match: func(r vfs.QuickViewRequest) bool { return strings.HasSuffix(r.Path, ".hovercancel") },
		preview: func(ctx context.Context, r vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return vfs.QuickViewResult{Lines: []string{"obsolete"}}, nil
		}}
	registerQuickViewTestProvider(t, provider)
	q, source, _ := newQuickViewProviderFixture(t, map[string][]byte{"cursor.txt": []byte("cursor"), "hover.hovercancel": []byte("slow")})
	hover := 0
	for i, entry := range source.Entries {
		if entry.Name == "cursor.txt" {
			source.CursorIdx = i
		} else {
			hover = i
		}
	}
	source.updateSemanticRevisions()
	kind, _ := source.semanticSourceInfo()
	id, _ := source.semanticEntryMetadata(source.Entries[hover], kind)
	action := map[string]any{"sourcePanelId": vtui.SemanticID(source), "catalogRevision": source.catalogRevision, "entryId": id, "generation": int64(1)}
	if !q.applyPreviewRequest(action) {
		t.Fatal("hover rejected")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("hover work did not start")
	}
	action["entryId"], action["generation"] = "", int64(2)
	if !q.applyPreviewRequest(action) {
		t.Fatal("clear rejected")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("obsolete work was not cancelled")
	}
	waitForQuickView(t, func() bool { return !q.cacheLoading })
	if q.semanticModel(1, 0, false).Name != "cursor.txt" || strings.Contains(strings.Join(q.cacheLines, ""), "obsolete") {
		t.Fatal("stale hover replaced cursor preview")
	}
}

func TestQuickViewCursorAndCatalogChangesClearHover(t *testing.T) {
	for _, changeCatalog := range []bool{false, true} {
		q, source, _ := newQuickViewProviderFixture(t, map[string][]byte{"a.txt": []byte("a"), "b.txt": []byte("b")})
		source.updateSemanticRevisions()
		other := 1 - source.GetCursorIndex()
		kind, _ := source.semanticSourceInfo()
		id, _ := source.semanticEntryMetadata(source.Entries[other], kind)
		if !q.applyPreviewRequest(map[string]any{"sourcePanelId": vtui.SemanticID(source), "catalogRevision": source.catalogRevision, "entryId": id, "generation": int64(1)}) {
			t.Fatal("hover rejected")
		}
		if changeCatalog {
			source.catalogRevision++
		} else {
			source.CursorIdx = other
		}
		q.prepareSelection()
		if q.previewEntryID != "" {
			t.Fatal("hover survived source navigation")
		}
	}
}
