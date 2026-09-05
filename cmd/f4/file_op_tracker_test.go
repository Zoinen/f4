package main

import (
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestFileOpTracker_SingleFile(t *testing.T) {
	// Total: 1 file, 1000 bytes
	total := vfs.OpStats{Files: 1, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	tracker.StartFile("test.bin", 1000)
	tracker.UpdateBytes(500)

	filePct, totalPct, name := tracker.GetProgress()

	if name != "test.bin" {
		t.Errorf("Expected name test.bin, got %q", name)
	}
	if filePct != 50 {
		t.Errorf("Expected 50%% file progress, got %d", filePct)
	}
	if totalPct != 50 {
		t.Errorf("Expected 50%% total progress, got %d", totalPct)
	}

	tracker.UpdateBytes(500)
	tracker.FileDone()

	filePct, totalPct, _ = tracker.GetProgress()
	if filePct != 0 {
		t.Errorf("Expected 0%% file progress after FileDone, got %d", filePct)
	}
	if totalPct != 100 {
		t.Errorf("Expected 100%% total progress, got %d", totalPct)
	}
}

func TestFileOpTracker_MultiFile(t *testing.T) {
	// Total: 2 files, 200 bytes total (100 each)
	total := vfs.OpStats{Files: 2, Bytes: 200}
	tracker := NewFileOpTracker(total)

	// Finish first file
	tracker.StartFile("f1.txt", 100)
	tracker.UpdateBytes(100)
	tracker.FileDone()

	// Half of second file
	tracker.StartFile("f2.txt", 100)
	tracker.UpdateBytes(50)

	filePct, totalPct, _ := tracker.GetProgress()

	// File 2 is at 50%
	if filePct != 50 {
		t.Errorf("FilePct error: %d", filePct)
	}
	// Total is (100 + 50) / 200 = 75%
	if totalPct != 75 {
		t.Errorf("TotalPct error: expected 75, got %d", totalPct)
	}
}

func TestFileOpTracker_ZeroBytesFallback(t *testing.T) {
	// Scenario: 10 empty folders, 0 bytes total volume
	total := vfs.OpStats{Dirs: 10, Bytes: 0}
	tracker := NewFileOpTracker(total)

	for i := 0; i < 5; i++ {
		tracker.DirDone()
	}

	_, totalPct, _ := tracker.GetProgress()

	// Should use item count (5/10 = 50%)
	if totalPct != 50 {
		t.Errorf("Expected 50%% progress based on item count, got %d", totalPct)
	}
}

func TestFileOpTracker_ZeroByteFileWaitsForCommit(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1})
	tracker.StartFileKnown("empty-cloud-object", 0, true)
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 0 || totalPct != 0 {
		t.Fatalf("pre-commit zero-byte progress = %d/%d, want 0/0", filePct, totalPct)
	}
	tracker.SetCurrentPercent(50)
	filePct, totalPct, _ = tracker.GetProgress()
	if filePct != 50 || totalPct != 50 {
		t.Fatalf("provider zero-byte progress = %d/%d, want 50/50", filePct, totalPct)
	}
	tracker.FileDone()
	_, totalPct, _ = tracker.GetProgress()
	if totalPct != 100 {
		t.Fatalf("committed zero-byte total = %d, want 100", totalPct)
	}
}

func TestFileOpTracker_OverReporting(t *testing.T) {
	total := vfs.OpStats{Files: 1, Bytes: 100}
	tracker := NewFileOpTracker(total)

	tracker.StartFile("growth.log", 100)
	tracker.UpdateBytes(150) // More than announced

	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 100 || totalPct != 99 {
		t.Errorf("Over-reporting/commit reservation failed: file=%d, total=%d", filePct, totalPct)
	}
}

func TestFileOpTracker_EmptyJob(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{})
	_, totalPct, _ := tracker.GetProgress()
	if totalPct != 100 {
		t.Errorf("Empty job should report 100%%, got %d", totalPct)
	}
}

func TestFileOpTracker_SkippedFiles(t *testing.T) {
	// Total: 10 files, 1000 bytes
	total := vfs.OpStats{Files: 10, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	// Process 2 files
	tracker.StartFile("f1", 100)
	tracker.UpdateBytes(100)
	tracker.FileDone()
	tracker.StartFile("f2", 100)
	tracker.UpdateBytes(100)
	tracker.FileDone()

	// Skip 1 file (announced size was 100)
	tracker.StartFile("f3", 100)
	tracker.FileSkipped()

	processed, _ := tracker.GetStats()
	if processed.Files != 3 {
		t.Errorf("Expected 3 files processed (including skipped), got %d", processed.Files)
	}

	_, totalPct, _ := tracker.GetProgress()
	// (100 + 100 + 100) / 1000 = 30%
	if totalPct != 30 {
		t.Errorf("Progress mismatch after skip: expected 30%%, got %d%%", totalPct)
	}
}
func TestFileOpTracker_ProcessedBytes(t *testing.T) {
	total := vfs.OpStats{Files: 1, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	tracker.StartFile("f1", 1000)
	tracker.UpdateBytes(450)

	processed, _ := tracker.GetStats()
	if processed.Bytes != 450 {
		t.Errorf("GetStats should include in-progress bytes: expected 450, got %d", processed.Bytes)
	}

	tracker.UpdateBytes(50)
	tracker.FileDone()

	processed, _ = tracker.GetStats()
	if processed.Bytes != 1000 {
		t.Errorf("GetStats failed after FileDone: expected 1000, got %d", processed.Bytes)
	}
}

func TestFileOpTracker_UsesProviderCommitProgress(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	tracker.StartFile("remote.bin", 1000)

	if delta := tracker.SetCurrentPercent(37); delta != 370 {
		t.Fatalf("37%% delta = %d, want 370", delta)
	}
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 37 || totalPct != 37 {
		t.Fatalf("provider progress = file %d%% total %d%%", filePct, totalPct)
	}

	if delta := tracker.SetCurrentPercent(100); delta != 630 {
		t.Fatalf("100%% delta = %d, want 630", delta)
	}
	tracker.FileDone()
	_, totalPct, _ = tracker.GetProgress()
	if totalPct != 100 {
		t.Fatalf("committed total = %d%%", totalPct)
	}
}

func TestFileOpTracker_ProviderRetryNeverMovesProgressBackwards(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	tracker.StartFile("digest-upload.bin", 1000)
	if delta := tracker.SetCurrentPercent(72); delta != 720 {
		t.Fatalf("first delta = %d, want 720", delta)
	}
	if delta := tracker.SetCurrentPercent(3); delta != 0 {
		t.Fatalf("retry delta = %d, want 0", delta)
	}
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 72 || totalPct != 72 {
		t.Fatalf("progress moved backwards to file=%d total=%d", filePct, totalPct)
	}
}

func TestFileOpTracker_UnknownSizeUsesProviderPercentThenLearnedSize(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, UnknownSizeFiles: 1})
	tracker.StartFileKnown("native-export.bin", 0, false)
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 0 || totalPct != 0 {
		t.Fatalf("unknown-size initial progress = file %d total %d, want 0/0", filePct, totalPct)
	}
	if delta := tracker.SetCurrentPercent(41); delta != 0 {
		t.Fatalf("unknown-size byte delta = %d, want 0", delta)
	}
	filePct, totalPct, _ = tracker.GetProgress()
	if filePct != 41 {
		t.Fatalf("unknown-size file progress = %d, want 41", filePct)
	}
	if totalPct != 41 {
		t.Fatalf("unknown-size total progress = %d, want 41", totalPct)
	}
	tracker.SetCurrentSize(2000)
	processed, _ := tracker.GetStats()
	if processed.Bytes != 820 {
		t.Fatalf("learned-size processed bytes = %d, want 820", processed.Bytes)
	}
	if delta := tracker.SetCurrentPercent(50); delta != 180 {
		t.Fatalf("learned-size delta = %d, want 180", delta)
	}
}

func TestFileOpTracker_MixedKnownAndUnknownUsesItemProgress(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 2, Bytes: 1000, UnknownSizeFiles: 1})
	tracker.StartFileKnown("known.bin", 1000, true)
	tracker.SetCurrentPercent(100)
	tracker.FileDone()
	tracker.StartFileKnown("export.bin", 0, false)
	tracker.SetCurrentPercent(40)
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 40 || totalPct != 70 {
		t.Fatalf("mixed progress = file %d total %d, want 40/70", filePct, totalPct)
	}
	processed, total := tracker.GetStats()
	if total.Bytes != 1000 || total.UnknownSizeFiles != 1 || processed.Bytes != 1000 {
		t.Fatalf("mixed stats processed=%#v total=%#v", processed, total)
	}
}

func TestGlobalAwareReporterResetsProviderProgressPerFile(t *testing.T) {
	reporter := &globalAwareReporter{tracker: NewFileOpTracker(vfs.OpStats{Files: 2, Bytes: 2})}
	reporter.providerProgress.Store(true)
	reporter.StartFile("next.bin", 1)
	if reporter.providerProgress.Load() {
		t.Fatal("provider progress from the previous file leaked into the next file")
	}
}

func TestGlobalAwareReporterClampsRetriesButResetsDistinctTransferPhase(t *testing.T) {
	capture := &mockReporter{}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	tracker.StartFileKnown("cross-cloud.bin", 1000, true)
	accounted := 0
	reporter := &globalAwareReporter{
		original:  capture,
		tracker:   tracker,
		getGlobal: func(string) (string, int, string) { return "", 0, "" },
		onBytes:   func(n int) { accounted += n },
	}
	reporter.UpdateTransfer("Downloading", "cross-cloud.bin", 72, "", 72, "")
	reporter.UpdateTransfer("Downloading", "cross-cloud.bin", 3, "", 3, "")
	if capture.lastCurrentPct != 72 {
		t.Fatalf("same-phase retry moved visible bar to %d", capture.lastCurrentPct)
	}
	reporter.UpdateTransfer("Uploading", "cross-cloud.bin", 0, "", 0, "")
	if capture.lastCurrentPct != 0 {
		t.Fatalf("new upload phase did not reset current bar: %d", capture.lastCurrentPct)
	}
	reporter.UpdateTransfer("Uploading", "cross-cloud.bin", 50, "", 50, "")
	if capture.lastCurrentPct != 50 {
		t.Fatalf("upload phase progress = %d, want 50", capture.lastCurrentPct)
	}
	if accounted != 1220 { // 72% download + 50% upload.
		t.Fatalf("phase throughput bytes = %d, want 1220", accounted)
	}
	_, totalPct, _ := tracker.GetProgress()
	if totalPct != 72 {
		t.Fatalf("logical total regressed during second phase: %d", totalPct)
	}
}
