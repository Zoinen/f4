package vfs

import (
	"context"
	"github.com/unxed/f4/vfs/hostmode"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTrailingDotsSupport(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Trailing dots issue is primarily a Windows API quirk")
	}

	tempDir := t.TempDir()
	vfs := NewOSVFS(tempDir)
	ctx := context.Background()

	// 1. Test Directory with trailing dot
	dotDirPath := filepath.Join(tempDir, "folder.")
	t.Logf("diag: hostmode.Posix=%v prepareOSPath=%q", hostmode.Posix(), prepareOSPath(dotDirPath))
	err := vfs.MkDir(ctx, dotDirPath)
	if err != nil {
		t.Fatalf("Failed to MkDir with trailing dot: %v", err)
	}

	// Verify it actually created "folder." and not "folder"
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}
	foundDir := false
	for _, e := range entries {
		if e.Name() == "folder." {
			foundDir = true
			break
		}
		if e.Name() == "folder" {
			t.Fatalf("OS stripped the dot! Created 'folder' instead of 'folder.'")
		}
	}
	if !foundDir {
		t.Fatalf("Could not find 'folder.' in directory listing")
	}

	// 2. Test File with trailing dot inside the dot directory
	err = vfs.SetPath(dotDirPath)
	if err != nil {
		t.Fatalf("Failed to SetPath to dot directory: %v", err)
	}

	dotFilePath := vfs.Join(vfs.GetPath(), "file.")
	f, err := vfs.Create(ctx, dotFilePath)
	if err != nil {
		t.Fatalf("Failed to Create file with trailing dot: %v", err)
	}
	if _, err := f.Write([]byte("test")); err != nil {
		t.Fatalf("Failed to write file with trailing dot: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Failed to close file with trailing dot: %v", err)
	}

	// 3. Test ReadDir and Stat
	stat, err := vfs.Stat(ctx, dotFilePath)
	if err != nil {
		t.Fatalf("Failed to Stat file with trailing dot: %v", err)
	}
	if stat.Name != "file." {
		t.Errorf("Stat returned wrong name: got %q, want 'file.'", stat.Name)
	}

	var foundFile bool
	err = vfs.ReadDir(ctx, vfs.GetPath(), func(chunk []VFSItem) {
		for _, item := range chunk {
			if item.Name == "file." {
				foundFile = true
			}
		}
	})
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if !foundFile {
		t.Errorf("ReadDir did not return 'file.'")
	}

	// 4. Test Rename
	newFilePath := vfs.Join(vfs.GetPath(), "file_renamed.")
	err = vfs.Rename(ctx, dotFilePath, newFilePath)
	if err != nil {
		t.Fatalf("Failed to Rename file with trailing dot: %v", err)
	}
	_, err = vfs.Stat(ctx, newFilePath)
	if err != nil {
		t.Fatalf("Failed to Stat renamed file: %v", err)
	}

	// 5. Test Remove
	err = vfs.Remove(ctx, newFilePath)
	if err != nil {
		t.Fatalf("Failed to Remove file with trailing dot: %v", err)
	}
	err = vfs.Remove(ctx, dotDirPath)
	if err != nil {
		t.Fatalf("Failed to Remove directory with trailing dot: %v", err)
	}
}
