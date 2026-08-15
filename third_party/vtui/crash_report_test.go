package vtui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrashReporter_MemoryLog(t *testing.T) {
	crashMu.Lock()
	// Save original and restore later
	origMax := maxLogLines
	origRing := logRing
	origIdx := logIdx
	origFull := logFull

	maxLogLines = 5
	logRing = nil // Force re-init in recordLogMemory
	logIdx = 0
	logFull = false
	crashMu.Unlock()

	defer func() {
		crashMu.Lock()
		maxLogLines = origMax
		logRing = origRing
		logIdx = origIdx
		logFull = origFull
		crashMu.Unlock()
	}()

	for i := 0; i < 7; i++ {
		recordLogMemory(fmt.Sprintf("L%d", i))
	}

	logs := getMemLogs()
	if len(logs) != 5 {
		t.Fatalf("Expected exactly 5 lines, got %d", len(logs))
	}
	if logs[0] != "L2" || logs[4] != "L6" {
		t.Errorf("Ring buffer wrapping failed: %v", logs)
	}
}

func TestRecordCrash(t *testing.T) {
	tmpDir := t.TempDir()
	CrashDirBase = tmpDir
	defer func() { CrashDirBase = "" }()

	recordLogMemory("Test line before crash")

	path := RecordCrash("Test Panic", []byte("goroutine 1 [running]:\nstack info"))
	if path == "" {
		t.Fatal("RecordCrash returned empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read crash file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "CRASH REPORT ===") {
		t.Error("Missing crash report header")
	}
	if !strings.Contains(content, "Test Panic") {
		t.Error("Missing panic value")
	}
	if !strings.Contains(content, "[running]") {
		t.Error("Missing stack trace")
	}
	if !strings.Contains(content, "Test line before crash") {
		t.Error("Missing log history")
	}
	if !strings.Contains(content, "Go Version:") {
		t.Error("Missing Go version info")
	}
}

func TestPruneStaleStderrLogs(t *testing.T) {
	dir := t.TempDir()

	// A pid far above any plausible live one: nothing owns this file.
	const deadPID = 4194303
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		return p
	}

	stale := write(fmt.Sprintf("stderr_20260101_000000_%d.log", deadPID), "")
	withOutput := write(fmt.Sprintf("stderr_20260101_000000_%d.log", deadPID+1), "panic: boom\n")
	ours := write(fmt.Sprintf("stderr_20260101_000000_%d.log", sessionPID), "")
	crash := write("crash_20260101_000000_1.log", "")
	unrelated := write("stderr_no_pid_here.log", "")

	pruneStaleStderrLogs(dir)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("empty log of a dead process was not removed")
	}
	for _, keep := range []string{withOutput, ours, crash, unrelated} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s should have been kept: %v", filepath.Base(keep), err)
		}
	}
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("this very process reported as not running")
	}
	if processAlive(4194303) {
		t.Skip("pid 4194303 happens to exist on this machine")
	}
}
