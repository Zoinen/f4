package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func withNativeDocumentNegotiation(t *testing.T) {
	t.Helper()
	previous, presentation := nativeDocumentViewport, AppConfig.GuiPresentation
	nativeDocumentViewport.enabled = true
	nativeDocumentViewport.geometry = nativeDocumentGeometry{}
	AppConfig.GuiPresentation = GuiPresentationGUI
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(func() { nativeDocumentViewport = previous; AppConfig.GuiPresentation = presentation })
}

func reportDocumentGeometry(columns, rows int, revision uint64) {
	HandleSemanticAction(map[string]any{"target": "app", "action": "document.viewport",
		"scope": "standalone", "columns": columns, "rows": rows, "geometryRevision": revision})
}

func TestDocumentGeometryFirstEditorWindowAndRepeatedOpen(t *testing.T) {
	withNativeDocumentNegotiation(t)
	for _, columns := range []int{12, 100} {
		reportDocumentGeometry(columns, 17, uint64(columns))
		for attempt := 0; attempt < 2; attempt++ {
			editor := NewEditorView(piecetable.New([]byte(strings.Repeat("x", 240))), nil, "wide.txt")
			editor.WordWrap = true
			seedNativeDocumentViewport(editor)
			editor.SetPosition(0, 0, 39, 8)
			node := editor.SemanticNode(nil)
			rows := appMapSlice(node["windowRows"])
			if semanticBool(node["layoutPending"]) || semanticInt(node["viewportColumns"]) != columns ||
				semanticInt(node["viewportRows"]) != 17 || len(rows) == 0 || appInt64(rows[0]["endOffset"]) != int64(columns) {
				t.Fatalf("attempt %d: first window does not use %dx17: pending=%v columns=%v rows=%v fragments=%v",
					attempt, columns, node["layoutPending"], node["viewportColumns"], node["viewportRows"], len(rows))
			}
			editor.Close()
		}
	}
}

func TestDocumentGeometryFirstViewerWindowAndStartupPending(t *testing.T) {
	withNativeDocumentNegotiation(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 300)), 0600); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(dir), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.WrapMode = true
	viewer.SetPosition(0, 0, 39, 8)
	pending := viewer.SemanticNode(nil)
	if !semanticBool(pending["layoutPending"]) || len(appMapSlice(pending["windowRows"])) != 0 {
		t.Fatal("startup published console-width text before native measurement")
	}
	reportDocumentGeometry(95, 16, 1)
	seedNativeDocumentViewport(viewer)
	node := viewer.SemanticNode(nil)
	rows := appMapSlice(node["windowRows"])
	if semanticBool(node["layoutPending"]) || len(rows) == 0 || appInt64(rows[0]["endOffset"]) != 95 || semanticInt(node["viewportRows"]) != 16 {
		t.Fatalf("first viewer content has wrong geometry: pending=%v rows=%v", node["layoutPending"], rows)
	}
}

func TestDocumentGeometryReflowPreservesSourceAndRejectsStalePointer(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(10, 5, 1)
	editor := NewEditorView(piecetable.New([]byte(strings.Repeat("x", 240))), nil, "reflow.txt")
	defer editor.Close()
	editor.WordWrap = true
	seedNativeDocumentViewport(editor)
	editor.ScrollTopRow = 5
	oldRevision := editor.semanticLayoutRevision
	applyNativeDocumentViewport(editor, nativeDocumentGeometry{columns: 25, rows: 8, revision: 2})
	fragment := editor.engine.GetFragments(0)[editor.ScrollTopRow]
	if fragment.ByteOffsetStart != 50 || editor.semanticLayoutRevision <= oldRevision {
		t.Fatalf("source anchor changed during reflow: row=%d fragment=%+v revision=%d", editor.ScrollTopRow, fragment, editor.semanticLayoutRevision)
	}
	editor.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(editor), "action": "editor.mouse", "phase": "press",
		"button": "left", "rowOffset": 50, "column": 4, "layoutRevision": oldRevision})
	if editor.CursorPos != 0 {
		t.Fatal("stale source pointer moved caret")
	}
	applyNativeDocumentViewport(editor, nativeDocumentGeometry{columns: 1, rows: 1, revision: 1})
	if editor.viewportWidth() != 25 {
		t.Fatal("stale geometry replaced latest measurement")
	}
}

func TestDocumentGeometryConsoleAndLegacyRemainConsoleSized(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(120, 40, 1)
	AppConfig.GuiPresentation = GuiPresentationText
	editor := NewEditorView(piecetable.New([]byte("text")), nil, "console.txt")
	defer editor.Close()
	editor.SetPosition(0, 0, 39, 8)
	seedNativeDocumentViewport(editor)
	if editor.viewportWidth() != 39 || editor.viewportHeight() != 8 || nativeDocumentLayoutPending(0) {
		t.Fatal("native measurement changed console geometry")
	}
	AppConfig.GuiPresentation = GuiPresentationGUI
	nativeDocumentViewport.enabled = false
	seedNativeDocumentViewport(editor)
	if editor.viewportWidth() != 39 || nativeDocumentLayoutPending(0) {
		t.Fatal("legacy client must not wait for geometry")
	}
}

func TestDocumentPointerLeftEdgeKeepsSignedNativeColumn(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(30, 8, 1)
	editor := NewEditorView(piecetable.New([]byte(strings.Repeat("x", 200)+"\nshort\n\n")), nil, "edge.txt")
	defer editor.Close()
	editor.SetPosition(0, 0, 39, 8)
	seedNativeDocumentViewport(editor)
	editor.ScrollLeft = 60
	for _, sample := range []struct {
		phase string
		col   int
	}{{"press", 10}, {"move", -3}} {
		editor.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(editor), "action": "editor.mouse",
			"phase": sample.phase, "button": "left", "moved": sample.phase == "move", "column": sample.col,
			"rowOffset": 0, "scrollLeft": 60, "layoutRevision": editor.semanticLayoutRevision})
	}
	if editor.ScrollLeft != 57 || editor.CursorPos != 57 {
		t.Fatalf("left edge was clamped: left=%d cursor=%d", editor.ScrollLeft, editor.CursorPos)
	}
	if !editor.selActive || editor.selAnchorOffset != 70 {
		t.Fatal("edge scroll lost selection anchor")
	}
}

func TestDocumentPendingHistoryNeverPublishesLoadingRows(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(40, 10, 1)
	editor := NewEditorView(piecetable.New([]byte("initial text\n")), nil, "history.txt")
	defer editor.Close()
	seedNativeDocumentViewport(editor)
	editor.targetLine = 10000
	editor.semanticExtentKnown = false
	node := editor.SemanticNode(nil)
	if !semanticBool(node["layoutPending"]) || len(appMapSlice(node["windowRows"])) != 0 {
		t.Fatal("saved target still pending but a loading row was published as content")
	}
}
