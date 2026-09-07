package main

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestPTY_Lifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY test skipped on Windows")
	}

	pty, err := NewPTY()
	if err != nil {
		t.Fatalf("Failed to create PTY: %v", err)
	}

	shell := "/bin/sh"
	if _, err := os.Stat("/bin/bash"); err == nil {
		shell = "/bin/bash"
	}

	err = pty.Run(shell, "-c", "sleep 100")
	if err != nil {
		t.Fatalf("Failed to run shell: %v", err)
	}

	process := pty.Cmd.Process
	if process == nil {
		t.Fatal("Process not started")
	}

	// Verify it is running
	if err := process.Signal(os.Interrupt); err != nil {
		t.Errorf("Process not responding to signal: %v", err)
	}

	// Close PTY
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}

	// Wait a bit for OS to clean up
	time.Sleep(100 * time.Millisecond)

	// Verify process is killed (Signal(0) returns error if dead)
	err = process.Signal(os.Signal(nil))
	if err == nil {
		t.Error("Process is still alive after pty.Close()")
	}
}
