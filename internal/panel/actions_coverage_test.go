package panel

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

type actionCoverageVFS struct {
	vfs.VFS
	result bool
	got    []string
}

func (v *actionCoverageVFS) HandlePanelAction(_ vfs.App, _ vfs.PanelAction, paths []string) bool {
	v.got = append([]string(nil), paths...)
	if len(paths) > 0 {
		paths[0] = "mutated by handler"
	}
	return v.result
}

func TestDispatchPanelActionCoverage(t *testing.T) {
	if DispatchPanelAction(nil, vfs.PanelActionEdit, []string{"x"}) {
		t.Fatal("nil frame must not consume an action")
	}
	pf := &PanelsFrame{}
	if DispatchPanelAction(pf, vfs.PanelActionEdit, []string{"x"}) {
		t.Fatal("frame without an active panel must not consume an action")
	}

	plain := &FileSystemPanel{Vfs: vfs.NewNullVFS(0)}
	pf.Panels[0] = plain
	if DispatchPanelAction(pf, vfs.PanelActionEdit, []string{"x"}) {
		t.Fatal("plain VFS must preserve host handling")
	}

	handler := &actionCoverageVFS{VFS: vfs.NewNullVFS(0), result: true}
	plain.Vfs = handler
	input := []string{"/one", "/two"}
	if !DispatchPanelAction(pf, vfs.PanelActionDelete, input) {
		t.Fatal("handler result was not returned")
	}
	if input[0] != "/one" || handler.got[0] != "/one" {
		t.Fatalf("handler did not receive an immutable snapshot: input=%v got=%v", input, handler.got)
	}
	handler.result = false
	if DispatchPanelAction(pf, vfs.PanelActionEdit, nil) {
		t.Fatal("false handler result must preserve host handling")
	}
}

func TestSelectedPanelActionPathsCoverage(t *testing.T) {
	dir := t.TempDir()
	fsp := &FileSystemPanel{
		Vfs: vfs.NewOSVFS(dir),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "alpha.txt"}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: ""}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		},
	}
	paths := SelectedPanelActionPaths(fsp)
	if len(paths) != 1 || paths[0] != filepath.Join(dir, "alpha.txt") {
		t.Fatalf("selected paths = %v", paths)
	}
	if got := SelectedPanelActionPaths(nil); got != nil {
		t.Fatalf("nil panel paths = %v", got)
	}

	fsp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "fallback.txt"}}}
	fsp.CursorIdx = 0
	paths = SelectedPanelActionPaths(fsp)
	if len(paths) != 1 || paths[0] != filepath.Join(dir, "fallback.txt") {
		t.Fatalf("fallback paths = %v", paths)
	}
	fsp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.CursorIdx = 0
	if got := SelectedPanelActionPaths(fsp); len(got) != 0 {
		t.Fatalf("parent entry should not produce action paths: %v", got)
	}
}
