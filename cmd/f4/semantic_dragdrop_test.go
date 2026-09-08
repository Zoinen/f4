package main

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"path/filepath"
	"testing"
)

func TestSemanticDropPlan(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.showPanels, pf.showLeftPanel, pf.showRightPanel = true, true, true
	src := pf.panels[0].(*FileSystemPanel)
	dst := pf.panels[1].(*FileSystemPanel)
	src.vfs = vfs.NewOSVFS(t.TempDir())
	dst.vfs = vfs.NewOSVFS(t.TempDir())
	src.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "a.txt"}}, {VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	dst.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true}}, {VFSItem: vfs.VFSItem{Name: "file.txt"}}, {VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	endpoint := func(fp *FileSystemPanel, side int) map[string]any {
		fp.updateSemanticRevisions()
		return map[string]any{"side": side, "panelId": vtui.SemanticID(fp), "path": fp.vfs.GetPath(), "catalogRevision": fp.catalogRevision}
	}
	id := func(fp *FileSystemPanel, idx int) string {
		kind, _ := fp.semanticSourceInfo()
		result, _ := fp.semanticEntryMetadata(fp.entries[idx], kind)
		return result
	}
	makeAction := func() map[string]any {
		a := endpoint(dst, 1)
		s := endpoint(src, 0)
		s["entryIds"] = []string{id(src, 0)}
		a["source"] = s
		a["operation"] = "move"
		return a
	}
	t.Run("directory and source snapshot", func(t *testing.T) {
		a := makeAction()
		a["entryId"] = id(dst, 0)
		p, err := pf.planSemanticDrop(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.target.dir != filepath.Join(dst.vfs.GetPath(), "folder") || !p.move || len(p.names) != 1 || p.names[0] != "a.txt" || p.sourceDir != src.vfs.GetPath() {
			t.Fatalf("bad plan: %+v", p)
		}
	})
	for _, idx := range []int{1, 2} {
		t.Run(dst.entries[idx].Name, func(t *testing.T) {
			a := makeAction()
			a["entryId"] = id(dst, idx)
			p, err := pf.planSemanticDrop(a)
			if err != nil || p.target.dir != dst.vfs.GetPath() {
				t.Fatalf("file/parent drop: %+v %v", p, err)
			}
		})
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"stale target", func(a map[string]any) { a["catalogRevision"] = int64(-1) }},
		{"replaced target", func(a map[string]any) { a["panelId"] = "another-panel" }},
		{"navigated target", func(a map[string]any) { a["path"] = "another-path" }},
		{"stale source", func(a map[string]any) { a["source"].(map[string]any)["catalogRevision"] = int64(-1) }},
		{"unknown source entry", func(a map[string]any) { a["source"].(map[string]any)["entryIds"] = []string{"missing"} }},
		{"parent source", func(a map[string]any) { a["source"].(map[string]any)["entryIds"] = []string{id(src, 1)} }},
		{"external move", func(a map[string]any) {
			delete(a, "source")
			a["paths"] = []string{filepath.Join(src.vfs.GetPath(), "a.txt")}
		}},
		{"external relative path", func(a map[string]any) { delete(a, "source"); a["operation"] = "copy"; a["paths"] = []string{"a.txt"} }},
		{"unsupported link", func(a map[string]any) { a["operation"] = "link" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := makeAction()
			test.mutate(a)
			if _, err := pf.planSemanticDrop(a); err == nil {
				t.Fatal("invalid drop accepted")
			}
		})
	}
	t.Run("external copy", func(t *testing.T) {
		a := makeAction()
		delete(a, "source")
		a["operation"] = "copy"
		a["paths"] = []string{filepath.Join(src.vfs.GetPath(), "a.txt")}
		if _, err := pf.planSemanticDrop(a); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("prepare complete selection and reject stale response", func(t *testing.T) {
		a := endpoint(src, 0)
		a["requestId"] = "drag-request"
		a["entryIds"] = []string{id(src, 0)}
		semanticLivePanels.Store(vtui.SemanticID(src), src)
		defer semanticLivePanels.Delete(vtui.SemanticID(src))
		response := prepareSemanticDrag(a)
		if response["ok"] != true || response["local"] != true || response["requestId"] != "drag-request" {
			t.Fatalf("unexpected drag preparation: %#v", response)
		}
		paths := semanticStringSlice(response["paths"])
		if len(paths) != 1 || paths[0] != filepath.Join(src.vfs.GetPath(), "a.txt") {
			t.Fatalf("wrong paths: %#v", paths)
		}
		a["catalogRevision"] = int64(-1)
		if prepareSemanticDrag(a)["ok"] != false {
			t.Fatal("stale preparation accepted")
		}
	})
}
