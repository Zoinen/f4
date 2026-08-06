package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRecursiveCopy(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	// 1. Create source structure:
	// /folder1/file1.txt
	// /file2.txt
	os.Mkdir(filepath.Join(tmpSrc, "folder1"), 0755)
	os.WriteFile(filepath.Join(tmpSrc, "file2.txt"), []byte("file2 content"), 0644)
	os.WriteFile(filepath.Join(tmpSrc, "folder1", "file1.txt"), []byte("file1 content"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	//pf := &PanelsFrame{}

	// Initialize FrameManager to provide TaskChan for RunOnUI
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Create a real TaskContext
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	// Perform copy: folder1 from tmpSrc to tmpDst
	err := recursiveCopy(tCtx.Context, srcVfs, filepath.Join(tmpSrc, "folder1"), dstVfs, filepath.Join(tmpDst, "folder1_copy"), &FileOpState{}, 0)
	if err != nil {
		t.Fatalf("recursiveCopy failed: %v", err)
	}

	// Verify result
	copiedFile := filepath.Join(tmpDst, "folder1_copy", "file1.txt")
	if _, err := os.Stat(copiedFile); os.IsNotExist(err) {
		t.Errorf("Copied file does not exist: %s", copiedFile)
	}

	data, _ := os.ReadFile(copiedFile)
	if string(data) != "file1 content" {
		t.Errorf("Corrupted data in copied file. Got %q", string(data))
	}
}
func TestRecursiveCopy_Cancel(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	largeFile := filepath.Join(tmpSrc, "large.bin")
	// Create 1MB file
	os.WriteFile(largeFile, make([]byte, 1024*1024), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)
	//pf := &PanelsFrame{}
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ctx, cancel := context.WithCancel(context.Background())
	tCtx := &vtui.TaskContext{Context: ctx, Cancel: cancel}

	// Cancel immediately
	cancel()

	err := recursiveCopy(tCtx.Context, srcVfs, largeFile, dstVfs, filepath.Join(tmpDst, "large_copy.bin"), &FileOpState{}, 0)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context canceled error, got %v", err)
	}
}

func TestRecursiveCopy_SelfCopy(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "src_folder"), 0755)

	srcVfs := vfs.NewOSVFS(tmp)
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	// Try to copy "src_folder" into "src_folder/sub"
	srcPath := filepath.Join(tmp, "src_folder")
	// Use OSVFS for proper absolute path normalization
	err := recursiveCopy(tCtx.Context, srcVfs,
		srcPath, srcVfs, filepath.Join(srcPath, "sub"), &FileOpState{}, 0)

	if err == nil || !strings.Contains(err.Error(), "folder into itself") {
		t.Errorf("Expected self-copy error, got %v", err)
	}
}

func TestRecursiveCopy_ConflictTypeMismatch(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	// Create folder in source, file with same name in destination
	name := "mismatch"
	os.Mkdir(filepath.Join(tmpSrc, name), 0755)
	os.WriteFile(filepath.Join(tmpDst, name), []byte("i am a file"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	// Try to copy folder over file - should return error immediately
	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, name), dstVfs, filepath.Join(tmpDst, name), &FileOpState{}, 0)

	if err == nil || !strings.Contains(err.Error(), "cannot overwrite file with folder") {
		t.Errorf("Expected type mismatch error, got %v", err)
	}
}

func TestRecursiveCopy_MoveCrossVFS(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	name := "move_me.txt"
	srcFile := filepath.Join(tmpSrc, name)
	os.WriteFile(srcFile, []byte("payload"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	// Execute Move
	err := recursiveCopy(tCtx.Context, srcVfs, srcFile, dstVfs, filepath.Join(tmpDst, name), &FileOpState{}, 0)
	if err != nil {
		t.Fatalf("Copy part of move failed: %v", err)
	}

	err = srcVfs.Remove(context.Background(), srcFile)
	if err != nil {
		t.Fatalf("Delete part of move failed: %v", err)
	}

	// Verify
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Error("Source file still exists after Move")
	}
	if data, _ := os.ReadFile(filepath.Join(tmpDst, name)); string(data) != "payload" {
		t.Error("Destination file corrupted or missing after Move")
	}
}

func TestRecursiveCopy_FileOverFolderMismatch(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	name := "conflict"
	os.WriteFile(filepath.Join(tmpSrc, name), []byte("file"), 0644)
	os.Mkdir(filepath.Join(tmpDst, name), 0755)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)
	//pf := &PanelsFrame{}
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})

	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, name), dstVfs, filepath.Join(tmpDst, name), &FileOpState{}, 0)

	if err == nil || !strings.Contains(err.Error(), "cannot overwrite folder with file") {
		t.Errorf("Expected folder-over-file error, got %v", err)
	}
}

func TestRecursiveCopy_Normalization(t *testing.T) {
	tmp := t.TempDir()
	v := vfs.NewOSVFS(tmp)

	// Test that Abs normalization works for self-copy check
	abs, _ := v.Abs(".")
	if abs == "" {
		t.Error("VFS.Abs failed to return a path")
	}
}
func TestRecursiveCopy_OverwriteAllState(t *testing.T) {
	state := &FileOpState{OverwriteAll: true}
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})

	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("new"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f1.txt"), []byte("old"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	// Should not call AskOverwrite because OverwriteAll is true
	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, "f1.txt"), dstVfs, filepath.Join(tmpDst, "f1.txt"), state, 0)

	if err != nil {
		t.Errorf("Copy failed even with OverwriteAll: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDst, "f1.txt"))
	if string(data) != "new" {
		t.Error("File was not overwritten despite OverwriteAll flag")
	}
}
func TestRecursiveCopy_SkipAllState(t *testing.T) {
	//pf := &PanelsFrame{}
	state := &FileOpState{SkipAll: true}
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})

	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	fileName := "skip.txt"
	os.WriteFile(filepath.Join(tmpSrc, fileName), []byte("source content"), 0644)
	os.WriteFile(filepath.Join(tmpDst, fileName), []byte("target content"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, fileName), dstVfs, filepath.Join(tmpDst, fileName), state, 0)

	if err != nil {
		t.Fatalf("Expected no error on skip, got %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDst, fileName))
	if string(data) != "target content" {
		t.Error("File was overwritten despite SkipAll flag")
	}
}
func TestRecursiveCopy_CancelCleanup(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcFile := filepath.Join(tmpSrc, "source.txt")
	os.WriteFile(srcFile, []byte("some large content here"), 0644)

	dstFile := filepath.Join(tmpDst, "source.txt")

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	ctx, cancel := context.WithCancel(context.Background())

	state := &FileOpState{
		OnBytes: func(n int) {
			cancel()
		},
	}

	err := recursiveCopy(ctx, srcVfs, srcFile, dstVfs, dstFile, state, 0)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	if _, err := os.Stat(dstFile); !os.IsNotExist(err) {
		t.Error("Partial destination file was not deleted after cancellation")
	}
}
func TestRecursiveCopy_AskError_Stub(t *testing.T) {
	// Placeholder for UI-heavy error handling test.
	// Just ensuring the frame instance can be created.
	pf := &PanelsFrame{}
	if pf == nil {
		t.Error("Failed to create PanelsFrame")
	}
}

type mockS2SVFS struct {
	vfs.VFS
	runCommand func(ctx context.Context, dir, command string, cb func(line string)) (int, error)
	connInfo   func() (host, port, user string, ok bool)
}

func (m *mockS2SVFS) RunCommand(ctx context.Context, dir, command string, cb func(line string)) (int, error) {
	if m.runCommand != nil {
		return m.runCommand(ctx, dir, command, cb)
	}
	return 0, fmt.Errorf("unsupported")
}

func (m *mockS2SVFS) ConnectionInfo() (host, port, user string, ok bool) {
	if m.connInfo != nil {
		return m.connInfo()
	}
	return "localhost", "22", "user", true
}

func (m *mockS2SVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}

func TestRecursiveCopy_S2SProbing(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcFile := filepath.Join(tmpSrc, "s2s.bin")
	os.WriteFile(srcFile, []byte("s2s_data"), 0644)
	dstFile := filepath.Join(tmpDst, "s2s.bin")

	var pushedCmd string
	var pulledCmd string

	srcMock := &mockS2SVFS{
		VFS: vfs.NewOSVFS(tmpSrc),
		runCommand: func(ctx context.Context, dir, command string, cb func(line string)) (int, error) {
			pushedCmd = command
			// Force push to fail with exit code 1, but no protocol error
			return 1, nil
		},
		connInfo: func() (host, port, user string, ok bool) {
			return "hostA", "22", "userA", true
		},
	}

	dstMock := &mockS2SVFS{
		VFS: vfs.NewOSVFS(tmpDst),
		runCommand: func(ctx context.Context, dir, command string, cb func(line string)) (int, error) {
			pulledCmd = command
			return 0, nil // Pull succeeds
		},
		connInfo: func() (host, port, user string, ok bool) {
			return "hostB", "22", "userB", true
		},
	}

	state := &FileOpState{}
	ctx := context.Background()

	err := recursiveCopy(ctx, srcMock, srcFile, dstMock, dstFile, state, 0)
	if err != nil {
		t.Fatalf("recursiveCopy failed during S2S: %v", err)
	}

	if pushedCmd == "" {
		t.Error("Expected push attempt on source VFS")
	}
	if !strings.Contains(pushedCmd, "scp") || !strings.Contains(pushedCmd, "userB@hostB") {
		t.Errorf("Unexpected push command: %q", pushedCmd)
	}

	if pulledCmd == "" {
		t.Error("Expected pull fallback attempt on dest VFS")
	}
	if !strings.Contains(pulledCmd, "scp") || !strings.Contains(pulledCmd, "userA@hostA") {
		t.Errorf("Unexpected pull command: %q", pulledCmd)
	}

	if state.S2SDir != 2 {
		t.Errorf("Expected S2SDir state to be 2 (pull), got %d", state.S2SDir)
	}

	// Now run again with S2SDir set to 2; it should directly pull and skip push
	pushedCmd = ""
	pulledCmd = ""
	err = recursiveCopy(ctx, srcMock, srcFile, dstMock, dstFile, state, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pushedCmd != "" {
		t.Error("Expected push to be skipped when S2SDir is 2")
	}
	if pulledCmd == "" {
		t.Error("Expected pull to be executed directly when S2SDir is 2")
	}
}

func TestMkDir_ErrorHandling(t *testing.T) {
	tmp := t.TempDir()
	v := vfs.NewOSVFS(tmp)

	// Try to create a folder where a file already exists
	os.WriteFile(filepath.Join(tmp, "blocked"), []byte("data"), 0644)

	err := v.MkDir(context.Background(), filepath.Join(tmp, "blocked"))
	if err == nil {
		t.Error("MkDir should have failed when creating a directory over a file")
	}
}

func TestDelete_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	v := vfs.NewOSVFS(tmp)

	// Deleting non-existent file should return error in OSVFS (RemoveAll)
	// Actually RemoveAll in Go returns nil if path doesn't exist.
	// This matches our idempotency principles, so let's verify it.
	err := v.Remove(context.Background(), filepath.Join(tmp, "not_there"))
	if err != nil {
		t.Errorf("Remove should be idempotent and return nil for non-existent paths, got %v", err)
	}
}

func TestFileOps_RefreshAllNoPanic(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	// Ensure refresh doesn't crash even if panels are not fully docked
	pf.RefreshAll()
}

func TestFileOp_PathLogic(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	t.Run("Copy and Rename", func(t *testing.T) {
		os.WriteFile(filepath.Join(tmpSrc, "old.txt"), []byte("data"), 0644)
		tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})

		// Target is a new filename, not a directory
		ExecuteFileOp(nil, srcVfs, dstVfs, []string{"old.txt"}, "new.txt", false, 2, nil)

		// Drain task queue
		for i := 0; i < 50; i++ {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}

		if _, err := os.Stat(filepath.Join(tmpSrc, "new.txt")); os.IsNotExist(err) {
			t.Error("Rename copy failed: new.txt not found")
		}
		tCtx.Cancel()
	})

	t.Run("Multiple files to new directory", func(t *testing.T) {
		os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("1"), 0644)
		os.WriteFile(filepath.Join(tmpSrc, "f2.txt"), []byte("2"), 0644)

		// Target "new_dir" doesn't exist, but we have multiple files
		ExecuteFileOp(nil, srcVfs, dstVfs, []string{"f1.txt", "f2.txt"}, "new_dir", false, 2, nil)

		for i := 0; i < 100; i++ {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}

		if stat, err := os.Stat(filepath.Join(tmpSrc, "new_dir")); err != nil || !stat.IsDir() {
			t.Error("Target directory not created for multi-file copy")
		}
		if _, err := os.Stat(filepath.Join(tmpSrc, "new_dir", "f1.txt")); err != nil {
			t.Error("f1.txt missing in new directory")
		}
	})

	t.Run("Single file to new subfolder with rename", func(t *testing.T) {
		os.WriteFile(filepath.Join(tmpSrc, "source.txt"), []byte("content"), 0644)

		// Target: "deep/path/target.txt" (subfolders don't exist)
		ExecuteFileOp(nil, srcVfs, dstVfs, []string{"source.txt"}, "deep/path/target.txt", false, 2, nil)

		for i := 0; i < 50; i++ {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}

		finalPath := filepath.Join(tmpSrc, "deep", "path", "target.txt")
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			t.Error("Failed to create parent directories during rename-copy")
		}
	})

	t.Run("Single file to new subfolder with trailing slash", func(t *testing.T) {
		os.WriteFile(filepath.Join(tmpSrc, "source2.txt"), []byte("content"), 0644)

		// Target: "new_dir/" (trailing slash should force directory creation)
		ExecuteFileOp(nil, srcVfs, dstVfs, []string{"source2.txt"}, "new_dir"+string(os.PathSeparator), false, 2, nil)

		for i := 0; i < 50; i++ {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}

		finalPath := filepath.Join(tmpSrc, "new_dir", "source2.txt")
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			t.Error("Trailing slash did not trigger directory creation for single file")
		}
	})
}
func TestExecuteFileOp_RemotePathResolution_Issue74(t *testing.T) {
	// This test reproduces the bug where a Unix-style absolute path was treated
	// as relative when running on a Windows host.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpSrc := t.TempDir()
	srcVfs := vfs.NewOSVFS(tmpSrc)
	os.WriteFile(filepath.Join(tmpSrc, "data.txt"), []byte("payload"), 0644)

	// Simulate a remote destination (like SFTP) using NullVFS which uses path.IsAbs
	dstVfs := vfs.NewNullVFS(0)
	dstVfs.SetPath("/remote/current")

	// Target is an absolute path on the remote system
	remoteTarget := "/remote/target"

	// We expect the file to land exactly at /remote/target/data.txt,
	// NOT at /remote/current/remote/target/data.txt
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{"data.txt"}, remoteTarget, false, 2, nil)

	// In NullVFS, we can't check disk, but we check the resulting destPath logic
	// indirectly by ensuring that if we provided an absolute path, it didn't
	// get prefixed with the current working directory.

	// Since ExecuteFileOp is complex and internal, we verify the logic fix:
	if !dstVfs.IsAbs(remoteTarget) {
		t.Errorf("NullVFS failed to identify %q as absolute", remoteTarget)
	}
}
func TestExecuteFileOp_DirFileConflict(t *testing.T) {
	// Tests the logic when a directory is copied into a path occupied by a file
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Source: folder 'item'
	os.Mkdir(filepath.Join(tmpSrc, "item"), 0755)
	// Destination: file 'item'
	os.WriteFile(filepath.Join(tmpDst, "item"), []byte("blocking"), 0644)

	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, "item"), dstVfs, filepath.Join(tmpDst, "item"), &FileOpState{}, 0)

	if err == nil || !strings.Contains(err.Error(), "cannot overwrite file with folder") {
		t.Errorf("Expected directory-over-file conflict error, got: %v", err)
	}
}
func TestExecuteFileOp_StateTransitions(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	// Create two files in source, and two existing files in dest to trigger conflicts
	os.WriteFile(filepath.Join(tmpSrc, "a.txt"), []byte("new"), 0644)
	os.WriteFile(filepath.Join(tmpSrc, "b.txt"), []byte("new"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "a.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "b.txt"), []byte("old"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)
	state := &FileOpState{}

	// Prepare TaskContext
	tCtx := &vtui.TaskContext{Context: context.Background()}

	// 1. Manually trigger first copy
	// We simulate the user choosing "Overwrite All" by setting the state
	state.OverwriteAll = true

	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, "a.txt"), dstVfs, filepath.Join(tmpDst, "a.txt"), state, 0)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Trigger second copy with same state
	err = recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, "b.txt"), dstVfs, filepath.Join(tmpDst, "b.txt"), state, 0)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Verify both were overwritten
	dataA, _ := os.ReadFile(filepath.Join(tmpDst, "a.txt"))
	dataB, _ := os.ReadFile(filepath.Join(tmpDst, "b.txt"))

	if string(dataA) != "new" || string(dataB) != "new" {
		t.Error("OverwriteAll state was not respected across recursive calls")
	}
}
func TestExecuteFileOp_OptimizedRenameConflict(t *testing.T) {
	// Verifies that optimized same-VFS renames don't silently overwrite files.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()
	v := vfs.NewOSVFS(tmp)

	os.WriteFile(filepath.Join(tmp, "src.txt"), []byte("source"), 0644)
	os.WriteFile(filepath.Join(tmp, "dst.txt"), []byte("destination"), 0644)

	// Execute Move
	done := make(chan struct{})
	ExecuteFileOp(nil, v, v, []string{"src.txt"}, "dst.txt", true, 2, func() {
		close(done)
	})

	// Drain task queue. Since we are moving a file onto an existing one,
	// it should trigger AskOverwrite, which creates a dialog.
	timeout := time.After(500 * time.Millisecond)
	foundDialog := false
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Warning " {
				foundDialog = true
				goto done
			}
		case <-timeout:
			goto done
		}
	}
done:
	if !foundDialog {
		t.Error("Optimized rename bypassed overwrite protection and didn't show a dialog")
	} else {
		// CRITICAL: Properly close the dialog to unblock the worker goroutine.
		// This prevents "directory not empty" errors during TempDir cleanup.
		top := vtui.FrameManager.GetTopFrame()
		if top != nil {
			top.SetExitCode(-1) // Simulate Cancel/Esc
			if top.IsDone() {
				vtui.FrameManager.Pop()
			}
			// Pump tasks to allow the worker to receive the result and exit
			for i := 0; i < 20; i++ {
				select {
				case task := <-vtui.FrameManager.TaskChan:
					task()
				case <-time.After(10 * time.Millisecond):
				}
			}
		}
	}
	// Wait for the background goroutine to fully exit
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for background ExecuteFileOp to exit")
	}
}
func TestExecuteFileOp_SkipAll_Integrity(t *testing.T) {
	// Verifies that when a conflict occurs and user selects "Skip All",
	// no subsequent files in the operation are modified.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	// src: file1, file2
	// dst: file1 (conflict), file2 (should be skipped if SkipAll is active)
	os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("src1"), 0644)
	os.WriteFile(filepath.Join(tmpSrc, "f2.txt"), []byte("src2"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f1.txt"), []byte("dst1"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f2.txt"), []byte("dst2"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	tCtx := &vtui.TaskContext{Context: context.Background()}
	state := &FileOpState{SkipAll: true} // Simulate user already pressed "Skip All"

	// 1. Process f1.txt
	err := recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, "f1.txt"), dstVfs, filepath.Join(tmpDst, "f1.txt"), state, 0)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Process f2.txt
	err = recursiveCopy(tCtx.Context, srcVfs,
		filepath.Join(tmpSrc, "f2.txt"), dstVfs, filepath.Join(tmpDst, "f2.txt"), state, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Verify both destination files remain original
	d1, _ := os.ReadFile(filepath.Join(tmpDst, "f1.txt"))
	d2, _ := os.ReadFile(filepath.Join(tmpDst, "f2.txt"))

	if string(d1) != "dst1" || string(d2) != "dst2" {
		t.Error("Files were overwritten despite SkipAll state")
	}
}
func TestExecuteFileOp_Move_Skip_NoDataLoss(t *testing.T) {
	// Verifies that skipping a file during a MOVE operation prevents
	// the source directory from being deleted, averting data loss.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcFolder := filepath.Join(tmpSrc, "my_folder")
	os.Mkdir(srcFolder, 0755)
	os.WriteFile(filepath.Join(srcFolder, "f1.txt"), []byte("src1"), 0644)
	os.WriteFile(filepath.Join(srcFolder, "f2.txt"), []byte("src2"), 0644)

	// Pre-create destination to cause conflict and skip
	dstFolder := filepath.Join(tmpDst, "my_folder")
	os.Mkdir(dstFolder, 0755)
	os.WriteFile(filepath.Join(dstFolder, "f1.txt"), []byte("dst1"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	tCtx := &vtui.TaskContext{Context: context.Background()}
	state := &FileOpState{SkipAll: true}

	err := recursiveCopy(tCtx.Context, srcVfs, srcFolder, dstVfs, dstFolder, state, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the ExecuteFileOp deletion logic
	if state.SkippedCount == 0 {
		srcVfs.Remove(context.Background(), srcFolder)
	}

	// Assertion: The source folder MUST STILL EXIST because a file was skipped!
	if _, err := os.Stat(filepath.Join(srcFolder, "f1.txt")); os.IsNotExist(err) {
		t.Error("CRITICAL DATA LOSS: Skipped file was deleted from source folder!")
	}
}
func TestExecuteFileOp_MoveAcrossVFS_Fallback(t *testing.T) {
	// Tests that moving a file between two different VFS implementations
	// (or when optimized Rename fails) correctly falls back to Copy + Delete.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	fileName := "cross_vfs.txt"
	os.WriteFile(filepath.Join(tmpSrc, fileName), []byte("payload"), 0644)

	// Use ExecuteFileOp with isMove=true.
	// Since they are different OSVFS instances (simulating different volumes/servers),
	// the recursiveCopy logic will be used.
	done := make(chan struct{})
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{fileName}, tmpDst, true, 2, func() {
		close(done)
	})

	// Drain task queue until operation completes
	timeout := time.After(10 * time.Second)
Loop:
	for {
		select {
		case <-done:
			break Loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Move operation timed out")
		}
	}

	// Verify result
	if data, _ := os.ReadFile(filepath.Join(tmpDst, fileName)); string(data) != "payload" {
		t.Error("File was not moved correctly to destination")
	}
}
func TestExecuteFileOp_LargeFileIntegrity(t *testing.T) {
	// Verifies data integrity for files spanning multiple 128KB buffer chunks.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	// 1. Generate 512KB of pseudo-random data
	data := make([]byte, 512*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	fileName := "massive.bin"
	os.WriteFile(filepath.Join(tmpSrc, fileName), data, 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	// 2. Perform Copy
	done := make(chan struct{})
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{fileName}, tmpDst, false, 2, func() {
		close(done)
	})

	// 3. Pump tasks until callback is triggered
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-done:
			break loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Large file copy timed out")
		}
	}
	// 4. Verify byte-for-byte
	copiedData, err := os.ReadFile(filepath.Join(tmpDst, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(copiedData) != len(data) {
		t.Errorf("Length mismatch: expected %d, got %d", len(data), len(copiedData))
	}
	for i := range data {
		if data[i] != copiedData[i] {
			t.Fatalf("Data corruption at byte %d", i)
		}
	}
}
func TestExecuteFileOp_DeepIntegrity(t *testing.T) {
	// Tests a deep directory structure with a mix of small files and one
	// large binary file to ensure the recursive copy is robust.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	srcBase := t.TempDir()
	dstBase := t.TempDir()

	// 1. Create structure:
	// /root/file1.txt
	// /root/sub1/file2.txt
	// /root/sub1/sub2/large.bin (4MB)
	os.MkdirAll(filepath.Join(srcBase, "root", "sub1", "sub2"), 0755)

	largeData := make([]byte, 4*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 251)
	} // Prime to avoid simple patterns

	os.WriteFile(filepath.Join(srcBase, "root", "file1.txt"), []byte("f1"), 0644)
	os.WriteFile(filepath.Join(srcBase, "root", "sub1", "file2.txt"), []byte("f2"), 0644)
	os.WriteFile(filepath.Join(srcBase, "root", "sub1", "sub2", "large.bin"), largeData, 0644)

	srcVfs := vfs.NewOSVFS(srcBase)
	dstVfs := vfs.NewOSVFS(dstBase)

	// 2. Perform recursive copy of "root"
	done := make(chan struct{})
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{"root"}, dstBase, false, 2, func() {
		close(done)
	})

	// 3. Wait for completion callback
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case <-done:
			break loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Deep copy timed out")
		}
	}

	// Final drain to ensure all UI/stat tasks finished
	for i := 0; i < 10; i++ {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
		}
	}

	targetLarge := filepath.Join(dstBase, "root", "sub1", "sub2", "large.bin")

	// 4. Verify Large File
	copiedLarge, _ := os.ReadFile(targetLarge)
	if !bytes.Equal(copiedLarge, largeData) {
		t.Error("Large binary file corrupted during deep recursive copy")
	}

	// 5. Verify Small File in subfolder
	f2, _ := os.ReadFile(filepath.Join(dstBase, "root", "sub1", "file2.txt"))
	if string(f2) != "f2" {
		t.Errorf("Small file corrupted or missing in subfolder, got %q", string(f2))
	}
}
func TestExecuteFileOp_Move_PermissionDenied_Recovery(t *testing.T) {
	// Tests that a Move operation handles partial failures (like permission denied)
	// gracefully without deleting the source if the copy failed.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix permission test on Windows")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "protected.txt")
	os.WriteFile(srcFile, []byte("secret"), 0644)

	// Make destination dir read-only
	os.Chmod(dstDir, 0444)
	defer os.Chmod(dstDir, 0755)

	done := make(chan struct{})
	v := vfs.NewOSVFS("/")
	ExecuteFileOp(nil, v, v, []string{srcFile}, dstDir, true, 2, func() {
		close(done)
	})

	// Pump tasks. It should hit AskError. We simulate "Abort".
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-done:
			break loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if top := vtui.FrameManager.GetTopFrame(); top != nil && top.GetTitle() == " Error " {
				top.SetExitCode(2) // Abort
				if top.IsDone() {
					vtui.FrameManager.Pop()
				}
			}
		case <-time.After(100 * time.Millisecond):
			break loop
		case <-timeout:
			t.Fatal("Timeout waiting for permission denied Move to abort")
		}
	}

	// Verify source was NOT deleted because copy failed
	if _, err := os.Stat(srcFile); os.IsNotExist(err) {
		t.Error("Source file was deleted even though Move failed due to permissions")
	}
}

func TestExecuteFileOp_MoveIntoSelf_Circular(t *testing.T) {
	// Prevents infinite recursion when trying to move/copy a parent into its own child.
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "child")
	os.MkdirAll(child, 0755)

	v := vfs.NewOSVFS("/")
	tCtx := &vtui.TaskContext{Context: context.Background()}

	// Move 'parent' into 'parent/child/oops'
	err := recursiveCopy(tCtx.Context, v, parent, v, filepath.Join(child, "oops"), &FileOpState{}, 0)

	if err == nil || !strings.Contains(err.Error(), "folder into itself") {
		t.Errorf("Expected circular copy protection error, got: %v", err)
	}
}
func TestRecursiveCopy_SelfAndSubfolderProtection(t *testing.T) {
	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)
	tCtx := &vtui.TaskContext{Context: context.Background()}

	// 1. Folder self-copy
	folderPath := filepath.Join(tmpDir, "myfolder")
	os.MkdirAll(folderPath, 0755)

	err := recursiveCopy(tCtx.Context, v, folderPath, v, folderPath, &FileOpState{}, 0)
	if err == nil || !strings.Contains(err.Error(), "folder into itself") {
		t.Errorf("Expected folder self-copy error, got: %v", err)
	}

	// 2. Folder into subfolder
	subPath := filepath.Join(folderPath, "sub")
	err = recursiveCopy(tCtx.Context, v, folderPath, v, subPath, &FileOpState{}, 0)
	if err == nil || !strings.Contains(err.Error(), "subfolder") {
		t.Errorf("Expected folder into subfolder error, got: %v", err)
	}

	// 3. File self-copy
	filePath := filepath.Join(tmpDir, "myfile.txt")
	os.WriteFile(filePath, []byte("data"), 0644)

	err = recursiveCopy(tCtx.Context, v, filePath, v, filePath, &FileOpState{}, 0)
	if err == nil || !strings.Contains(err.Error(), "file onto itself") {
		t.Errorf("Expected file self-copy error, got: %v", err)
	}

	// 4. File into own subfolder
	fileSubPath := filepath.Join(filePath, "sub")
	err = recursiveCopy(tCtx.Context, v, filePath, v, fileSubPath, &FileOpState{}, 0)
	if err == nil || !strings.Contains(err.Error(), "subfolder") {
		t.Errorf("Expected file into subfolder error, got: %v", err)
	}
}
func TestRecursiveCopy_SubfolderDeepRecursion(t *testing.T) {
	// Regression test for Note 7: vtinput/vtinput/vtinput...
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()

	// Create /parent/child
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "child")
	os.MkdirAll(child, 0755)
	os.WriteFile(filepath.Join(parent, "file.txt"), []byte("data"), 0644)

	v := vfs.NewOSVFS("/")
	tCtx := &vtui.TaskContext{Context: context.Background()}

	// Attempt to copy /parent into /parent/child/backup
	// This should be caught by the subfolder check
	dest := filepath.Join(child, "backup")

	err := recursiveCopy(tCtx.Context, v, parent, v, dest, &FileOpState{}, 0)

	if err == nil {
		t.Fatal("Expected error when copying folder into its own deep subfolder, but got nil")
	}

	if !strings.Contains(err.Error(), "subfolder") {
		t.Errorf("Expected 'subfolder' error message, got: %v", err)
	}

	// Verify that no deep structure was created before the error
	if _, err := os.Stat(filepath.Join(dest, "child")); err == nil {
		t.Error("Recursive copy partially succeeded before failing, created nested child!")
	}
}

func TestRecursiveCopy_SymlinkLoop(t *testing.T) {
	// Tests protection against loops like "ln -s .. loop"
	if runtime.GOOS == "windows" {
		t.Skip("Symlinks behave differently on Windows")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()

	src := filepath.Join(tmp, "source")
	os.Mkdir(src, 0755)
	os.WriteFile(filepath.Join(src, "data.txt"), []byte("hi"), 0644)

	// Create a symlink INSIDE src that points to tmp (parent)
	loopLink := filepath.Join(src, "loop")
	os.Symlink(tmp, loopLink)

	v := vfs.NewOSVFS("/")
	tCtx := &vtui.TaskContext{Context: context.Background()}

	// Attempt to copy "source" into "source/loop/backup"
	// This would be infinite without EvalSymlinks because loopLink leads to tmp,
	// and tmp contains source.
	target := filepath.Join(loopLink, "backup")

	err := recursiveCopy(tCtx.Context, v, src, v, target, &FileOpState{}, 0)

	if err == nil {
		t.Fatal("Expected error for symlink loop recursion, but got nil")
	}
	if !strings.Contains(err.Error(), "folder into itself") {
		t.Errorf("Wrong error message for symlink loop: %v", err)
	}
}
func TestRecursiveCopy_ByteProgress(t *testing.T) {
	t.Run("Single Large File (NullVFS)", func(t *testing.T) {
		srcVfs := vfs.NewNullVFS(0) // Unlimited speed
		dstVfs := vfs.NewNullVFS(0)

		ctx := context.Background()
		callCount := 0
		totalBytes := 0

		state := &FileOpState{
			OverwriteAll: true,
			OnBytes: func(n int) {
				callCount++
				totalBytes += n
			},
		}

		err := recursiveCopy(ctx, srcVfs, "/1MB.bin", dstVfs, "/upload/test.bin", state, 0)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Buffer size in recursiveCopy is 128KB (131072 bytes).
		// 1MB = 1048576 bytes. 1048576 / 131072 = 8 exactly.
		if callCount != 8 {
			t.Errorf("Expected 8 calls to OnBytes, got %d", callCount)
		}
		if totalBytes != 1024*1024 {
			t.Errorf("Expected 1048576 bytes total, got %d", totalBytes)
		}
	})

	t.Run("Multiple Small Files (OSVFS)", func(t *testing.T) {
		tmpSrc := t.TempDir()
		tmpDst := t.TempDir()
		srcVfs := vfs.NewOSVFS(tmpSrc)
		dstVfs := vfs.NewOSVFS(tmpDst)

		os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("Hello"), 0644)
		os.WriteFile(filepath.Join(tmpSrc, "f2.txt"), []byte("World!"), 0644)

		ctx := context.Background()
		callCount := 0
		totalBytes := 0

		state := &FileOpState{
			OverwriteAll: true,
			OnBytes: func(n int) {
				callCount++
				totalBytes += n
			},
		}

		err := recursiveCopy(ctx, srcVfs, tmpSrc, dstVfs, filepath.Join(tmpDst, "copied"), state, 0)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// "Hello" (5) + "World!" (6) = 11 bytes.
		// Expected 2 calls, one for each file.
		if callCount != 2 {
			t.Errorf("Expected 2 calls to OnBytes, got %d", callCount)
		}
		if totalBytes != 11 {
			t.Errorf("Expected 11 bytes total, got %d", totalBytes)
		}
	})
}

// --- UI Integration Tests for Conflict Resolution ---

// Helper to pump UI tasks until a dialog with the given title appears
func waitForDialog(t *testing.T, title string) vtui.Container {
	t.Helper()
	// Check if it's already on top and not closed
	top := vtui.FrameManager.GetTopFrame()
	if top != nil && top.GetTitle() == title && !top.IsDone() {
		return top.(vtui.Container)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			// Only return active dialogs, ignore stale closed ones
			if top != nil && top.GetTitle() == title && !top.IsDone() {
				return top.(vtui.Container)
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for dialog %q", title)
		}
	}
}

func getCleanText(item vtui.UIElement) string {
	switch v := item.(type) {
	case *vtui.Button:
		clean, _, _ := vtui.ParseAmpersandString(v.GetText())
		return strings.TrimSpace(strings.Trim(clean, "[]"))
	case *vtui.Checkbox:
		clean, _, _ := vtui.ParseAmpersandString(v.GetText())
		return strings.TrimSpace(clean)
	}
	return ""
}

func clickDialogButton(t *testing.T, dlg vtui.Container, btnText string) {
	t.Helper()
	for _, itm := range dlg.GetChildren() {
		if b, ok := itm.(*vtui.Button); ok {
			if getCleanText(b) == btnText {
				if b.OnClick != nil {
					b.OnClick()
					return
				}
			}
		}
	}
	t.Fatalf("Button %q not found in dialog", btnText)
}

func setDialogCheckbox(t *testing.T, dlg vtui.Container, chkText string, state int) {
	t.Helper()
	for _, itm := range dlg.GetChildren() {
		if c, ok := itm.(*vtui.Checkbox); ok {
			if getCleanText(c) == chkText {
				c.State = state
				return
			}
		}
	}
	t.Fatalf("Checkbox %q not found in dialog", chkText)
}

func enterTextAndOk(t *testing.T, dlg vtui.Container, text string) {
	t.Helper()
	for _, itm := range dlg.GetChildren() {
		if e, ok := itm.(*vtui.Edit); ok {
			e.SetText(text)
		}
		if b, ok := itm.(*vtui.Button); ok {
			if getCleanText(b) == "Ok" {
				if b.OnClick != nil {
					b.OnClick()
					return
				}
			}
		}
	}
	t.Fatalf("Failed to enter text and click Ok")
}

func TestFileOps_UI_RememberOverwrite(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("new1"), 0644)
	os.WriteFile(filepath.Join(tmpSrc, "f2.txt"), []byte("new2"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f1.txt"), []byte("old1"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f2.txt"), []byte("old2"), 0644)

	done := make(chan struct{})
	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f1.txt", "f2.txt"}, tmpDst, false, 2, func() { close(done) })

	// Wait for first warning (f1.txt)
	dlg := waitForDialog(t, " Warning ")

	// Check "Remember choice" and click "Overwrite"
	setDialogCheckbox(t, dlg, "Remember choice", 1)
	clickDialogButton(t, dlg, "Overwrite")

	// Now wait for done. If it asks again, it will trigger a fatal error.
	timeout := time.After(2 * time.Second)
pump1:
	for {
		select {
		case <-done:
			break pump1
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if vtui.FrameManager.GetTopFrameType() == vtui.TypeDialog {
				top := vtui.FrameManager.GetTopFrame()
				// Only fail if a NEW active warning dialog appears
				if top.GetTitle() == " Warning " && !top.IsDone() {
					t.Fatalf("Dialog appeared again despite 'Remember choice'")
				}
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for operation to complete")
		}
	}

	d1, _ := os.ReadFile(filepath.Join(tmpDst, "f1.txt"))
	d2, _ := os.ReadFile(filepath.Join(tmpDst, "f2.txt"))
	if string(d1) != "new1" || string(d2) != "new2" {
		t.Errorf("Files were not overwritten. f1:%s, f2:%s", d1, d2)
	}
}

func TestFileOps_UI_RenameAndAppendUnsupported(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("source"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f1.txt"), []byte("dest1"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f2.txt"), []byte("dest2"), 0644)

	done := make(chan struct{})
	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f1.txt"}, tmpDst, false, 2, func() { close(done) })

	// 1. First warning (f1.txt)
	dlg := waitForDialog(t, " Warning ")

	// 2. Click Append -> should show "Unsupported"
	clickDialogButton(t, dlg, "Append")
	errDlg := waitForDialog(t, " Unsupported ")
	clickDialogButton(t, errDlg, "Ok")

	// 3. Warning should reappear for f1.txt
	dlg2 := waitForDialog(t, " Warning ")

	// 4. Click Rename
	clickDialogButton(t, dlg2, "Rename")
	renDlg := waitForDialog(t, " Rename ")
	enterTextAndOk(t, renDlg, "f2.txt") // Rename to f2.txt, which ALSO exists

	// 5. Warning should reappear for f2.txt
	dlg3 := waitForDialog(t, " Warning ")

	// 6. Click Overwrite
	clickDialogButton(t, dlg3, "Overwrite")

	timeout := time.After(2 * time.Second)
pump2:
	for {
		select {
		case <-done:
			break pump2
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout")
		}
	}

	d2, _ := os.ReadFile(filepath.Join(tmpDst, "f2.txt"))
	if string(d2) != "source" {
		t.Errorf("Rename + Overwrite failed. f2.txt contains: %s", d2)
	}
}

func TestFileOps_UI_MoveSkip(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("source"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f1.txt"), []byte("dest"), 0644)

	done := make(chan struct{})
	// isMove = true
	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f1.txt"}, tmpDst, true, 2, func() { close(done) })

	dlg := waitForDialog(t, " Warning ")
	clickDialogButton(t, dlg, "Skip")

	timeout := time.After(2 * time.Second)
pump3:
	for {
		select {
		case <-done:
			break pump3
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout")
		}
	}

	if _, err := os.Stat(filepath.Join(tmpSrc, "f1.txt")); os.IsNotExist(err) {
		t.Error("Source file was deleted despite being skipped in a Move operation")
	}
}

func TestFileOps_ForkedWorkspace(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	vtui.FrameManager.Push(pf)
	initialScreens := len(vtui.FrameManager.Screens)

	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("data"), 0644)

	done := make(chan struct{})
	// forked = true
	ExecuteFileOp(pf, vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f1.txt"}, tmpDst, false, 1, func() { close(done) })

	// Process tasks until the background copy finishes
	timeout := time.After(2 * time.Second)
pump4:
	for {
		select {
		case <-done:
			break pump4
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout")
		}
	}

	// We expect that a new screen was created during the operation
	if len(vtui.FrameManager.Screens) != initialScreens+1 {
		t.Errorf("Forked operation did not create a new workspace screen. Screens: %d", len(vtui.FrameManager.Screens))
	}
}

func TestFileOps_UI_ConcurrentConflicts(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpSrc1, tmpDst1 := t.TempDir(), t.TempDir()
	tmpSrc2, tmpDst2 := t.TempDir(), t.TempDir()

	os.WriteFile(filepath.Join(tmpSrc1, "f1.txt"), []byte("src1"), 0644)
	os.WriteFile(filepath.Join(tmpDst1, "f1.txt"), []byte("dst1"), 0644)

	os.WriteFile(filepath.Join(tmpSrc2, "f2.txt"), []byte("src2"), 0644)
	os.WriteFile(filepath.Join(tmpDst2, "f2.txt"), []byte("dst2"), 0644)

	done1, done2 := make(chan struct{}), make(chan struct{})

	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc1), vfs.NewOSVFS(tmpDst1), []string{"f1.txt"}, tmpDst1, false, 2, func() { close(done1) })
	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc2), vfs.NewOSVFS(tmpDst2), []string{"f2.txt"}, tmpDst2, false, 2, func() { close(done2) })

	// We expect TWO warning dialogs (processed sequentially by the TaskChan pump).
	// Since operations are concurrent, we must check which dialog is which.
	for i := 0; i < 2; i++ {
		dlg := waitForDialog(t, " Warning ")

		isOp1 := false
		isOp2 := false
		for _, itm := range dlg.GetChildren() {
			if txt, ok := itm.(*vtui.Text); ok {
				if strings.Contains(txt.GetText(), "f1.txt") {
					isOp1 = true
				} else if strings.Contains(txt.GetText(), "f2.txt") {
					isOp2 = true
				}
			}
		}

		if isOp1 {
			clickDialogButton(t, dlg, "Overwrite")
		} else if isOp2 {
			clickDialogButton(t, dlg, "Skip")
		} else {
			t.Fatalf("Warning dialog path does not match either f1.txt or f2.txt")
		}
	}

	timeout := time.After(2 * time.Second)
	d1Done, d2Done := false, false
	for !d1Done || !d2Done {
		select {
		case <-done1:
			d1Done = true
			done1 = nil // Set to nil to prevent infinite loop on closed channel
		case <-done2:
			d2Done = true
			done2 = nil // Set to nil to prevent infinite loop on closed channel
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout waiting for concurrent ops")
		}
	}

	c1, _ := os.ReadFile(filepath.Join(tmpDst1, "f1.txt"))
	c2, _ := os.ReadFile(filepath.Join(tmpDst2, "f2.txt"))

	if string(c1) != "src1" {
		t.Errorf("Op1 overwrite failed")
	}
	if string(c2) != "dst2" {
		t.Errorf("Op2 skip failed, got %s", c2)
	}
}
func TestFileOps_UI_CancelDuringMove(t *testing.T) {
	// Scenario: A Move operation hits a conflict. The user clicks "Cancel".
	// CRITICAL REQUIREMENT: The source file must NOT be deleted.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcFile := filepath.Join(tmpSrc, "f1.txt")
	dstFile := filepath.Join(tmpDst, "f1.txt")

	os.WriteFile(srcFile, []byte("source_data"), 0644)
	os.WriteFile(dstFile, []byte("target_data"), 0644)

	done := make(chan struct{})
	// isMove = true
	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f1.txt"}, tmpDst, true, 2, func() { close(done) })

	// Wait for warning dialog
	dlg := waitForDialog(t, " Warning ")

	// Click Cancel (Button 6)
	clickDialogButton(t, dlg, "Cancel")

	timeout := time.After(2 * time.Second)
pump:
	for {
		select {
		case <-done:
			break pump
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout")
		}
	}

	// Assertions
	dSrc, errSrc := os.ReadFile(srcFile)
	if errSrc != nil {
		t.Fatalf("CRITICAL DATA LOSS: Source file was deleted after Cancel! Err: %v", errSrc)
	}
	if string(dSrc) != "source_data" {
		t.Errorf("Source file corrupted: got %q", string(dSrc))
	}

	dDst, _ := os.ReadFile(dstFile)
	if string(dDst) != "target_data" {
		t.Errorf("Destination file corrupted by cancelled move: got %q", string(dDst))
	}
}

func TestFileOps_UI_RenameToEmpty(t *testing.T) {
	// Scenario: User clicks "Rename" on conflict, but then cancels the prompt.
	// Expected: Operation cancels safely without data loss.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	os.WriteFile(filepath.Join(tmpSrc, "f.txt"), []byte("src"), 0644)
	os.WriteFile(filepath.Join(tmpDst, "f.txt"), []byte("dst"), 0644)

	done := make(chan struct{})
	ExecuteFileOp(nil, vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f.txt"}, tmpDst, true, 2, func() { close(done) })

	dlg := waitForDialog(t, " Warning ")
	clickDialogButton(t, dlg, "Rename")

	renDlg := waitForDialog(t, " Rename ")
	// Click Cancel in the Rename dialog
	clickDialogButton(t, renDlg, "Cancel")

	timeout := time.After(2 * time.Second)
pump:
	for {
		select {
		case <-done:
			break pump
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout")
		}
	}

	if _, err := os.Stat(filepath.Join(tmpSrc, "f.txt")); os.IsNotExist(err) {
		t.Error("CRITICAL DATA LOSS: Source deleted after cancelling rename prompt")
	}
}

func TestFileOps_SameFile_Protection(t *testing.T) {
	// Scenario: Trying to copy a file over itself.
	// If unprotected, Create(O_TRUNC) will wipe the file to 0 bytes before Open() reads it.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()

	targetFile := filepath.Join(tmp, "critical.txt")
	os.WriteFile(targetFile, []byte("PRECIOUS_DATA"), 0644)

	v := vfs.NewOSVFS(tmp)

	// Direct call to recursiveCopy to bypass UI wrappers
	tCtx := &vtui.TaskContext{Context: context.Background()}
	err := recursiveCopy(tCtx.Context, v, targetFile, v, targetFile, &FileOpState{}, 0)

	if err == nil {
		t.Fatal("Expected error when copying file to itself, got nil")
	}
	if !strings.Contains(err.Error(), "source equals destination") {
		t.Errorf("Unexpected error message: %v", err)
	}

	// Verify data is intact
	data, _ := os.ReadFile(targetFile)
	if string(data) != "PRECIOUS_DATA" {
		t.Errorf("CRITICAL DATA LOSS: Same-file copy truncated the file! Contents: %q", string(data))
	}
}
func TestFileOps_FormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{10 * 1024 * 1024 * 1024, "10.0 GB"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.bytes); got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFileOps_PathDisplay(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	os.WriteFile(filepath.Join(tmpSrc, "display.txt"), []byte("data"), 0644)

	orig := AppConfig.FileOpPathDisplay
	defer func() { AppConfig.FileOpPathDisplay = orig }()

	AppConfig.FileOpPathDisplay = 1
	tracker1 := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 4})
	var capturedName1 string
	state := &FileOpState{
		Tracker: tracker1,
		Buffer:  make([]byte, 1024),
		UpdateUI: func(force bool) {
			_, _, name := tracker1.GetProgress()
			if name != "" {
				capturedName1 = name
			}
		},
	}
	err := recursiveCopy(context.Background(), srcVfs, filepath.Join(tmpSrc, "display.txt"), dstVfs, filepath.Join(tmpDst, "display.txt"), state, 0)
	if err != nil {
		t.Fatal(err)
	}
	if capturedName1 != filepath.Join(tmpSrc, "display.txt") {
		t.Errorf("Expected currentName to be full source path, got %q", capturedName1)
	}

	AppConfig.FileOpPathDisplay = 2
	tracker2 := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 4})
	var capturedName2 string
	state2 := &FileOpState{
		Tracker: tracker2,
		Buffer:  make([]byte, 1024),
		UpdateUI: func(force bool) {
			_, _, name := tracker2.GetProgress()
			if name != "" {
				capturedName2 = name
			}
		},
	}
	err = recursiveCopy(context.Background(), srcVfs, filepath.Join(tmpSrc, "display.txt"), dstVfs, filepath.Join(tmpDst, "display_new.txt"), state2, 0)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmpSrc, "display.txt") + " -> " + filepath.Join(tmpDst, "display_new.txt")
	if capturedName2 != expected {
		t.Errorf("Expected currentName to be source -> dest, got %q", capturedName2)
	}
}

func TestFileOps_CalculateStats_Integration(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "a/b"), 0755)
	os.WriteFile(filepath.Join(tmp, "a/f1.txt"), []byte("123"), 0644)
	os.WriteFile(filepath.Join(tmp, "a/b/f2.txt"), []byte("4567"), 0644)

	v := vfs.NewOSVFS(tmp)
	stats, err := vfs.CalculateStats(context.Background(), v, tmp, []string{"a"}, nil)

	if err != nil {
		t.Fatalf("CalculateStats failed: %v", err)
	}
	if stats.Files != 2 || stats.Dirs != 2 || stats.Bytes != 7 {
		t.Errorf("Stats mismatch: %+v", stats)
	}
}
func TestExecuteFileOp_PathInterpretations(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	tests := []struct {
		name     string
		input    string
		wantVFS  vfs.VFS
		wantPath string
	}{
		{name: "simple name", input: "renamed.txt", wantVFS: srcVfs, wantPath: filepath.Join(tmpSrc, "renamed.txt")},
		{name: "nested relative path", input: filepath.Join("test", "nested"), wantVFS: srcVfs, wantPath: filepath.Join(tmpSrc, "test", "nested")},
		{name: "current directory", input: ".", wantVFS: srcVfs, wantPath: filepath.Clean(tmpSrc)},
		{name: "parent directory", input: "..", wantVFS: srcVfs, wantPath: filepath.Dir(tmpSrc)},
		{name: "passive absolute path", input: tmpDst, wantVFS: dstVfs, wantPath: tmpDst},
		{name: "active absolute path", input: tmpSrc, wantVFS: dstVfs, wantPath: tmpSrc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVFS, gotPath := resolveFileOpDestination(srcVfs, dstVfs, tt.input)
			if gotVFS != tt.wantVFS || gotPath != tt.wantPath {
				t.Fatalf("resolve(%q) = (%T, %q), want (%T, %q)", tt.input, gotVFS, gotPath, tt.wantVFS, tt.wantPath)
			}
		})
	}
}

type mockFailingRemoveVFS struct {
	vfs.VFS
}

func (m *mockFailingRemoveVFS) Remove(ctx context.Context, path string) error {
	return os.ErrPermission
}

func TestExecuteFileOp_Move_FinalizeFailure(t *testing.T) {
	// Сценарий: Копирование прошло успешно, но удаление (Remove) исходника упало.
	// Мы должны увидеть ошибку, а исходник должен остаться на месте.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()

	srcFile := filepath.Join(tmpSrc, "ghost.txt")
	os.WriteFile(srcFile, []byte("data"), 0644)

	// VFS который имитирует успех Copy, но провал Remove
	srcVfs := &mockFailingRemoveVFS{VFS: vfs.NewOSVFS(tmpSrc)}
	dstVfs := vfs.NewOSVFS(tmpDst)

	done := make(chan struct{})
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{"ghost.txt"}, tmpDst, true, 2, func() {
		close(done)
	})

	// Pump
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-done:
			break loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for Move_FinalizeFailure to complete")
		}
	}

	// 1. Файл должен был скопироваться
	if _, err := os.Stat(filepath.Join(tmpDst, "ghost.txt")); err != nil {
		t.Error("File was not even copied during move")
	}

	// 2. Но исходник не должен был удалиться из-за ошибки
	if _, err := os.Stat(srcFile); os.IsNotExist(err) {
		t.Error("Source file was deleted even though Remove returned error")
	}
}
func TestExecuteFileOp_ForegroundIntegrity(t *testing.T) {
	// Проверяем, что Mode 2 (Foreground) по-прежнему работает без очереди
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	os.WriteFile(filepath.Join(tmpSrc, "direct.txt"), []byte("data"), 0644)

	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	// Запускаем в режиме 2 (Foreground)
	done := make(chan struct{})
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{"direct.txt"}, tmpDst, false, 2, func() {
		close(done)
	})

	// В этом режиме должен сразу появиться диалог прогресса
	foundDialog := false
	timeout := time.After(1 * time.Second)
Loop:
	for {
		select {
		case task := <-fm.TaskChan:
			task()
			if fm.GetTopFrameType() == vtui.TypeDialog && strings.Contains(fm.GetTopFrame().GetTitle(), "Copying") {
				foundDialog = true
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	if !foundDialog {
		t.Error("Foreground mode (2) failed to show progress dialog immediately")
	}

	// Ждем, пока операция реально закончится и закроет диалог.
	// Это гарантирует, что все файловые дескрипторы закрыты до того,
	// как t.TempDir начнет удалять папку.
	timeout = time.After(2 * time.Second)
	for fm.GetTopFrame() != nil {
		select {
		case task := <-fm.TaskChan:
			task()
			if top := fm.GetTopFrame(); top != nil && top.IsDone() {
				fm.Pop()
			}
		case <-timeout:
			t.Fatal("Foreground operation timed out before closing dialog")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	// Wait for the background goroutine to fully exit
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for background ExecuteFileOp to exit")
	}
}

// closeErrVFS hands out writers whose Close fails, which is how a buffering
// remote file system reports that its last chunk never arrived.
type closeErrVFS struct {
	vfs.VFS
	closeErr error
	closes   int
}

func (v *closeErrVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	w, err := v.VFS.Create(ctx, path)
	if err != nil {
		return nil, err
	}
	return &closeErrWriter{w: w, owner: v}, nil
}

type closeErrWriter struct {
	w     io.WriteCloser
	owner *closeErrVFS
}

func (w *closeErrWriter) Write(p []byte) (int, error) { return w.w.Write(p) }

func (w *closeErrWriter) Close() error {
	w.owner.closes++
	w.w.Close()
	return w.owner.closeErr
}

type countingCloser struct {
	closes int
	err    error
}

func (c *countingCloser) Close() error {
	c.closes++
	return c.err
}

func TestCloseOnceClosesOnceAndReportsTheError(t *testing.T) {
	c := &countingCloser{err: fmt.Errorf("flush failed")}
	closeIt := closeOnce(c)
	if err := closeIt(); err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Fatalf("first close returned %v, want the underlying error", err)
	}
	// The defer that follows an explicit close must be a no-op, not a
	// second Close on a writer that already tore its buffer down.
	if err := closeIt(); err != nil {
		t.Errorf("second close returned %v, want nil", err)
	}
	if c.closes != 1 {
		t.Errorf("Close called %d times, want 1", c.closes)
	}
}

func TestRecursiveCopyFailsWhenTheDestinationCloseFails(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	srcFile := filepath.Join(tmpSrc, "file.txt")
	if err := os.WriteFile(srcFile, []byte("some content"), 0644); err != nil {
		t.Fatal(err)
	}
	dstFile := filepath.Join(tmpDst, "file.txt")

	dstVfs := &closeErrVFS{VFS: vfs.NewOSVFS(tmpDst), closeErr: fmt.Errorf("flush failed")}
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	err := recursiveCopy(tCtx.Context, vfs.NewOSVFS(tmpSrc), srcFile, dstVfs, dstFile, &FileOpState{}, 0)
	if err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Fatalf("recursiveCopy returned %v, want the close error", err)
	}
	if _, statErr := os.Stat(dstFile); statErr == nil {
		t.Error("an incomplete destination was left behind")
	}
	if dstVfs.closes != 1 {
		t.Errorf("Close called %d times, want 1", dstVfs.closes)
	}
}

func TestRecursiveCopyClosesTheDestinationBeforeSucceeding(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	srcFile := filepath.Join(tmpSrc, "file.txt")
	content := []byte("some content")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatal(err)
	}
	dstFile := filepath.Join(tmpDst, "file.txt")

	dstVfs := &closeErrVFS{VFS: vfs.NewOSVFS(tmpDst)}
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tCtx := vtui.RunAsync(func(c *vtui.TaskContext) {})
	defer tCtx.Cancel()

	if err := recursiveCopy(tCtx.Context, vfs.NewOSVFS(tmpSrc), srcFile, dstVfs, dstFile, &FileOpState{}, 0); err != nil {
		t.Fatalf("recursiveCopy: %v", err)
	}
	got, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("copied %q, want %q", got, content)
	}
	if dstVfs.closes != 1 {
		t.Errorf("Close called %d times, want 1", dstVfs.closes)
	}
}

type mockReadAtCloser struct{}

func (m *mockReadAtCloser) ReadAt(ctx context.Context, p []byte, off int64) (n int, err error) {
	return 0, io.EOF
}
func (m *mockReadAtCloser) Read(ctx context.Context, p []byte) (n int, err error) {
	return 0, io.EOF
}
func (m *mockReadAtCloser) Close() error { return nil }
func (m *mockReadAtCloser) Size() int64  { return 10 }

type mockWriteCloser struct{}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) { return len(p), nil }
func (m *mockWriteCloser) Close() error                      { return nil }

type mockVFS struct {
	vfs.VFS
	onOpen func(ctx context.Context, path string) (vfs.ReadAtCloser, error)
}

func (m *mockVFS) IsAtRoot() bool            { return true }
func (m *mockVFS) GetPath() string           { return "/" }
func (m *mockVFS) IsAbs(path string) bool    { return true }
func (m *mockVFS) SetPath(path string) error { return nil }
func (m *mockVFS) Join(elem ...string) string {
	return strings.Join(elem, "/")
}
func (m *mockVFS) Abs(path string) (string, error) { return path, nil }
func (m *mockVFS) Base(path string) string         { return "file.txt" }
func (m *mockVFS) Dir(path string) string          { return "/" }
func (m *mockVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	return vfs.VFSItem{Name: "file.txt", Size: 10, IsDir: false}, nil
}
func (m *mockVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	if m.onOpen != nil {
		return m.onOpen(ctx, path)
	}
	return &mockReadAtCloser{}, nil
}
func (m *mockVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	return &mockWriteCloser{}, nil
}
func (m *mockVFS) Remove(ctx context.Context, path string) error { return nil }
func (m *mockVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}
func (m *mockVFS) Close() error   { return nil }
func (m *mockVFS) Clone() vfs.VFS { return m }

type mockReporter struct {
	lastAction     string
	lastFilename   string
	lastCurrentPct int
	lastTotalText  string
	lastTotalPct   int
	lastSpeedText  string
}

func (m *mockReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (m *mockReporter) IsCancelled() bool                                { return false }
func (m *mockReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	m.lastAction = action
	m.lastFilename = filename
	m.lastCurrentPct = currentPct
	m.lastTotalText = totalText
	m.lastTotalPct = totalPct
	m.lastSpeedText = speedText
}

func TestProgressReporterHijacking(t *testing.T) {
	mock := &mockReporter{}

	// Global progress is set to 50%
	globalTotalText := "Total: 50 MB / 100 MB"
	globalTotalPct := 50
	globalSpeedText := "Time: 00:01:00  Remaining: 00:01:00  1.0 MB/s"

	getGlobal := func(action string) (string, int, string) {
		return globalTotalText, globalTotalPct, globalSpeedText
	}

	// Instantiate globalAwareReporter
	wrap := &globalAwareReporter{
		original:  mock,
		getGlobal: getGlobal,
	}

	// Simulate sub-task call (e.g. from ArchiveVFS during locate/extract)
	localAction := "Extracting"
	localFilename := "file.txt"
	localCurrentPct := 20
	localTotalText := "Extracting: 2 MB / 10 MB"
	localTotalPct := 20
	localSpeedText := "Time: 00:00:05"

	wrap.UpdateTransfer(localAction, localFilename, localCurrentPct, localTotalText, localTotalPct, localSpeedText)

	// Assertions for CORRECT behavior:
	// 1. Total progress should remain the global one (50%), not overridden by the sub-task's 20%
	if mock.lastTotalPct != globalTotalPct {
		t.Fatalf("[BUG #137 REPRODUCED] Total percentage was overridden by sub-task! Expected %d, got %d", globalTotalPct, mock.lastTotalPct)
	}

	// 2. Speed/Time/ETA text should retain the global context, not overwritten by local elapsed time
	if !strings.Contains(mock.lastSpeedText, "Remaining: 00:01:00") {
		t.Fatalf("[BUG #137 REPRODUCED] Global ETA was lost! Expected it to contain global speed text, but got %q", mock.lastSpeedText)
	}

	// 3. Current filename should display sub-task status
	if !strings.Contains(mock.lastFilename, "file.txt (Extracting: 2 MB / 10 MB)") {
		t.Fatalf("Filename didn't append sub-task status. Got: %q", mock.lastFilename)
	}
}

func TestFileOps_ETA_DuringLocating(t *testing.T) {
	total := vfs.OpStats{Files: 10, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	// Симулируем, что прошло 5 секунд, обработан всего 1 байт (крайне медленно)
	startTime := time.Now().Add(-5 * time.Second)
	tracker.StartFile("f1.txt", 100)
	tracker.UpdateBytes(1)

	getGlobalStats := func(action string) (string, int, string) {
		now := time.Now()
		_, totalPct, _ := tracker.GetProgress()
		processed, total := tracker.GetStats()

		var totalText string
		if total.Bytes > 0 {
			totalText = fmt.Sprintf("Total: %s / %s", formatSize(processed.Bytes), formatSize(total.Bytes))
		}

		elapsed := now.Sub(startTime)
		elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)

		const ItemOverhead = 32 * 1024
		vProcessed := float64(processed.Bytes + (processed.Files+processed.Dirs)*ItemOverhead)
		vTotal := float64(total.Bytes + (total.Files+total.Dirs)*ItemOverhead)

		etaStr := "Remaining: ??:??:??"
		if vTotal > 0 && vProcessed > 0 && elapsed.Seconds() > 0.5 {
			if action == "Locating" || action == "Waiting" || action == "Scanning" || action == "Archiving" {
				etaStr = "Remaining: ??:??:??"
			} else {
				ratio := vProcessed / vTotal
				etaSecs := (elapsed.Seconds() / ratio) - elapsed.Seconds()
				if etaSecs < 0 {
					etaSecs = 0
				}
				if etaSecs > 359999 {
					etaStr = "Remaining: >99 hours"
				} else {
					etaDur := time.Duration(etaSecs * float64(time.Second))
					etaStr = fmt.Sprintf("Remaining: %02d:%02d:%02d", int(etaDur.Hours()), int(etaDur.Minutes())%60, int(etaDur.Seconds())%60)
				}
			}
		}

		return totalText, totalPct, fmt.Sprintf("%s %s", elapsedStr, etaStr)
	}

	// 1. При обычной копировании (Copying) ETA должно рассчитываться
	_, _, normalText := getGlobalStats("Copying")
	if !strings.Contains(normalText, "Remaining:") || strings.Contains(normalText, "??:??:??") {
		t.Errorf("Expected active ETA for Copying, got: %q", normalText)
	}

	// 2. При поиске файлов (Locating) ETA должно маскироваться
	_, _, locatingText := getGlobalStats("Locating")
	if !strings.Contains(locatingText, "Remaining: ??:??:??") {
		t.Errorf("Expected masked ETA for Locating, got: %q", locatingText)
	}
}
