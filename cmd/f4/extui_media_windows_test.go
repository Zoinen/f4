//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestExtUiMediaPreviewDoesNotLockMoveOrReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, []byte("old image"), 0600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(dir)
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	ctx := context.Background()
	register := func(revision int64) extUiImageSourceDescriptor {
		item, err := fs.Stat(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		d := broker.Register(mediaSourceRegistration{PanelID: "preview", CatalogVersion: revision, FS: fs, Path: path, Item: item})
		broker.CommitPanel("preview", revision, []string{d.ResourceID})
		return d
	}
	old := register(1)
	if data, _, err := broker.ReadRange(ctx, old.ResourceID, 0, 9); err != nil || string(data) != "old image" {
		t.Fatalf("initial preview: %q, %v", data, err)
	}
	moved := filepath.Join(dir, "moved.png")
	if err := os.Rename(path, moved); err != nil {
		t.Fatalf("preview blocks move: %v", err)
	}
	if err := os.Remove(moved); err != nil {
		t.Fatalf("preview blocks removal: %v", err)
	}
	if err := os.WriteFile(path, []byte("replacement image"), 0600); err != nil {
		t.Fatal(err)
	}
	next := register(2)
	if next.ResourceID == old.ResourceID {
		t.Fatal("replacement reused the old content resource")
	}
	if data, _, err := broker.ReadRange(ctx, next.ResourceID, 0, 17); err != nil || string(data) != "replacement image" {
		t.Fatalf("replacement preview: %q, %v", data, err)
	}
}
