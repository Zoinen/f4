package main

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestViewerWrappedTabUsesLogicalLineOrigin(t *testing.T) {
	old := AppConfig.EditorTabSize
	AppConfig.EditorTabSize = 4
	t.Cleanup(func() { AppConfig.EditorTabSize = old })
	viewer := cachedSemanticViewer([]byte("123\t\n\tZ"))
	viewer.SetPosition(0, 0, 2, 4)
	window := viewer.semanticWindow()
	if !window.ready || len(window.rows) != 4 {
		t.Fatalf("window not coherent: %+v", window)
	}
	wantStarts := []int64{0, 2, 5, 6}
	wantEnds := []int64{2, 5, 6, 7}
	for i, row := range window.rows {
		if row.Offset != wantStarts[i] || row.EndOffset != wantEnds[i] {
			t.Fatalf("row %d: %+v", i, row)
		}
	}
	row, err := viewer.projectRow(2, 2)
	if err != nil || len(row.cells) != 2 || row.cells[0].Char != '3' || row.cells[1].Char != ' ' || row.nextColumn != 0 {
		t.Fatalf("wrapped tab/newline projected incorrectly: %+v err=%v", row, err)
	}
	if start, ready := viewer.semanticWrappedRowStart(3, 2); !ready || start != 2 || viewer.semanticWrapSeek.resolvedColumn != 2 {
		t.Fatalf("tab source hit=(%d,%v), column=%d", start, ready, viewer.semanticWrapSeek.resolvedColumn)
	}
	position, ready := viewer.semanticPreviousTextRowPosition(5, 2)
	if !ready || position.offset != 2 || position.column != 2 {
		t.Fatalf("previous row position=%+v ready=%v", position, ready)
	}
	row, err = viewer.projectRow(5, 2)
	if err != nil || row.end != 6 || row.nextColumn != 4 {
		t.Fatalf("tab after newline retained old origin: %+v %v", row, err)
	}
}

func TestViewerTabSettingInvalidatesLayoutAndPendingConstruction(t *testing.T) {
	old := AppConfig.EditorTabSize
	AppConfig.EditorTabSize = 4
	t.Cleanup(func() { AppConfig.EditorTabSize = old })
	viewer := cachedSemanticViewer([]byte("1\t2\t3\n"))
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{columns: 4, rows: 3, revision: 1})
	first := viewer.SemanticNode(nil)
	before := viewer.semanticLayoutRevision
	key := viewer.constructionKey(viewer.TopOffset, 4, 3, 8)
	viewer.semanticProjection = &viewerWindowConstruction{key: key}
	viewer.consoleProjection = &viewerWindowConstruction{key: key}
	viewer.projectionContinuationPending = true
	viewer.semanticPendingScroll, viewer.semanticPendingGeneration = true, 7
	AppConfig.EditorTabSize = 8
	// A coordinate action from the displayed old layout must be rejected even
	// if it arrives before another semantic frame samples the changed setting.
	viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": 4, "generation": uint64(8), "layoutRevision": before,
	})
	if viewer.semanticLayoutRevision != before+1 || viewer.semanticPendingScroll ||
		viewer.semanticProjection != nil || viewer.consoleProjection != nil || viewer.projectionContinuationPending {
		t.Fatal("tab setting preserved a stale mapping or unfinished construction")
	}
	if key == viewer.constructionKey(viewer.TopOffset, 4, 3, 8) {
		t.Fatal("construction identity omitted effective tab size")
	}
	second := viewer.SemanticNode(nil)
	if second["layoutPending"] != false || semanticString(first["windowContentKey"]) == semanticString(second["windowContentKey"]) {
		t.Fatal("tab change did not publish a coherent new row projection")
	}
	AppConfig.EditorTabSize = 0 // The fallback is the same effective size, 8.
	viewer.SemanticNode(nil)
	if viewer.semanticLayoutRevision != before+1 {
		t.Fatal("equivalent default tab size caused unnecessary reflow")
	}
}

func TestViewerProjectionWideAndInvalidBytesShareNavigation(t *testing.T) {
	old := AppConfig.EditorTabSize
	AppConfig.EditorTabSize = 4
	t.Cleanup(func() { AppConfig.EditorTabSize = old })
	viewer := cachedSemanticViewer([]byte("a界\t\xffb\n"))
	first, err := viewer.projectRowAt(0, 3, 0)
	if err != nil || first.end != 4 || first.nextColumn != 3 || first.cells[2].Char != vtui.WideCharFiller {
		t.Fatalf("wide row=%+v err=%v", first, err)
	}
	second, err := viewer.projectRowAt(first.end, 3, first.nextColumn)
	if err != nil || second.end != 8 || second.nextColumn != 0 || second.cells[0].Char != ' ' || second.cells[1].Char != '?' || second.cells[2].Char != 'b' {
		t.Fatalf("invalid-byte/tab row=%+v err=%v", second, err)
	}
	for _, offset := range []int64{1, 2, 3} {
		start, ready := viewer.semanticWrappedRowStart(offset, 3)
		if !ready || start != 0 {
			t.Fatalf("wide source offset %d resolved=(%d,%v)", offset, start, ready)
		}
	}
}

func TestViewerReflowPublishesContainingSourceFragment(t *testing.T) {
	viewer := cachedSemanticViewer([]byte("123456789"))
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{columns: 2, rows: 2, revision: 1})
	viewer.TopOffset = 2
	viewer.SemanticNode(nil)
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{columns: 3, rows: 2, revision: 2})
	node := viewer.SemanticNode(nil)
	if viewer.TopOffset != 0 || node["layoutPending"] != false {
		t.Fatalf("reflow lost source anchor: top=%d node=%+v", viewer.TopOffset, node)
	}
	rows := appMapSlice(node["windowRows"])
	if len(rows) == 0 || appInt64(rows[0]["offset"]) != 0 || appInt64(rows[0]["endOffset"]) != 3 {
		t.Fatalf("reflow started inside a fragment: %+v", rows)
	}
}

type failingViewerProjectionReader struct{ vfs.ReadAtCloser }

func (failingViewerProjectionReader) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessDirectLocal
}
func (failingViewerProjectionReader) ReadAt(context.Context, []byte, int64) (int, error) {
	return 0, errors.New("viewer source disconnected")
}

func TestViewerPermanentSeekErrorNeverAcknowledgesReadyWindow(t *testing.T) {
	viewer := cachedSemanticViewer([]byte("abcdef"))
	viewer.backend.file = failingViewerProjectionReader{viewer.backend.file}
	viewer.backend.cacheData = nil
	viewer.TopOffset = 2
	viewer.semanticNeedsReflow = true
	node := viewer.SemanticNode(nil)
	if viewer.TopOffset != 2 || node["layoutPending"] != true || semanticString(node["loadError"]) == "" {
		t.Fatalf("failed source became a ready row: %+v", node)
	}
}

func TestViewerProjectionDecodeMode(t *testing.T) {
	code := []byte{
		0x48, 0x83, 0xec, 0x28, // sub rsp, 0x28
		0xe8, 0x00, 0x00, 0x00, 0x00, // call +0
		0x48, 0x83, 0xc4, 0x28, // add rsp, 0x28
		0xc3, // ret
	}
	viewer := cachedSemanticViewer(code)
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{columns: 80, rows: 10, revision: 1})
	viewer.DecodeMode = true
	viewer.DisasmMode = 64

	node := viewer.SemanticNode(nil)
	if node["mode"] != "decode" || node["decodeMode"] != true {
		t.Fatalf("unexpected mode in node: %+v", node)
	}
	rows := appMapSlice(node["windowRows"])
	if len(rows) < 4 {
		t.Fatalf("expected at least 4 disassembly rows, got %d", len(rows))
	}
	expectedOffsets := []int64{0, 4, 9, 13}
	expectedEnds := []int64{4, 9, 13, 14}
	for i := 0; i < 4; i++ {
		off := appInt64(rows[i]["offset"])
		end := appInt64(rows[i]["endOffset"])
		if off != expectedOffsets[i] || end != expectedEnds[i] {
			t.Fatalf("row %d offsets mismatch: got (%d, %d), want (%d, %d)", i, off, end, expectedOffsets[i], expectedEnds[i])
		}
	}
}

func TestViewerDecodeModeScrollWindowAction(t *testing.T) {
	code := []byte{
		0x48, 0x83, 0xec, 0x28, // sub rsp, 0x28 (0..4)
		0xe8, 0x00, 0x00, 0x00, 0x00, // call +0 (4..9)
		0x48, 0x83, 0xc4, 0x28, // add rsp, 0x28 (9..13)
		0xc3, // ret (13..14)
	}
	viewer := cachedSemanticViewer(code)
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{columns: 80, rows: 10, revision: 1})
	viewer.DecodeMode = true
	viewer.DisasmMode = 64

	firstNode := viewer.SemanticNode(nil)
	generation := appInt64(firstNode["windowGeneration"])

	// Scroll to offset 4 (second instruction)
	viewer.HandleSemanticAction(map[string]any{
		"target":         vtui.SemanticID(viewer),
		"action":         "viewer.scrollWindow",
		"offset":         int64(4),
		"generation":     uint64(generation + 1),
		"layoutRevision": viewer.semanticLayoutRevision,
	})

	secondNode := viewer.SemanticNode(nil)
	if viewer.TopOffset != 4 {
		t.Fatalf("expected TopOffset 4 after scroll and SemanticNode, got %d", viewer.TopOffset)
	}

	rows := appMapSlice(secondNode["windowRows"])
	if len(rows) == 0 || appInt64(rows[0]["offset"]) != 0 {
		// Window should include preceding instruction row at offset 0
		t.Fatalf("expected window to contain preceding instruction row at offset 0: %+v", rows[0])
	}
}
