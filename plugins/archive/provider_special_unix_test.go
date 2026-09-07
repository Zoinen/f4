//go:build unix

package archive

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"golang.org/x/sys/unix"
)

func TestArchiveProvider_CanOpen_SpecialFile(t *testing.T) {
	p := &ArchiveProvider{}
	tmpDir := t.TempDir()
	fifoPath := filepath.Join(tmpDir, "test.fifo")
	if err := unix.Mkfifo(fifoPath, 0o666); err != nil {
		t.Fatalf("Mkfifo failed: %v", err)
	}
	parent := vfs.NewOSVFS(tmpDir)
	if p.CanOpen(context.Background(), parent, "test.fifo") {
		t.Error("Expected CanOpen=false for FIFO file")
	}
}
