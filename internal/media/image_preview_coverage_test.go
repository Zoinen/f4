package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestImageReadHeadHonorsFileSizeAndErrors(t *testing.T) {
	if _, err := imageReadHead(context.Background(), nil, "x.jpg", 10); err == nil {
		t.Fatal("nil VFS must be rejected")
	}

	root := t.TempDir()
	path := filepath.Join(root, "tiny.jpg")
	if err := os.WriteFile(path, []byte("thumbnail"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := imageReadHead(context.Background(), vfs.NewOSVFS(root), path, 100)
	if err != nil || string(got) != "thumbnail" {
		t.Fatalf("short file read = %q, err=%v", got, err)
	}
	if _, err := imageReadHead(context.Background(), vfs.NewOSVFS(root), path, 0); err == nil {
		t.Fatal("zero read limit must be rejected")
	}
}
