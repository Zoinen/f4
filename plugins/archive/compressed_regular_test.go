package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestArchiveVFSCompressedRegularFileUsesLogicalEntry(t *testing.T) {
	payload := bytes.Repeat([]byte("Start-Date: 2026-08-23  10:00:00\n"), 500)
	root := t.TempDir()
	filename := filepath.Join(root, "history.log.1.gz")

	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	archiveVFS, err := NewArchiveVFS(vfs.NewOSVFS(root), filename)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = archiveVFS.Close() })

	var items []vfs.VFSItem
	if err := archiveVFS.ReadDir(context.Background(), archiveVFS.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("ReadDir returned %d entries, want one", len(items))
	}
	if items[0].Name != "history.log.1" {
		t.Fatalf("entry name = %q, want history.log.1", items[0].Name)
	}
	if items[0].Size != int64(len(payload)) {
		t.Fatalf("entry size = %d, want decompressed size %d", items[0].Size, len(payload))
	}

	opened, err := archiveVFS.Open(context.Background(), archiveVFS.Join(archiveVFS.GetPath(), items[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := opened.Read(context.Background(), buf)
		if n > 0 {
			_, _ = got.Write(buf[:n])
		}
		if readErr != nil {
			if readErr != io.EOF {
				t.Fatal(readErr)
			}
			break
		}
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), payload) {
		t.Fatalf("decompressed payload mismatch: got %d bytes, want %d", got.Len(), len(payload))
	}
}
