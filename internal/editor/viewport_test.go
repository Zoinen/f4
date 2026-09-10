package editor

import (
	config "github.com/unxed/f4/internal/config"
	piecetable "github.com/unxed/f4/internal/piecetable"
	semantic "github.com/unxed/f4/internal/semantic"
	vtui "github.com/unxed/vtui"
	strings "strings"
	testing "testing"
)

func TestDocumentGeometryReflowPreservesSourceAndRejectsStalePointer(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(10, 5, 1)
	editor := NewEditorView(piecetable.New([]byte(strings.Repeat("x", 240))), nil, "reflow.txt")
	defer editor.Close()
	editor.WordWrap = true
	semantic.SeedNativeDocumentViewport(editor)
	editor.ScrollTopRow = 5
	oldRevision := editor.semanticLayoutRevision
	semantic.ApplyNativeDocumentViewport(editor, semantic.NativeDocumentGeometry{Columns: 25, Rows: 8, Revision: 2})
	fragment := editor.Engine.GetFragments(0)[editor.ScrollTopRow]
	if fragment.ByteOffsetStart != 50 || editor.semanticLayoutRevision <= oldRevision {
		t.Fatalf("source anchor changed during reflow: row=%d fragment=%+v revision=%d", editor.ScrollTopRow, fragment, editor.semanticLayoutRevision)
	}
	editor.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(editor), "action": "editor.mouse", "phase": "press",
		"button": "left", "rowOffset": 50, "column": 4, "layoutRevision": oldRevision})
	if editor.CursorPos != 0 {
		t.Fatal("stale source pointer moved caret")
	}
	semantic.ApplyNativeDocumentViewport(editor, semantic.NativeDocumentGeometry{Columns: 1, Rows: 1, Revision: 1})
	if editor.viewportWidth() != 25 {
		t.Fatal("stale geometry replaced latest measurement")
	}
}

func TestDocumentGeometryConsoleAndLegacyRemainConsoleSized(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(120, 40, 1)
	config.App.GuiPresentation = config.GuiPresentationText
	editor := NewEditorView(piecetable.New([]byte("text")), nil, "console.txt")
	defer editor.Close()
	editor.SetPosition(0, 0, 39, 8)
	semantic.SeedNativeDocumentViewport(editor)
	if editor.viewportWidth() != 39 || editor.ViewportHeight() != 8 || semantic.NativeDocumentLayoutPending(0) {
		t.Fatal("native measurement changed console geometry")
	}
	config.App.GuiPresentation = config.GuiPresentationGUI
	restore := semantic.BeginNativeDocumentViewport(false)
	defer restore()
	semantic.SeedNativeDocumentViewport(editor)
	if editor.viewportWidth() != 39 || semantic.NativeDocumentLayoutPending(0) {
		t.Fatal("legacy client must not wait for geometry")
	}
}

func TestDocumentPointerLeftEdgeKeepsSignedNativeColumn(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(30, 8, 1)
	editor := NewEditorView(piecetable.New([]byte(strings.Repeat("x", 200)+"\nshort\n\n")), nil, "edge.txt")
	defer editor.Close()
	editor.SetPosition(0, 0, 39, 8)
	semantic.SeedNativeDocumentViewport(editor)
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
	if !editor.SelActive || editor.SelAnchorOffset != 70 {
		t.Fatal("edge scroll lost selection anchor")
	}
}

func TestDocumentPendingHistoryNeverPublishesLoadingRows(t *testing.T) {
	withNativeDocumentNegotiation(t)
	reportDocumentGeometry(40, 10, 1)
	editor := NewEditorView(piecetable.New([]byte("initial text\n")), nil, "history.txt")
	defer editor.Close()
	semantic.SeedNativeDocumentViewport(editor)
	editor.TargetLine = 10000
	editor.semanticExtentKnown = false
	node := editor.SemanticNode(nil)
	if !semantic.Bool(node["layoutPending"]) || len(semantic.AppMapSlice(node["windowRows"])) != 0 {
		t.Fatal("saved target still pending but a loading row was published as content")
	}
}
