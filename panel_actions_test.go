package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

type recordingPanelActionVFS struct {
	*vfs.NullVFS
	action vfs.PanelAction
	paths  []string
}

func (r *recordingPanelActionVFS) HandlePanelAction(_ vfs.App, action vfs.PanelAction, paths []string) bool {
	r.action = action
	r.paths = append([]string(nil), paths...)
	return true
}

func TestDispatchPanelActionUsesSemanticActionAndFullPaths(t *testing.T) {
	fs := &recordingPanelActionVFS{NullVFS: vfs.NewNullVFS(0)}
	fsp := &FileSystemPanel{
		vfs:     fs,
		entries: []*fileEntry{{VFSItem: vfs.VFSItem{Name: "profile row"}}},
	}
	pf := &PanelsFrame{panels: [2]Panel{fsp, nil}, activeIdx: 0}
	paths := selectedPanelActionPaths(fsp)
	if !dispatchPanelAction(pf, vfs.PanelActionEdit, paths) {
		t.Fatal("handler did not consume action")
	}
	wantPath := fs.Join(fs.GetPath(), "profile row")
	if fs.action != vfs.PanelActionEdit || len(fs.paths) != 1 || fs.paths[0] != wantPath {
		t.Fatalf("handler got action=%v paths=%q", fs.action, fs.paths)
	}
	paths[0] = "/mutated"
	if fs.paths[0] != wantPath {
		t.Fatal("handler input aliased caller slice")
	}
}

func TestDispatchPanelActionFallsBackForOrdinaryVFS(t *testing.T) {
	fsp := &FileSystemPanel{vfs: vfs.NewNullVFS(0)}
	pf := &PanelsFrame{panels: [2]Panel{fsp, nil}, activeIdx: 0}
	if dispatchPanelAction(pf, vfs.PanelActionCreate, []string{"/"}) {
		t.Fatal("ordinary VFS consumed semantic action")
	}
}
