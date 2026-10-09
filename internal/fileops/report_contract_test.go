package fileops

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestOperationReportAndIgnoredErrorAccounting(t *testing.T) {
	report := openOpReport("Copy", FileOpOptions{IgnoreReadErrors: true, ReadAttempts: 2}, "/source", []string{"a.txt", "b.txt"}, "/dest")
	if report.file == nil || report.path == "" {
		t.Fatalf("openOpReport=%+v", report)
	}
	path := report.path
	tracker := NewFileOpTracker(vfs.OpStats{Files: 2, Bytes: 10})
	uiUpdates := 0
	state := &FileOpState{
		Report:           report,
		Tracker:          tracker,
		IgnoreReadErrors: true,
		UpdateUI:         func(bool) { uiUpdates++ },
	}
	state.fileCopied("/source/a.txt", "/dest/a.txt")
	state.skipItem("/source/b.txt", "/dest/b.txt")
	ordinary := errors.New("read failed")
	if !state.tolerateRead("/source/b.txt", ordinary) {
		t.Fatal("ordinary ignored read error was not tolerated")
	}
	if state.tolerateWrite("/dest/b.txt", ordinary) {
		t.Fatal("write error was tolerated while write-ignore was disabled")
	}
	if state.tolerateRead("/source/b.txt", context.Canceled) {
		t.Fatal("cancellation must not be tolerated")
	}
	if state.SkippedCount != 1 || state.FailedCount != 1 || uiUpdates != 2 {
		t.Fatalf("state counters skipped=%d failed=%d ui=%d", state.SkippedCount, state.FailedCount, uiUpdates)
	}

	report.close(errors.New("stopped"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	text := string(data)
	for _, want := range []string{"Copy started", "source folder: /source", "item: a.txt", "OK       /source/a.txt -> /dest/a.txt", "SKIPPED", "FAILED", "stopped", "finished: 1 done, 1 failed, 1 skipped"} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q in %q", want, text)
		}
	}
	if report.file != nil {
		t.Fatal("closed report still owns its file")
	}
	report.close(nil)
}

func TestOperationReportNilAndNoOpBranches(t *testing.T) {
	var report *opReport
	report.printf("ignored")
	report.close(nil)
	var state *FileOpState
	state.fileCopied("a", "b")
	state.note("ignored")
	if state.tolerate(false, "read", "a", errors.New("x")) {
		t.Fatal("nil state tolerated an error")
	}
	if (&FileOpState{}).tolerate(true, "read", "a", nil) {
		t.Fatal("nil error was tolerated")
	}
	if (&FileOpState{}).tolerate(true, "read", "a", vfs.ErrOperationPartial) {
		t.Fatal("uncertain operation was tolerated")
	}
}
