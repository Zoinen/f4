package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

type coverageTaskReporter struct {
	scans      int
	cancelled  bool
	lastAction string
	lastName   string
	lastPct    int
	totalText  string
	totalPct   int
	speedText  string
}

func (r *coverageTaskReporter) UpdateScan(_ string, _, _ int64) { r.scans++ }
func (r *coverageTaskReporter) IsCancelled() bool               { return r.cancelled }
func (r *coverageTaskReporter) UpdateTransfer(action, name string, pct int, total string, totalPct int, speed string) {
	r.lastAction = action
	r.lastName = name
	r.lastPct = pct
	r.totalText = total
	r.totalPct = totalPct
	r.speedText = speed
}

func TestGlobalAwareReporterCoversDelegationAndCommitPhases(t *testing.T) {
	original := &coverageTaskReporter{}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	accounted := 0
	reporter := &globalAwareReporter{
		original: original,
		tracker:  tracker,
		getGlobal: func(string) (string, int, string) {
			return "Total: 1 KB / 1 KB", 50, "global speed"
		},
		onBytes: func(n int) { accounted += n },
	}

	reporter.StartFileKnown("cross-cloud.bin", 0, false, 2, 1)
	reporter.SetCurrentSize(1000)
	reporter.SetCurrentSize(0)
	reporter.UpdateBytes(-1)
	reporter.UpdateBytes(100)
	reporter.UpdateTransfer("Uploading", "cross-cloud.bin", 40, "commit", 40, "local speed")

	if original.lastAction != "Uploading" || original.lastName != "cross-cloud.bin (commit)" {
		t.Fatalf("delegated transfer = %#v", original)
	}
	if original.totalPct != 50 || original.totalText != "Total: 1 KB / 1 KB" || original.speedText != "global speed" {
		t.Fatalf("global progress was not preserved: %#v", original)
	}
	if accounted != 500 {
		t.Fatalf("accounted phase bytes = %d, want 500", accounted)
	}

	reporter.UpdateScan("/tmp", 2, 1)
	if original.scans != 1 || reporter.IsCancelled() {
		t.Fatalf("delegation state = scans %d cancelled %v", original.scans, reporter.IsCancelled())
	}
	reporter.FileSkipped()
	reporter.DirDone()
	processed, _ := tracker.GetStats()
	if processed.Files != 1 || processed.Dirs != 1 {
		t.Fatalf("processed stats = %#v", processed)
	}

	reporter.StartFile("download.bin", 1000)
	reporter.UpdateTransfer("Downloading", "download.bin", 25, "", 25, "")
	before := accounted
	reporter.UpdateBytes(100)
	if accounted != before {
		t.Fatal("provider-owned download bytes were counted a second time")
	}
	original.cancelled = true
	if !reporter.IsCancelled() {
		t.Fatal("cancellation was not delegated")
	}
}

func TestFileOperationPureSafetyHelpers(t *testing.T) {
	closeCalls := 0
	closeErr := errors.New("close failed")
	close := closeOnce(testCloserFunc(func() error {
		closeCalls++
		return closeErr
	}))
	if err := close(); !errors.Is(err, closeErr) {
		t.Fatalf("first close error = %v", err)
	}
	if err := close(); err != nil || closeCalls != 1 {
		t.Fatalf("second close = %v, calls = %d", err, closeCalls)
	}

	partial := &vfs.PartialOperationError{}
	unknown := &vfs.UnknownOperationStateError{}
	for _, test := range []struct {
		name    string
		err     error
		display bool
	}{
		{name: "partial", err: partial, display: true},
		{name: "unknown", err: unknown, display: true},
		{name: "cancel", err: context.Canceled, display: false},
		{name: "deadline", err: context.DeadlineExceeded, display: true},
	} {
		if !operationMustNotRetry(test.err) || shouldDisplayFileOpError(test.err) != test.display {
			t.Fatalf("safety helpers disagreed for %s", test.name)
		}
	}
	if shouldDisplayFileOpError(nil) || shouldDisplayFileOpError(context.Canceled) {
		t.Fatal("cancellation or nil should not display an operation error")
	}
	if !shouldDisplayFileOpError(errors.New("ordinary failure")) {
		t.Fatal("ordinary operation failure was hidden")
	}

	dir := t.TempDir()
	linkTarget := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(linkTarget, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkTarget, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	resolved := resolveSymlinksForCompare(filepath.Join(link, "new", "file.txt"))
	want := filepath.Join(resolveSymlinksForCompare(linkTarget), "new", "file.txt")
	if resolved != want {
		t.Fatalf("resolved comparison path = %q, want %q", resolved, want)
	}
}

func TestTryOptimizedRenameUsesAtomicNoReplace(t *testing.T) {
	dir := t.TempDir()
	filesystem := vfs.NewOSVFS(dir)
	source := filepath.Join(dir, "source.txt")
	destination := filepath.Join(dir, "destination.txt")
	if err := os.WriteFile(source, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	ok, err := tryOptimizedRename(context.Background(), filesystem, filesystem, source, destination)
	if err != nil || !ok {
		t.Fatalf("empty-destination rename = %v, %v", ok, err)
	}

	source = filepath.Join(dir, "source-2.txt")
	if err := os.WriteFile(source, []byte("source-2"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	ok, err = tryOptimizedRename(context.Background(), filesystem, filesystem, source, destination)
	if err != nil || ok {
		t.Fatalf("occupied-destination rename = %v, %v", ok, err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "keep" {
		t.Fatalf("destination changed to %q, %v", data, err)
	}
}

type testCloserFunc func() error

func (f testCloserFunc) Close() error { return f() }
