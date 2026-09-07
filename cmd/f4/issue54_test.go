package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestIssue54_History(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	// 1. Create a real executable script in a temporary directory
	tmpDir := t.TempDir()
	scriptName := "runme.sh"
	scriptPath := filepath.Join(tmpDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho 1"), 0600); err != nil {
		t.Fatal(err)
	}

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// 2. Setup the panel to point to this directory
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.vfs = vfs.NewOSVFS(tmpDir)

	// We manually populate entries to avoid waiting for async ReadDir
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: scriptName, IsDir: false, IsExecutable: true}},
	}
	fsp.Refresh()
	fsp.SetCursorIndex(1)
	pf.activeIdx = 0

	// 3. Trigger Enter on the script
	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// 4. Wait for async IsTerminalRunnable and the resulting History update task.
	// We use a non-blocking drain to prevent deadlocks and handle the async work time.
	timeout := time.After(2 * time.Second)
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if len(pf.cmdLine.Edit.History) > 0 {
				break Loop
			}
		case <-timeout:
			t.Fatal("History update timed out - runnable check likely failed or task wasn't posted")
		default:
			// Give the async goroutine a chance to run
			time.Sleep(10 * time.Millisecond)
		}
	}

	// 5. Verify results
	history := pf.cmdLine.Edit.History
	expected := scriptName
	if runtime.GOOS != "windows" {
		expected = "./" + expected
	}
	if history[0] != expected {
		t.Errorf("History item format error. Got: %q, want: %q", history[0], expected)
	}
}
