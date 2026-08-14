//go:build !windows

package archive

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestArchiveProvider_CanOpen_SpecialFile(t *testing.T) {
	p := &ArchiveProvider{}
	tmpDir := t.TempDir()
	fifoPath := filepath.Join(tmpDir, "test.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o666); err != nil {
		t.Fatalf("Mkfifo failed: %v", err)
	}
	parent := vfs.NewOSVFS(tmpDir)
	if p.CanOpen(context.Background(), parent, "test.fifo") {
		t.Error("Expected CanOpen=false for FIFO file")
	}
}
