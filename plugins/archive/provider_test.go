package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestArchiveProvider_Properties(t *testing.T) {
	p := &ArchiveProvider{}

	if p.Name() != "zipper/archive" {
		t.Errorf("Unexpected name: %q", p.Name())
	}

	if p.Priority() != 10 {
		t.Errorf("Unexpected priority: %d", p.Priority())
	}
}

func TestArchiveProvider_CanOpen(t *testing.T) {
	p := &ArchiveProvider{}
	ctx := context.Background()

	// 1. Valid formats (e.g. .zip)
	tmpZip := filepath.Join(t.TempDir(), "test.zip")
	os.WriteFile(tmpZip, []byte("PK\x03\x04..."), 0644) // Zip magic bytes
	if !p.CanOpen(ctx, nil, tmpZip) {
		t.Errorf("Expected CanOpen=true for %q", tmpZip)
	}

	// 2. Invalid formats (e.g. .txt)
	tmpTxt := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(tmpTxt, []byte("plain text"), 0644)
	if p.CanOpen(ctx, nil, tmpTxt) {
		t.Errorf("Expected CanOpen=false for %q", tmpTxt)
	}
}
func TestArchiveProvider_Open(t *testing.T) {
	p := &ArchiveProvider{}
	ctx := context.Background()

	// Setup simple valid zip
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "empty.zip")
	// Write empty zip structure (22 bytes EOCD)
	os.WriteFile(zipPath, []byte("\x50\x4b\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), 0644)

	parent := vfs.NewOSVFS(tmpDir)

	v, err := p.Open(ctx, parent, zipPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer v.Close()

	if _, ok := v.(*ArchiveVFS); !ok {
		t.Errorf("Expected Open to return *ArchiveVFS, got %T", v)
	}
}
