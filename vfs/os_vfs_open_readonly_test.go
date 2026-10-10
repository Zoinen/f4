package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestOSVFS_OpenRegularFileIsReadOnly pins what #1364 depends on: the handle
// Open gives out for a regular file carries no write access. The editor and
// the viewer hold that handle for as long as they show the file, and on
// Windows write access on it is what locks other programs' readers out.
func TestOSVFS_OpenRegularFileIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("line1 1\nline2 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := NewOSVFS(dir).Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	w, ok := f.(interface {
		WriteAt(p []byte, off int64) (int, error)
	})
	if !ok {
		t.Fatalf("%T no longer exposes WriteAt; rewrite this test against the new wrapper", f)
	}
	if n, err := w.WriteAt([]byte("X"), 0); err == nil {
		t.Fatalf("WriteAt through the Open handle wrote %d byte(s): the handle has write access", n)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "line1 1\nline2 2\n" {
		t.Fatalf("file changed through the Open handle: %q", got)
	}
}
