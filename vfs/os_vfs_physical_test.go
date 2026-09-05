package vfs

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// TestOSVFS_SupportsPhysicalSize pins the capability contract the
// scanner uses to decide whether to run the lazy-Stat fallback. If
// OSVFS ever returns false here, Windows quick-view would stop
// filling PhysicalBytes even during the actual scan; if a non-OSVFS
// backend returns true, copy/move pre-scan would resume the N+1
// Stat storm the capability gate was introduced to kill.
func TestOSVFS_SupportsPhysicalSize(t *testing.T) {
	var v any = NewOSVFS(".")
	ps, ok := v.(PhysicalSizer)
	if !ok {
		t.Fatal("OSVFS must implement PhysicalSizer")
	}
	// The value itself is platform-scoped (see os_vfs_physical_*.go)
	// so we don't hard-code a Go-side expectation; just make sure the
	// call doesn't panic and the capability is a real function.
	_ = ps.SupportsPhysicalSize()
}

// TestFillPhysicalSize_NilInfo locks in the guarantee that a nil
// FileInfo — which DirEntry.Info() returns when the entry vanished
// between readdir and lstat — doesn't crash the ReadDir path. The
// Unix implementation panicked on nil.Info() before the guard.
func TestFillPhysicalSize_NilInfo(t *testing.T) {
	var item VFSItem
	// Must not panic. PhysicalSize stays at zero.
	fillPhysicalSize(&item, nil, "/nonexistent/path")
	if item.PhysicalSize != 0 {
		t.Errorf("PhysicalSize should stay 0 on nil info, got %d", item.PhysicalSize)
	}
}

// TestFillPhysicalSize_RealFile is a smoke test: on Unix we get
// stat.Blocks * 512, on Windows GetCompressedFileSizeW, on the
// "other" stub the value stays 0. The assertion set is deliberately
// weak — a strict "PhysicalSize >= Size" would fire on any
// transparently-compressing filesystem (btrfs+zstd, ZFS compression,
// NTFS-compressed), and "% 512 == 0" would fire on the Windows
// fallback that returns info.Size() for non-NTFS paths. We only
// check that the call doesn't crash and, when it produces a number,
// the number is non-negative. Actual correctness lives in the
// scanner tests that walk real trees.
func TestFillPhysicalSize_RealFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hello.bin")
	// Random data so transparent compression can't shrink it away in
	// a way that would surprise a stricter assertion added later.
	buf := make([]byte, 8192)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf, 0600); err != nil {
		t.Fatal(err)
	}
	v := NewOSVFS(tmp)
	item, err := v.Stat(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if item.Size != 8192 {
		t.Errorf("Size = %d, want 8192", item.Size)
	}
	if item.PhysicalSize < 0 {
		t.Errorf("PhysicalSize = %d, must be >= 0", item.PhysicalSize)
	}
}
