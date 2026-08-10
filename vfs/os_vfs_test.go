package vfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOSVFS_Mutations(t *testing.T) {
	tmpDir := t.TempDir()
	vfs := NewOSVFS(tmpDir)

	// Test MkDir
	newDirPath := vfs.Join(tmpDir, "new_folder")
	err := vfs.MkDir(context.Background(), newDirPath)
	if err != nil {
		t.Fatalf("MkDir failed: %v", err)
	}

	stat, err := vfs.Stat(context.Background(), newDirPath)
	if err != nil || !stat.IsDir {
		t.Errorf("MkDir did not create a directory properly")
	}

	// Test Create & Open (Write/Read)
	filePath := vfs.Join(newDirPath, "test.txt")
	wc, err := vfs.Create(context.Background(), filePath)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	wc.Write([]byte("VFS Test Data"))
	wc.Close()

	rc, err := vfs.Open(context.Background(), filePath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if rc.Size() != 13 {
		t.Errorf("Expected file size 13, got %d", rc.Size())
	}

	buf := make([]byte, 4)
	n, err := rc.ReadAt(context.Background(), buf, 4)
	rc.Close()
	if err != nil || string(buf[:n]) != "Test" {
		t.Errorf("ReadAt failed. Expected 'Test', got %q", string(buf[:n]))
	}

	// Test Rename
	renamedPath := vfs.Join(newDirPath, "renamed.txt")
	err = vfs.Rename(context.Background(), filePath, renamedPath)
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	_, err = vfs.Stat(context.Background(), filePath)
	if err == nil {
		t.Error("Old file still exists after rename")
	}

	stat, err = vfs.Stat(context.Background(), renamedPath)
	if err != nil || stat.Name != "renamed.txt" {
		t.Error("Renamed file not found or invalid")
	}

	// Test Remove
	err = vfs.Remove(context.Background(), newDirPath)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, err = vfs.Stat(context.Background(), newDirPath)
	if err == nil {
		t.Error("Directory still exists after Remove")
	}
}

func TestOSVFS_Symlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlinks on Windows require special privileges")
	}

	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	// 1. Create a real directory
	targetDir := filepath.Join(tmpDir, "real_folder")
	os.Mkdir(targetDir, 0755)

	// 2. Create a symlink to that directory
	linkPath := filepath.Join(tmpDir, "link_folder")
	err := os.Symlink(targetDir, linkPath)
	if err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// 3. Read directory and check if link_folder is marked as IsDir
	found := false
	err = v.ReadDir(context.Background(), tmpDir, func(items []VFSItem) {
		for _, itm := range items {
			if itm.Name == "link_folder" {
				found = true
				if !itm.IsDir {
					t.Error("Symlink to directory was not recognized as a directory")
				}
			}
		}
	})

	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if !found {
		t.Error("Symlink entry not found in ReadDir")
	}
}

func TestOSVFS_Capabilities(t *testing.T) {
	vfs := NewOSVFS(".")
	caps := vfs.GetCapabilities()

	if !caps.HasRandomAccess || !caps.HasServerSideCopy || !caps.HasServerSideMove {
		t.Error("OSVFS should support RandomAccess, ServerSideCopy, and ServerSideMove")
	}
}

func TestOSVFS_SearchStub(t *testing.T) {
	v := NewOSVFS(".")
	// Check that calling Search doesn't panic and returns nil for now
	ch, err := v.Search(context.Background(), "path", "pattern")
	if err != nil {
		t.Errorf("Search stub should not return error, got %v", err)
	}
	if ch != nil {
		t.Error("Search stub should return nil channel for OSVFS")
	}
}
func TestOSVFS_ReadDir_Cancellation(t *testing.T) {
	tmpDir := t.TempDir()
	// Create 100 files to ensure multiple chunks/iterations
	for i := 0; i < 100; i++ {
		os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i)), []byte("data"), 0644)
	}

	v := NewOSVFS(tmpDir)
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	// Start ReadDir and cancel immediately
	cancel()

	err := v.ReadDir(ctx, tmpDir, func(items []VFSItem) {
		count += len(items)
	})

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
	if count > 0 {
		t.Errorf("Callback should not have been called after cancellation, but got %d items", count)
	}
}
func TestOSVFS_SetPath_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	// 1. Existing dir -> Success
	sub := filepath.Join(tmpDir, "exist")
	os.Mkdir(sub, 0755)
	if err := v.SetPath(sub); err != nil {
		t.Errorf("SetPath failed for existing dir: %v", err)
	}

	// 2. Non-existent path -> Error
	if err := v.SetPath(filepath.Join(tmpDir, "missing")); err == nil {
		t.Error("SetPath should fail for non-existent path")
	}

	// 3. Path is a file -> Error
	file := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(file, []byte("data"), 0644)
	if err := v.SetPath(file); err == nil {
		t.Error("SetPath should fail if path is a file")
	}
}
func TestOSVFS_Abs_Consistency(t *testing.T) {
	// Test that Abs is relative to the VFS path, not process CWD
	tmp := t.TempDir()
	vfsPath := filepath.Join(tmp, "vfs_root")
	os.Mkdir(vfsPath, 0755)

	v := NewOSVFS(vfsPath)

	// Even if we change process CWD (not recommended in tests, but for clarity)
	abs, err := v.Abs("file.txt")
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(vfsPath, "file.txt")
	if abs != expected {
		t.Errorf("Abs failed: expected %q, got %q", expected, abs)
	}
}

func TestOSVFS_LocalPathProvider(t *testing.T) {
	root := t.TempDir()
	v := NewOSVFS(root)

	provider, ok := any(v).(LocalPathProvider)
	if !ok {
		t.Fatal("OSVFS does not implement LocalPathProvider")
	}
	got, err := provider.LocalPath("preview.jpg")
	if err != nil {
		t.Fatalf("LocalPath returned error: %v", err)
	}
	want := filepath.Join(root, "preview.jpg")
	if got != want {
		t.Fatalf("LocalPath = %q, want %q", got, want)
	}
}
func TestOSVFS_Abs_CWD_Independence(t *testing.T) {
	// Create a folder structure: /tmp/root/subdir
	tmp := t.TempDir()
	vfsRoot := filepath.Join(tmp, "vfs_root")
	os.MkdirAll(vfsRoot, 0755)

	v := NewOSVFS(vfsRoot)

	// Simulate process running in a completely different directory
	// (we don't change os.Chdir because it's global and bad for parallel tests,
	// but we check that VFS doesn't use it).

	relPath := "somefile.txt"
	abs, err := v.Abs(relPath)
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(vfsRoot, relPath)
	if abs != expected {
		t.Errorf("OSVFS.Abs depends on process CWD! Expected %q, got %q", expected, abs)
	}
}

func TestOSVFS_SetPath_Relative(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := "my_sub_folder"
	os.Mkdir(filepath.Join(tmpDir, subDir), 0755)

	v := NewOSVFS(tmpDir)

	// Test navigating into subfolder by relative name
	err := v.SetPath(subDir)
	if err != nil {
		t.Fatalf("SetPath(relative) failed: %v", err)
	}

	expected, _ := filepath.Abs(filepath.Join(tmpDir, subDir))
	if v.GetPath() != expected {
		t.Errorf("Path mismatch: expected %q, got %q", expected, v.GetPath())
	}
}
