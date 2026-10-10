//go:build !windows

package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unicode/utf8"
)

func TestOSVFSPhasedListingPreservesFilenameBytesAndMetadata(t *testing.T) {
	for _, name := range []string{"file", "file-\xff"} {
		t.Run(name, func(t *testing.T) { checkPhasedFilenameAndMetadata(t, name) })
	}
}

func checkPhasedFilenameAndMetadata(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("contents"), 0o600); err != nil {
		if !utf8.ValidString(name) && errors.Is(err, syscall.EILSEQ) {
			t.Skip("host filesystem rejects filenames containing invalid UTF-8")
		}
		t.Fatal(err)
	}
	filesystem := NewOSVFS(dir)
	var plain VFSItem
	if err := filesystem.ReadDir(context.Background(), dir, func(items []VFSItem) { plain = items[0] }); err != nil {
		t.Fatal(err)
	}
	var base, metadata VFSItem
	if err := filesystem.ReadDirPhased(context.Background(), dir, func(phase DirectoryReadPhase, items []VFSItem) {
		if phase == DirectoryReadBase {
			base = items[0]
		} else if phase == DirectoryReadMetadata {
			metadata = items[0]
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(base.Name) || base.Name != plain.Name || metadata.Name != plain.Name {
		t.Fatalf("phased filenames differ from normal listing: base=%q metadata=%q plain=%q", base.Name, metadata.Name, plain.Name)
	}
	if metadata.KnownMetadata != plain.KnownMetadata || metadata.UnixMode != plain.UnixMode || metadata.ReparseTag != plain.ReparseTag || !metadata.ATime.Equal(plain.ATime) || !metadata.CTime.Equal(plain.CTime) {
		t.Fatalf("phased metadata = %#v, normal metadata = %#v", metadata, plain)
	}
	if _, err := filesystem.Stat(context.Background(), filesystem.Join(dir, base.Name)); err != nil {
		t.Fatalf("display filename does not round-trip to the host: %v", err)
	}
}
