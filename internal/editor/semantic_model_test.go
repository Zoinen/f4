package editor

import (
	fmt "fmt"
	piecetable "github.com/unxed/f4/internal/piecetable"
	semantic "github.com/unxed/f4/internal/semantic"
	theme "github.com/unxed/f4/internal/theme"
	chroma "github.com/unxed/f4/plugins/chroma"
	vfs "github.com/unxed/f4/vfs"
	vtui "github.com/unxed/vtui"
	strings "strings"
	testing "testing"
)

func TestSemantic_EditorViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	Pt := piecetable.New([]byte("hello"))
	ev := NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.Modified = false
	ev.CursorPos = ev.GetLineLength(0)

	// 1. Test insertText
	actionInsert := map[string]any{
		"target": vtui.SemanticID(ev),
		"action": "editor.insertText",
		"text":   " world",
	}
	if !ev.HandleSemanticAction(actionInsert) {
		t.Fatal("editor insert action was not handled")
	}
	if ev.GetText() != "hello world" {
		t.Errorf("expected 'hello world', got %q", ev.GetText())
	}
	if !ev.Modified {
		t.Error("editor should be marked as modified after insertion")
	}

	// 2. Test Undo
	actionUndo := map[string]any{
		"target": vtui.SemanticID(ev),
		"action": "editor.undo",
	}
	if !ev.HandleSemanticAction(actionUndo) {
		t.Fatal("editor undo action was not handled")
	}
	if ev.GetText() != "hello" {
		t.Errorf("expected 'hello' after undo, got %q", ev.GetText())
	}
}

func TestSemantic_EditorWindowAndScrollPreserveCursor(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&content, "row-%03d\n", i)
	}
	ev := NewEditorView(piecetable.New([]byte(content.String())), nil, "window.txt")
	ev.SetPosition(0, 0, 39, 8)
	ev.ScrollTopRow = 40
	ev.CursorLine = 44
	ev.CursorPos = 2
	cursorLine, cursorPos := ev.CursorLine, ev.CursorPos

	node := ev.SemanticNode(nil)
	windowRows := semantic.AppMapSlice(node["windowRows"])
	viewportRow := semantic.Int(node["viewportRow"])
	if semantic.String(node["scrollUnit"]) != "rows" ||
		semantic.AppInt64(node["viewportStart"]) != 40 ||
		semantic.AppInt64(node["contentExtent"]) != int64(ev.Engine.GetTotalVisualRows()) ||
		node["contentExtentKnown"] != true {
		t.Fatalf("invalid editor window contract: %#v", node)
	}
	if viewportRow < 1 || viewportRow >= len(windowRows) ||
		semantic.Int(windowRows[viewportRow]["visualRow"]) != 40 {
		t.Fatalf("viewport row=%d rows=%#v", viewportRow, windowRows)
	}
	for i := 0; i+1 < len(windowRows); i++ {
		if semantic.Int(windowRows[i+1]["visualRow"]) != semantic.Int(windowRows[i]["visualRow"])+1 {
			t.Fatalf("non-contiguous editor rows at %d", i)
		}
	}
	if got := semantic.AppInt64(node["cursorAbsoluteRow"]); got != int64(cursorLine) {
		t.Fatalf("cursor absolute row=%d, want %d", got, cursorLine)
	}
	if got := semantic.String(node["topBarLeft"]); got != " window.txt" {
		t.Fatalf("editor top bar left=%q, want %q", got, " window.txt")
	}
	editorStatus := semantic.String(node["topBarRight"])
	if !strings.Contains(editorStatus, vfs.DisplayCodepageName(ev.Codepage)) ||
		!strings.Contains(editorStatus, "Ln  45/") ||
		!strings.Contains(editorStatus, "Col   3") {
		t.Fatalf("editor top bar right=%q does not contain codepage and cursor", editorStatus)
	}
	if !ev.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(ev), "action": "editor.scroll", "visualRow": 73,
	}) {
		t.Fatal("editor scroll was not handled")
	}
	if ev.ScrollTopRow != 40 || !ev.semanticPendingScroll {
		t.Fatalf("unpublished scroll changed viewport: top=%d pending=%v", ev.ScrollTopRow, ev.semanticPendingScroll)
	}
	ev.SemanticNode(nil) // Ready rows and viewport placement commit together.
	if ev.ScrollTopRow != 73 || ev.CursorLine != cursorLine || ev.CursorPos != cursorPos {
		t.Fatalf("scroll changed editor state: top=%d cursor=%d:%d", ev.ScrollTopRow,
			ev.CursorLine, ev.CursorPos)
	}
	before := ev.semanticWindowGeneration
	ev.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(ev), "action": "editor.scroll", "visualRow": 1 << 30,
	})
	if ev.semanticWindowGeneration != before {
		t.Fatal("clamped request acknowledged before its window was projected")
	}
	ev.SemanticNode(nil)
	if ev.semanticWindowGeneration != before+1 {
		t.Fatalf("clamped editor generation=%d, want %d", ev.semanticWindowGeneration, before+1)
	}
}

func TestSemantic_EditorWindowExportsCursorOutsideCanonicalViewport(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&content, "row-%03d\n", i)
	}
	ev := NewEditorView(piecetable.New([]byte(content.String())), nil, "cursor-window.txt")
	ev.SetPosition(0, 0, 39, 8)
	ev.SetVisible(true)
	ev.ScrollTopRow = 40
	ev.CursorLine = 37 // Above the terminal viewport, but inside its top overscan.
	ev.CursorPos = 2

	node := ev.SemanticNode(nil)
	if got := semantic.AppInt64(node["cursorAbsoluteRow"]); got != 37 {
		t.Fatalf("cursor absolute row=%d, want 37", got)
	}
	if got := semantic.Int(node["cursorVisualRow"]); got != -3 {
		t.Fatalf("cursor viewport row=%d, want -3", got)
	}
	if got := semantic.Int(node["cursorVisualColumn"]); got != 2 {
		t.Fatalf("cursor visual column=%d, want 2", got)
	}
	if node["cursorVisible"] != true || semantic.String(node["cursorShape"]) != "underline" {
		t.Fatalf("cursor metadata not exported outside canonical viewport: %#v", node)
	}
	windowRows := semantic.AppMapSlice(node["windowRows"])
	foundCursorRow := false
	for _, row := range windowRows {
		if semantic.Int(row["visualRow"]) == 37 {
			foundCursorRow = true
			break
		}
	}
	if !foundCursorRow {
		t.Fatalf("cursor row 37 missing from overscan: %#v", windowRows)
	}

	ev.ScrollLeft = 3
	ev.Overtype = true
	node = ev.SemanticNode(nil)
	if got := semantic.Int(node["cursorVisualColumn"]); got != -1 {
		t.Fatalf("horizontally clipped cursor column=%d, want -1", got)
	}
	if node["cursorVisible"] != false || semantic.String(node["cursorShape"]) != "block" {
		t.Fatalf("horizontal clipping or cursor shape was lost: %#v", node)
	}
}

func TestEditorSemanticSurfaceUsesHighlighterAndExportsCursor(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

	pt := piecetable.New([]byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"))
	ev := NewEditorView(pt, nil, "main.go")
	defer ev.Close()
	ev.Highlighter = (&chroma.ChromaProvider{}).Create("main.go", "")
	ev.SetPosition(0, 0, 79, 23)
	ev.SetVisible(true)
	ev.SetFocus(true)
	ev.CursorLine = 2
	ev.CursorPos = 5
	ev.EnsureCursorVisible()

	rendered := semantic.SemanticRenderSurface(ev.X1, ev.Y1+1, ev.X2-1, ev.Y2, ev.DisplayObject)
	if !rendered.CursorVisible {
		t.Fatalf("editor cursor was not exported: %#v", rendered)
	}
	colors := make(map[string]bool)
	for _, row := range rendered.Rows {
		for _, run := range row {
			if strings.TrimSpace(run.Text) != "" {
				colors[run.Foreground] = true
			}
		}
	}
	if len(colors) < 2 {
		t.Fatalf("syntax-colored editor surface has only %d foreground color(s): %#v", len(colors), colors)
	}
}
