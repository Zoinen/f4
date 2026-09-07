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
	if _, err := wc.Write([]byte("VFS Test Data")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close writer failed: %v", err)
	}

	rc, err := vfs.Open(context.Background(), filePath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if rc.Size() != 13 {
		t.Errorf("Expected file size 13, got %d", rc.Size())
	}

	buf := make([]byte, 4)
	n, err := rc.ReadAt(context.Background(), buf, 4)
	if closeErr := rc.Close(); closeErr != nil {
		t.Fatalf("Close reader failed: %v", closeErr)
	}
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
	if err := os.Mkdir(targetDir, 0700); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

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
func TestOSVFS_Lstat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlinks on Windows require special privileges")
	}

	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("hello world"), 0600); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(targetFile, linkFile); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	statItem, err := v.Stat(context.Background(), linkFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if statItem.Size != 11 {
		t.Errorf("Stat on symlink should give target size 11, got %d", statItem.Size)
	}

	lstatItem, err := v.Lstat(context.Background(), linkFile)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if !lstatItem.IsSymlink {
		t.Error("Lstat item should have IsSymlink = true")
	}
	if lstatItem.Size == 11 {
		t.Errorf("Lstat on symlink should give symlink size (len of target path), got target size %d", lstatItem.Size)
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
		file := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
			t.Fatalf("Failed to create %s: %v", file, err)
		}
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

func TestOSVFS_ReadDirPhasedPublishesStableBaseBeforeMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "alpha.txt"), []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "folder"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".dot-entry"), []byte("dot"), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewOSVFS(tmpDir)
	var phases []DirectoryReadPhase
	base := make(map[string]VFSItem)
	metadata := make(map[string]VFSItem)
	err := v.ReadDirPhased(context.Background(), tmpDir, func(phase DirectoryReadPhase, chunk []VFSItem) {
		phases = append(phases, phase)
		for _, item := range chunk {
			switch phase {
			case DirectoryReadBase:
				base[item.Name] = item
			case DirectoryReadMetadata:
				metadata[item.Name] = item
			}
		}
	})
	if err != nil {
		t.Fatalf("ReadDirPhased failed: %v", err)
	}
	if len(phases) == 0 || phases[0] != DirectoryReadBase {
		t.Fatalf("phase order = %v, want an authoritative base first", phases)
	}
	seenMetadata := false
	for _, phase := range phases {
		if phase == DirectoryReadMetadata {
			seenMetadata = true
		}
		if seenMetadata && phase == DirectoryReadBase {
			t.Fatalf("base phase appeared after metadata: %v", phases)
		}
	}
	if len(base) != 3 {
		t.Fatalf("base size = %d, want 3", len(base))
	}
	if runtime.GOOS != "windows" && len(metadata) != len(base) {
		t.Fatalf("base/metadata sizes = %d/%d, want 3/3", len(base), len(metadata))
	}
	for name, baseItem := range base {
		if runtime.GOOS == "windows" {
			if !baseItem.SizeKnown || baseItem.MTime.IsZero() {
				t.Fatalf("Windows base row %q did not retain enumerated metadata: %+v", name, baseItem)
			}
			continue
		}
		metadataItem, ok := metadata[name]
		if !ok {
			t.Fatalf("metadata missing base row %q", name)
		}
		if baseItem.Name != metadataItem.Name || baseItem.IsDir != metadataItem.IsDir ||
			baseItem.IsHidden != metadataItem.IsHidden || baseItem.IsSymlink != metadataItem.IsSymlink {
			t.Fatalf("row identity/type changed between phases: base=%+v metadata=%+v", baseItem, metadataItem)
		}
		if baseItem.SizeKnown || !baseItem.MTime.IsZero() {
			t.Fatalf("base row %q leaked deferred metadata: %+v", name, baseItem)
		}
	}
	metadataSource := metadata
	if runtime.GOOS == "windows" {
		metadataSource = base
	}
	if got := metadataSource["alpha.txt"]; !got.SizeKnown || got.Size != int64(len("payload")) || got.MTime.IsZero() {
		t.Fatalf("file metadata was not enriched: %+v", got)
	}
	if got := metadataSource["folder"]; !got.IsDir || !got.SizeKnown {
		t.Fatalf("directory metadata was not enriched: %+v", got)
	}
}

func TestOSVFS_ReadDirPhasedPublishesAuthoritativeEmptyBase(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)
	var phases []DirectoryReadPhase
	var base []VFSItem
	err := v.ReadDirPhased(context.Background(), tmpDir, func(phase DirectoryReadPhase, chunk []VFSItem) {
		phases = append(phases, phase)
		if phase == DirectoryReadBase {
			base = append(base, chunk...)
		}
	})
	if err != nil {
		t.Fatalf("ReadDirPhased failed: %v", err)
	}
	if len(phases) != 1 || phases[0] != DirectoryReadBase {
		t.Fatalf("phases = %v, want one authoritative base callback", phases)
	}
	if len(base) != 0 {
		t.Fatalf("empty directory base contained %d rows: %+v", len(base), base)
	}
}

func TestOSVFS_ReadDirPhasedCancellationAfterBaseSkipsMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewOSVFS(tmpDir)
	ctx, cancel := context.WithCancel(context.Background())
	metadataCallbacks := 0
	err := v.ReadDirPhased(ctx, tmpDir, func(phase DirectoryReadPhase, _ []VFSItem) {
		if phase == DirectoryReadBase {
			cancel()
		} else {
			metadataCallbacks++
		}
	})
	if err != context.Canceled {
		t.Fatalf("ReadDirPhased error = %v, want context.Canceled", err)
	}
	if metadataCallbacks != 0 {
		t.Fatalf("received %d metadata callbacks after base cancellation", metadataCallbacks)
	}
}

func TestOSVFS_ReadDirPhasedPreservesSymlinkDirectoryClassification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlinks on Windows require special privileges")
	}
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(tmpDir, "link")); err != nil {
		t.Fatal(err)
	}
	v := NewOSVFS(tmpDir)
	var base, metadata VFSItem
	err := v.ReadDirPhased(context.Background(), tmpDir, func(phase DirectoryReadPhase, chunk []VFSItem) {
		for _, item := range chunk {
			if item.Name != "link" {
				continue
			}
			if phase == DirectoryReadBase {
				base = item
			} else {
				metadata = item
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !base.IsDir || !base.IsSymlink || !metadata.IsDir || !metadata.IsSymlink {
		t.Fatalf("symlink directory classification changed: base=%+v metadata=%+v", base, metadata)
	}
}
func TestOSVFS_SetPath_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	// 1. Existing dir -> Success
	sub := filepath.Join(tmpDir, "exist")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	if err := v.SetPath(sub); err != nil {
		t.Errorf("SetPath failed for existing dir: %v", err)
	}

	// 2. Non-existent path -> Error
	if err := v.SetPath(filepath.Join(tmpDir, "missing")); err == nil {
		t.Error("SetPath should fail for non-existent path")
	}

	// 3. Path is a file -> Error
	file := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := v.SetPath(file); err == nil {
		t.Error("SetPath should fail if path is a file")
	}
}

func TestOSVFS_SetPathOptimisticDefersValidationToReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)
	missing := filepath.Join(tmpDir, "stale-row")

	previousHook := OSVFSSetPathBenchmarkHook
	statEvents := 0
	OSVFSSetPathBenchmarkHook = func(event string, fields ...any) {
		statEvents++
	}
	t.Cleanup(func() { OSVFSSetPathBenchmarkHook = previousHook })

	if err := v.SetPathOptimistic(missing); err != nil {
		t.Fatalf("SetPathOptimistic rejected an accepted panel path: %v", err)
	}
	if got := v.GetPath(); got != filepath.Clean(missing) {
		t.Fatalf("optimistic path = %q, want %q", got, filepath.Clean(missing))
	}
	if statEvents != 0 {
		t.Fatalf("SetPathOptimistic performed %d synchronous validation stages", statEvents)
	}

	err := v.ReadDir(context.Background(), v.GetPath(), func([]VFSItem) {})
	if !os.IsNotExist(err) {
		t.Fatalf("background ReadDir error = %v, want not-exist validation failure", err)
	}
}

func TestOSVFS_SetPathOptimisticNormalizesRelativeRowWithoutStat(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)
	child := filepath.Join(tmpDir, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatalf("create child: %v", err)
	}

	if err := v.SetPathOptimistic("child"); err != nil {
		t.Fatalf("SetPathOptimistic relative row: %v", err)
	}
	if got := v.GetPath(); got != filepath.Clean(child) {
		t.Fatalf("optimistic relative path = %q, want %q", got, filepath.Clean(child))
	}
}
func TestOSVFS_Abs_Consistency(t *testing.T) {
	// Test that Abs is relative to the VFS path, not process CWD
	tmp := t.TempDir()
	vfsPath := filepath.Join(tmp, "vfs_root")
	if err := os.Mkdir(vfsPath, 0700); err != nil {
		t.Fatalf("Failed to create VFS root: %v", err)
	}

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
	if err := os.MkdirAll(vfsRoot, 0700); err != nil {
		t.Fatalf("Failed to create VFS root: %v", err)
	}

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
	if err := os.Mkdir(filepath.Join(tmpDir, subDir), 0700); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

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
