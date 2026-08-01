package vfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSVFS_SymlinkPreservation(t *testing.T) {
	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "real_dir")
	os.MkdirAll(realDir, 0755)

	linkDir := filepath.Join(tmpDir, "link_dir")
	err := os.Symlink(realDir, linkDir)
	if err != nil {
		t.Skip("Symlinks not supported on this platform/configuration; skipping test")
	}

	v := NewOSVFS(tmpDir)

	// Навигация внутрь симлинка
	err = v.SetPath(linkDir)
	if err != nil {
		t.Fatalf("SetPath on symlink failed: %v", err)
	}

	// Проверяем, что VFS-путь сохранился как путь к симлинку (а не раскрылся в real_dir)
	got := filepath.Clean(v.GetPath())
	want := filepath.Clean(linkDir)
	if got != want {
		t.Errorf("Expected VFS path to remain the symlink path %q, got resolved path %q", want, got)
	}
}
