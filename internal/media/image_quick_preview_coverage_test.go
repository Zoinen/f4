package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestImageQuickPreviewRejectsUnsupportedAndMalformedFiles(t *testing.T) {
	if _, _, err := imageQuickPreview(context.Background(), nil, "picture.png"); err == nil {
		t.Fatal("non-JPEG preview was accepted")
	}

	root := t.TempDir()
	empty := filepath.Join(root, "empty.jpg")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := imageReadHead(context.Background(), vfs.NewOSVFS(root), empty, 32); err == nil {
		t.Fatal("empty file read was accepted")
	}

	malformed := filepath.Join(root, "malformed.jpg")
	if err := os.WriteFile(malformed, []byte{0xff, 0xd8, 0xff, 0xda}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, decoder, err := imageQuickPreview(context.Background(), vfs.NewOSVFS(root), malformed); err == nil || decoder != "" {
		t.Fatalf("malformed JPEG preview = decoder %q, err %v", decoder, err)
	}
}
