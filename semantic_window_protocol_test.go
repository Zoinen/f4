package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func comparableSemanticRow(row map[string]any) map[string]any {
	result := make(map[string]any, len(row)-1)
	for key, value := range row {
		// Index is local to the bounded array. Absolute byte/visual extents are
		// the stable identity shared by two overlapping windows.
		if key != "index" {
			result[key] = value
		}
	}
	return result
}

func semanticRowsByExtent(t *testing.T, node map[string]any, unit string) map[int64]map[string]any {
	t.Helper()
	result := make(map[int64]map[string]any)
	for _, row := range appMapSlice(node["windowRows"]) {
		var extent int64
		if unit == "rows" {
			extent = appInt64(row["visualRow"])
		} else {
			extent = appInt64(row["offset"])
		}
		if _, duplicate := result[extent]; duplicate {
			t.Fatalf("duplicate %s extent %d in %#v", unit, extent, node["windowRows"])
		}
		result[extent] = row
	}
	return result
}

func assertSemanticOverlapStable(t *testing.T, first, second map[string]any, unit string) int {
	t.Helper()
	firstRows := semanticRowsByExtent(t, first, unit)
	secondRows := semanticRowsByExtent(t, second, unit)
	overlap := 0
	for extent, oldRow := range firstRows {
		newRow, ok := secondRows[extent]
		if !ok {
			continue
		}
		overlap++
		if !reflect.DeepEqual(comparableSemanticRow(oldRow), comparableSemanticRow(newRow)) {
			t.Fatalf("overlap changed at %s extent %d:\nold %#v\nnew %#v",
				unit, extent, oldRow, newRow)
		}
	}
	return overlap
}

func TestSemanticWindowProtocol_EditorKeepsThreeScreensAndStableOverlap(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for row := 0; row < 200; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "overlap.txt")
	editor.SetPosition(0, 0, 39, 8)
	editor.ScrollTopRow = 80

	first := editor.SemanticNode(nil)
	viewport := semanticInt(first["viewportRows"])
	buffer := semanticWindowBufferRows(viewport)
	if viewport != 8 || buffer != viewport {
		t.Fatalf("viewport=%d buffer=%d, want one full viewport of overscan", viewport, buffer)
	}
	if got, want := len(appMapSlice(first["windowRows"])), 3*viewport; got != want {
		t.Fatalf("initial bounded rows=%d, want %d", got, want)
	}

	// With three screens buffered, QML requests the next window after half a
	// screen of travel. Even if the response arrives only at the old ListView's
	// lower bound, that live top-row anchor is still present in the new window.
	requestTop := editor.ScrollTopRow + viewport/2
	oldBoundaryTop := appInt64(first["windowStart"]) + int64(2*viewport)
	if !editor.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(editor), "action": "editor.scroll",
		"visualRow": requestTop, "generation": uint64(1),
	}) {
		t.Fatal("editor window request was not handled")
	}
	second := editor.SemanticNode(nil)
	if overlap := assertSemanticOverlapStable(t, first, second, "rows"); overlap < 2*viewport {
		t.Fatalf("editor overlap=%d rows, want at least %d", overlap, 2*viewport)
	}
	if _, ok := semanticRowsByExtent(t, second, "rows")[oldBoundaryTop]; !ok {
		t.Fatalf("new window lost live old-boundary anchor row %d", oldBoundaryTop)
	}
}

func TestSemanticWindowProtocol_ViewerKeepsThreeScreensAndStableOverlap(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "overlap.txt")
	var content strings.Builder
	var offsets []int64
	for row := 0; row < 200; row++ {
		offsets = append(offsets, int64(content.Len()))
		fmt.Fprintf(&content, "line-%03d\n", row)
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 39, 8)
	viewer.TopOffset = offsets[80]
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := awaitSemanticViewerWindow(t, viewer, 24)
	viewport := semanticInt(first["viewportRows"])
	if got, want := len(appMapSlice(first["windowRows"])), 3*viewport; got != want {
		t.Fatalf("initial bounded rows=%d, want %d", got, want)
	}
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": offsets[84], "generation": uint64(1),
	}) {
		t.Fatal("viewer window request was not handled")
	}
	second := awaitSemanticViewerWindow(t, viewer, 24)
	if overlap := assertSemanticOverlapStable(t, first, second, "bytes"); overlap < 2*viewport {
		t.Fatalf("viewer overlap=%d rows, want at least %d", overlap, 2*viewport)
	}
	oldBoundaryOffset := offsets[88]
	if _, ok := semanticRowsByExtent(t, second, "bytes")[oldBoundaryOffset]; !ok {
		t.Fatalf("new viewer window lost live old-boundary anchor offset %d", oldBoundaryOffset)
	}
}

func TestSemanticWindowProtocol_StaleGenerationsCannotMoveEditor(t *testing.T) {
	var content strings.Builder
	for row := 0; row < 100; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "generation.txt")
	editor.SetPosition(0, 0, 39, 8)
	target := vtui.SemanticID(editor)

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.scroll", "visualRow": 40,
		"generation": uint64(41),
	})
	if editor.ScrollTopRow != 40 || editor.semanticWindowGeneration != 41 {
		t.Fatalf("fresh request produced top=%d generation=%d", editor.ScrollTopRow,
			editor.semanticWindowGeneration)
	}
	for _, generation := range []uint64{40, 41} {
		editor.HandleSemanticAction(map[string]any{
			"target": target, "action": "editor.scroll", "visualRow": 3,
			"generation": generation,
		})
		if editor.ScrollTopRow != 40 || editor.semanticWindowGeneration != 41 {
			t.Fatalf("stale generation %d moved editor to top=%d generation=%d",
				generation, editor.ScrollTopRow, editor.semanticWindowGeneration)
		}
	}
	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.scroll", "visualRow": 1 << 20,
		"generation": uint64(42),
	})
	if editor.semanticWindowGeneration != 42 {
		t.Fatalf("clamped no-op did not ACK exact generation 42: %d",
			editor.semanticWindowGeneration)
	}
}

func TestSemanticWindowProtocol_ContentKeyIgnoresCursorOnlySceneButTracksRepaint(t *testing.T) {
	vtui.SetDefaultPalette()
	editor := NewEditorView(piecetable.New([]byte("alpha\nbeta\ngamma\ndelta\n")), nil,
		"content-key.txt")
	editor.SetPosition(0, 0, 39, 8)
	editor.CursorLine = 1
	first := semanticString(editor.SemanticNode(nil)["windowContentKey"])
	if first == "" {
		t.Fatal("editor did not export a window content key")
	}

	// Cursor geometry is exported separately and must not force QML to inspect
	// or replace an otherwise byte-for-byte identical row model.
	editor.CursorLine = 2
	second := semanticString(editor.SemanticNode(nil)["windowContentKey"])
	if second != first {
		t.Fatalf("cursor-only scene changed row content key: %q -> %q", first, second)
	}

	// Selection is painted into the semantic runs, so it must invalidate the
	// key even though the window coordinates and underlying text are unchanged.
	editor.selActive = true
	editor.selAnchorOffset = editor.li.GetLineOffset(1)
	editor.CursorLine = 1
	editor.CursorPos = 3
	selected := semanticString(editor.SemanticNode(nil)["windowContentKey"])
	if selected == first {
		t.Fatalf("selection repaint retained row content key %q", selected)
	}
}

func TestSemanticWindowProtocol_SupersededAsyncViewerSeekCannotJumpBack(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "async.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 2*1024*1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 39, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	target := vtui.SemanticID(viewer)

	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "viewer.scrollWindow", "offset": int64(1024 * 1024),
		"generation": uint64(7),
	})
	if !viewer.semanticPendingScroll || viewer.semanticPendingGeneration != 7 {
		t.Fatalf("far request pending=%v generation=%d",
			viewer.semanticPendingScroll, viewer.semanticPendingGeneration)
	}
	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "viewer.scrollWindow", "offset": int64(0),
		"generation": uint64(8),
	})
	if viewer.semanticPendingScroll || viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("new request state pending=%v top=%d generation=%d",
			viewer.semanticPendingScroll, viewer.TopOffset, viewer.semanticWindowGeneration)
	}

	select {
	case task := <-vtui.FrameManager.TaskChan:
		task() // Completes the cache fill started only for generation 7.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for superseded viewer cache fill")
	}
	_ = viewer.SemanticNode(nil)
	if viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("late generation 7 completion jumped to top=%d generation=%d",
			viewer.TopOffset, viewer.semanticWindowGeneration)
	}
	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "viewer.scrollWindow", "offset": int64(512 * 1024),
		"generation": uint64(7),
	})
	if viewer.TopOffset != 0 || viewer.semanticPendingScroll || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("stale action mutated viewer: top=%d pending=%v generation=%d",
			viewer.TopOffset, viewer.semanticPendingScroll, viewer.semanticWindowGeneration)
	}
}
