//go:build unix

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOSVFS_SpecialFiles(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	fifoPath := filepath.Join(tmpDir, "test.fifo")
	err := unix.Mkfifo(fifoPath, 0666)
	if err != nil {
		t.Skipf("Mkfifo not supported or failed: %v", err)
	}

	// 1. Try to open FIFO for reading - should fail immediately with os.ErrInvalid instead of hanging
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = v.Open(ctx, fifoPath)
	if err != os.ErrInvalid {
		t.Errorf("Expected os.ErrInvalid when opening FIFO, got %v", err)
	}

	// 2. Try to create/overwrite FIFO - should fail immediately
	_, err = v.Create(ctx, fifoPath)
	if err != os.ErrInvalid {
		t.Errorf("Expected os.ErrInvalid when creating over FIFO, got %v", err)
	}
}
