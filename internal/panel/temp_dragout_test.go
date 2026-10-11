package panel

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// Dragging out of a temporary panel offers the referenced files themselves
// (#1604): the panel used to fall back to copying them into a scratch
// directory, and the reporter saw it close as the pointer left the panel.
func TestTempPanelDragOutOffersTheReferencedFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("temporary"), 0600); err != nil {
		t.Fatal(err)
	}
	source := vfs.NewOSVFS(root)
	store := &TempPanelStore{}
	tmp := NewTempPanelVFS(source, store, 0)
	if err := tmp.AddReferences(context.Background(), source, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	refs := store.references(0)
	if len(refs) != 1 {
		t.Fatalf("references = %#v", refs)
	}

	got, ok := tmp.LocalPaths([]string{refs[0].display})
	if !ok || len(got) != 1 || got[0] != file {
		t.Fatalf("LocalPaths = %q, %v, want %q", got, ok, file)
	}
	if _, ok := tmp.LocalPaths([]string{"missing"}); ok {
		t.Fatal("an entry that is not in the panel must not resolve")
	}
	if _, ok := tmp.LocalPaths(nil); ok {
		t.Fatal("nothing selected must not resolve")
	}

	paths, ok := LocalDragPaths(&FileSystemPanel{Vfs: tmp}, []string{refs[0].display})
	if !ok || len(paths) != 1 || paths[0] != file {
		t.Fatalf("LocalDragPaths over a temporary panel = %q, %v, want %q", paths, ok, file)
	}

	// A reference into a file system that is not the local disk is copied out
	// as before.
	remoteStore := &TempPanelStore{}
	remoteStore.appendReferences(0, []tempPanelReference{{source: vfs.NewNullVFS(0), path: "x", display: "x"}})
	remote := NewTempPanelVFS(source, remoteStore, 0)
	if _, ok := remote.LocalPaths([]string{"x"}); ok {
		t.Fatal("a non-local reference must not be offered as a local path")
	}
}
