package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestTempPanelVFSStoresReferencesWithoutCopying(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "one.txt")
	if err := os.WriteFile(filePath, []byte("temporary reference"), 0600); err != nil {
		t.Fatal(err)
	}

	store := &tempPanelStore{}
	tmp := newTempPanelVFS(nil, store, 0)
	source := vfs.NewOSVFS(root)
	if err := tmp.AddReferences(context.Background(), source, []string{"one.txt", "one.txt"}); err != nil {
		t.Fatal(err)
	}

	items := readTempPanelItems(t, tmp)
	if len(items) != 1 {
		t.Fatalf("temporary panel has %d items, want one deduplicated reference", len(items))
	}
	if items[0].Name != filePath {
		t.Fatalf("temporary panel item name = %q, want %q", items[0].Name, filePath)
	}

	entryPath := tmp.Join(tmp.GetPath(), items[0].Name)
	reader, err := tmp.Open(context.Background(), entryPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(readAtCloserReader{reader})
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "temporary reference" {
		t.Fatalf("temporary panel read %q, want original file contents", data)
	}

	if tmp.HandlePanelAction(nil, vfs.PanelActionCreate, []string{entryPath}) {
		t.Fatal("temporary panel consumed the create action used by Shift+F4")
	}
	tmp.removePanelReferences([]string{entryPath})
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("removing a temporary-panel reference touched the source file: %v", err)
	}
	if got := len(readTempPanelItems(t, tmp)); got != 0 {
		t.Fatalf("temporary panel has %d items after removing its reference, want zero", got)
	}
}

func TestTempPanelVFSParentIsRestoredByPanelSwitch(t *testing.T) {
	parent := vfs.NewOSVFS(t.TempDir())
	tmp := newTempPanelVFS(parent, &tempPanelStore{}, 3)
	if got := tmp.ParentVFS(); got != parent {
		t.Fatalf("ParentVFS() = %T, want the source VFS", got)
	}
	if !tmp.IsAtRoot() {
		t.Fatal("new temporary panel is not at its root")
	}
	if got := tmp.GetPath(); got != "tmp:3" {
		t.Fatalf("temporary panel root = %q, want tmp:3", got)
	}
}

func TestTempPanelStoreReplacesSearchResultsInSelectedSlot(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	for _, name := range []string{first, second} {
		if err := os.WriteFile(name, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}

	store := &tempPanelStore{}
	source := vfs.NewOSVFS(root)
	store.replaceWithSearchResults(2, source, []FoundFile{{Path: first, Item: vfs.VFSItem{Name: "first.txt"}}})
	store.replaceWithSearchResults(2, source, []FoundFile{{Path: second, Item: vfs.VFSItem{Name: "second.txt"}}})

	tmp := newTempPanelVFS(nil, store, 2)
	items := readTempPanelItems(t, tmp)
	if len(items) != 1 || items[0].Name != second {
		t.Fatalf("search-result replacement produced %#v, want only %q", items, second)
	}
}

func TestTempPanelVFSDeleteRemovesReferenceAndRealItem(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "delete.txt")
	if err := os.WriteFile(filePath, []byte("delete me"), 0600); err != nil {
		t.Fatal(err)
	}

	store := &tempPanelStore{}
	tmp := newTempPanelVFS(nil, store, 0)
	source := vfs.NewOSVFS(root)
	if err := tmp.AddReferences(context.Background(), source, []string{"delete.txt"}); err != nil {
		t.Fatal(err)
	}
	items := readTempPanelItems(t, tmp)
	if len(items) != 1 {
		t.Fatalf("temporary panel has %d items, want one", len(items))
	}

	entryPath := tmp.Join(tmp.GetPath(), items[0].Name)
	if err := tmp.Remove(context.Background(), entryPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("deleting a temporary-panel item left the real file in place: %v", err)
	}
	if got := len(readTempPanelItems(t, tmp)); got != 0 {
		t.Fatalf("temporary panel has %d items after deletion, want zero", got)
	}
}

type readAtCloserReader struct {
	reader vfs.ReadAtCloser
}

func (r readAtCloserReader) Read(p []byte) (int, error) {
	return r.reader.Read(context.Background(), p)
}

func readTempPanelItems(t *testing.T, tmp *TempPanelVFS) []vfs.VFSItem {
	t.Helper()
	var items []vfs.VFSItem
	if err := tmp.ReadDir(context.Background(), tmp.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	return items
}
