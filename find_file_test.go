package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFileContainsText_ChunkOverlap(t *testing.T) {
	// The word "SECRETPASSWORD" is 14 bytes long.
	// If chunk boundary splits it "SECRET" | "PASSWORD", we must still find it.

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "overlap.txt")

	// We will create a file just large enough to force our internal 128KB buffer to loop,
	// or we can test the function directly by overriding its internal buffer size logic
	// if it was exposed. Since it's hardcoded to 128KB, we write 128KB of padding,
	// then the secret word crossing the boundary.

	padding := make([]byte, 128*1024-6) // Leaves 6 bytes at the end of the first chunk
	for i := range padding {
		padding[i] = 'A'
	}

	data := append(padding, []byte("SECRETPASSWORD")...)
	os.WriteFile(path, data, 0644)

	v := vfs.NewOSVFS(tmpDir)

	// Test 1: Should find it
	found := fileContainsText(context.Background(), v, path, "secretpassword")
	if !found {
		t.Error("fileContainsText failed to find string crossing chunk boundary")
	}

	// Test 2: Should not find non-existent string
	foundMissing := fileContainsText(context.Background(), v, path, "missingpassword")
	if foundMissing {
		t.Error("fileContainsText falsely reported finding a non-existent string")
	}
}

func TestExecuteFindFile_MaskMatching(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test1.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test2.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test3.go"), []byte("package test"), 0644)

	v := vfs.NewOSVFS(tmpDir)

	ExecuteFindFile(nil, v, tmpDir, "*.go", "package")

	// Drain UI tasks to wait for search completion
	timeout := time.After(2 * time.Second)
	isDone := false
	for !isDone {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			// If a message box appears, the search finished
			if vtui.FrameManager.GetTopFrameType() == vtui.TypeDialog {
				frame := vtui.FrameManager.GetTopFrame()
				if frame != nil && frame.GetTitle() == " Search Results " {
					isDone = true
					// Search successfully finished and showed the results dialog
				}
			}
		case <-timeout:
			t.Fatal("Search operation timed out")
		}
	}

	if !isDone {
		t.Error("Search did not complete successfully")
	}
}

func TestLayout_SearchResultsDialog(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	v := vfs.NewOSVFS(t.TempDir())
	found := []FoundFile{{Path: filepath.FromSlash("/tmp/test.txt"), Item: vfs.VFSItem{Name: "test.txt", Size: 123}}}

	pf := NewPanelsFrame()
	defer pf.Close()
	ShowSearchResults(pf, v, found)

	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	vtui.AssertLayout(t, dlg)
}
