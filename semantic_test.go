package main

import (
	"testing"
	"time"

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

	node := fp.SemanticPanelNode(&vtui.SemanticContext{Width: 80, Height: 25}, 0, true)
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

func TestTerminalViewSemanticNodeExportsRows(t *testing.T) {
	tv := NewTerminalView(12, 4)
	tv.SetPosition(0, 0, 11, 3)
	tv.Title = "Shell"
	for _, r := range "ok" {
		tv.PutChar(r, DefaultTermAttr)
	}

	node := tv.SemanticNode(&vtui.SemanticContext{Width: 12, Height: 4})
	if node["kind"] != "terminal" {
		t.Fatalf("kind = %v, want terminal", node["kind"])
	}
	if node["title"] != "Shell" {
		t.Fatalf("title = %v, want Shell", node["title"])
	}
	rows := node["rows"].([]map[string]any)
	if len(rows) == 0 {
		t.Fatal("expected terminal rows")
	}
	found := false
	for _, row := range rows {
		for _, run := range row["runs"].([]map[string]any) {
			if run["text"] == "ok          " {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("terminal text was not exported: %#v", rows)
	}
}
