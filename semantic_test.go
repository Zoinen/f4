package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFileSystemPanelSemanticPanelNode(t *testing.T) {
	tmp := t.TempDir()
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		sortMode:      SortSize,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true, Mode: "drwxr-xr-x"}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 1234, MTime: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), Mode: "-rw-r--r--"}, Selected: true},
		},
	}
	fp.SetCanFocus(true)
	fp.SetPosition(0, 0, 39, 9)
	fp.SetCursorIndex(1)

	model := fp.semanticPanelModel(&vtui.SemanticContext{Width: 80, Height: 25}, 0, true)
	node := model.ToMap()

	if node["kind"] != "filePanel" {
		t.Fatalf("kind = %v, want filePanel", node["kind"])
	}
	if node["active"] != true || node["side"] != 0 {
		t.Fatalf("unexpected panel identity: active=%v side=%v", node["active"], node["side"])
	}
	if node["cursor"] != 1 {
		t.Fatalf("cursor = %v, want 1", node["cursor"])
	}
	if node["path"] != tmp {
		t.Fatalf("path = %v, want %s", node["path"], tmp)
	}
	if node["selectedCount"] != 1 {
		t.Fatalf("selectedCount = %v, want 1", node["selectedCount"])
	}
	entries := node["entries"].([]map[string]any)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[1]["name"] != "alpha.txt" || entries[1]["selected"] != true {
		t.Fatalf("unexpected entry snapshot: %#v", entries[1])
	}
}

func TestPanelsFrameSemanticActionAcceptsQMLNumbers(t *testing.T) {
	tmp := t.TempDir()
	left := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 12}},
			{VFSItem: vfs.VFSItem{Name: "beta.txt", Size: 34}},
		},
	}
	right := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(40, 0, 79, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(41, 1, 78, 6, nil),
		viewMode:      ViewModeDetailed,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "right.txt", Size: 56}},
		},
	}
	pf := &PanelsFrame{
		panels:    [2]Panel{left, right},
		activeIdx: 0,
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.cursor",
		"side":   float64(0),
		"index":  float64(2),
	}) {
		t.Fatal("panel cursor action was not handled")
	}
	if left.GetCursorIndex() != 2 {
		t.Fatalf("left cursor = %d, want 2", left.GetCursorIndex())
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.activate",
		"side":   float64(1),
	}) {
		t.Fatal("activate panel action was not handled")
	}
	if pf.activeIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.activeIdx)
	}
}

func TestSemantic_EditorViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	pt := piecetable.New([]byte("hello"))
	ev := NewEditorView(pt, nil, "test.txt")
	ev.modified = false
	ev.CursorPos = ev.getLineLength(0)

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
	if !ev.modified {
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

func TestSemantic_ViewerViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "view.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	v := vfs.NewOSVFS(tmp)
	viewer, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("failed to create viewer: %v", err)
	}

	// Test scroll action
	actionScroll := map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scroll",
		"offset": float64(6), // Starts 'line2'
	}
	if !viewer.HandleSemanticAction(actionScroll) {
		t.Fatal("viewer scroll action was not handled")
	}
	if viewer.TopOffset != 6 {
		t.Errorf("expected TopOffset 6, got %d", viewer.TopOffset)
	}
}
