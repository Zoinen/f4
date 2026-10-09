package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestNavigateToPathCoversCurrentVFSAbsoluteDirectoryAndURI(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	fsp := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(root))
	other := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(root))
	pf := &PanelsFrame{Panels: [2]Panel{fsp, other}}

	if !pf.NavigateToPath(fsp, ".") || fsp.PendingSelection != ".." {
		t.Fatalf("current VFS navigation failed: pending=%q path=%q", fsp.PendingSelection, fsp.Vfs.GetPath())
	}
	if !pf.NavigateToPath(fsp, child) || fsp.PendingSelection != ".." {
		t.Fatalf("absolute directory navigation failed: pending=%q path=%q", fsp.PendingSelection, fsp.Vfs.GetPath())
	}
	if pf.NavigateToPath(fsp, "unknown-scheme://missing") {
		t.Fatal("unsupported URI provider was accepted")
	}
}
