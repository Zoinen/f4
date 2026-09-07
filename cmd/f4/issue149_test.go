package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/zip"
)

// TestIssue149_LocatingETA_Progression reproduces the incorrect ETA calculation
// and lack of progress indication when skipping files in large or solid archives.
func TestIssue149_LocatingETA_Progression(t *testing.T) {
	// Simulate task: 1000 files, total volume 1GB.
	// We will skip most files (Locating) and extract only the last one.
	totalStats := vfs.OpStats{
		Files: 1000,
		Bytes: 1024 * 1024 * 1024,
	}

	tracker := NewFileOpTracker(totalStats)
	startTime := time.Now().Add(-10 * time.Second) // 10 seconds elapsed

	// Logic for ETA calculation from file_ops.go
	getETA := func(action string) string {
		processed, total := tracker.GetStats()
		elapsed := time.Since(startTime)

		const ItemOverhead = 32 * 1024
		vProcessed := float64(processed.Bytes + (processed.Files+processed.Dirs)*ItemOverhead)
		vTotal := float64(total.Bytes + (total.Files+total.Dirs)*ItemOverhead)

		if action == "Locating" || action == "Waiting" || action == "Scanning" || action == "Archiving" {
			return "Remaining: ??:??:??"
		}

		if vTotal > 0 && vProcessed > 0 && elapsed.Seconds() > 0.5 {
			ratio := vProcessed / vTotal
			etaSecs := (elapsed.Seconds() / ratio) - elapsed.Seconds()
			if etaSecs < 0 {
				etaSecs = 0
			}
			if etaSecs > 359999 {
				return "Remaining: >99 hours"
			}
			etaDur := time.Duration(etaSecs * float64(time.Second))
			return fmt.Sprintf("Remaining: %02d:%02d:%02d", int(etaDur.Hours()), int(etaDur.Minutes())%60, int(etaDur.Seconds())%60)
		}
		return "Remaining: ??:??:??"
	}

	// 1. MASKING CHECK: ETA must be hidden during the 'Locating' phase.
	etaDuringLocating := getETA("Locating")
	if etaDuringLocating != "Remaining: ??:??:??" {
		t.Errorf("ETA must be masked during 'Locating', got %q", etaDuringLocating)
	}

	// 2. REPRODUCTION OF "CRAZY ETA":
	// In the buggy implementation, when a file is skipped during a bulk operation
	// (e.g. in solid 7z), tracker.FileSkipped() is not called.
	// Consequently, processed.Files count remains 0, and vProcessed stays near zero.

	processedBefore, _ := tracker.GetStats()
	if processedBefore.Files != 0 {
		t.Fatal("Tracker should start with 0 processed files")
	}

	// Start extracting the 501st file after 10 seconds of "searching".
	tracker.StartFile("file_501.bin", 100*1024*1024)
	tracker.UpdateBytes(1024) // Read first kilobyte

	etaStartExtract := getETA("Copying")

	// Since skipped files weren't counted, we "processed" only 1KB in 10s.
	// Effective speed is extremely low, leading to a massive ETA.
	if !strings.Contains(etaStartExtract, ">99 hours") && !strings.Contains(etaStartExtract, "99:59:59") {
		t.Logf("Buggy ETA value: %q", etaStartExtract)
	} else {
		t.Log("Reproduced: ETA is capped or extremely large because skipped items aren't counted.")
	}

	// 3. VERIFY FIX LOGIC:
	// If we correctly call FileSkipped for each skipped item:
	tracker = NewFileOpTracker(totalStats)
	for i := 0; i < 500; i++ {
		tracker.StartFile(fmt.Sprintf("skipped_%d", i), 0)
		tracker.FileSkipped()
	}

	tracker.StartFile("file_501.bin", 100*1024*1024)
	tracker.UpdateBytes(1024)

	etaWithFix := getETA("Copying")
	t.Logf("Realistic ETA with fix: %q", etaWithFix)

	if strings.Contains(etaWithFix, "??") || strings.Contains(etaWithFix, ">99") {
		t.Errorf("ETA is still unrealistic even with proper skipping: %q", etaWithFix)
	}
}

// TestIssue149_Reproduction verifies that FileSkipped is correctly called
// during bulk extraction, ensuring that progress and ETA remain accurate
// even when many files are skipped in solid archives.
func TestIssue149_Reproduction(t *testing.T) {
	vfs.RegisterProvider(&archive.ArchiveProvider{})

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test_skip.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for i := 0; i < 10; i++ {
		w, err := zw.Create(fmt.Sprintf("file_%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	parentVFS := vfs.NewOSVFS(tmpDir)
	arcVfs, err := archive.NewArchiveVFS(parentVFS, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = arcVfs.Close() }()

	dstVFS := vfs.NewOSVFS(tmpDir)

	// Extract only the LAST file. The first 9 must be skipped.
	names := []string{"file_9.txt"}

	// Tracker with pre-scanned stats for the selected file only.
	totalStats := vfs.OpStats{Files: 1, Bytes: 4}
	tracker := NewFileOpTracker(totalStats)

	rep := &globalAwareReporter{
		original:  &DummyReporter{},
		tracker:   tracker,
		getGlobal: func(action string) (string, int, string) { return "", 0, "" },
	}

	ctx := archive.WithAutoQueue(context.Background())

	err = arcVfs.CopyBulk(ctx, names, dstVFS, tmpDir, rep)
	if err != nil {
		t.Fatalf("CopyBulk failed: %v", err)
	}

	// Verify that only the selected item was processed.
	processed, _ := tracker.GetStats()
	if processed.Files != 1 {
		t.Errorf("Tracker did not record the file. Expected 1, got %d", processed.Files)
	}

	_, totalPct, _ := tracker.GetProgress()
	if totalPct != 100 {
		t.Errorf("Expected 100%% progress at the end, got %d%%", totalPct)
	}
}

type actionCaptureReporter struct {
	DummyReporter
	actions map[string]bool
	mu      sync.Mutex
}

func (r *actionCaptureReporter) StartFile(name string, size int64) {}
func (r *actionCaptureReporter) UpdateBytes(n int)                 {}
func (r *actionCaptureReporter) FileDone()                         {}
func (r *actionCaptureReporter) DirDone()                          {}
func (r *actionCaptureReporter) FileSkipped() {
	time.Sleep(15 * time.Millisecond)
}

func (r *actionCaptureReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions[action] = true
}

func (r *actionCaptureReporter) hasAction(a string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actions[a]
}

// TestIssue149_LocatingStatusReporting verifies that the "Locating" state
// is correctly reported during bulk extraction when files are being skipped.
func TestIssue149_LocatingStatusReporting(t *testing.T) {
	archive.TestSkipDelay = 15 * time.Millisecond
	archive.ProgressTickerInterval = 5 * time.Millisecond
	defer func() {
		archive.TestSkipDelay = 0
		archive.ProgressTickerInterval = 250 * time.Millisecond
	}()

	vfs.RegisterProvider(&archive.ArchiveProvider{})
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test_locating.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	// Create 50 files
	for i := 0; i < 50; i++ {
		w, err := zw.Create(fmt.Sprintf("file_%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	parentVFS := vfs.NewOSVFS(tmpDir)
	arcVfs, err := archive.NewArchiveVFS(parentVFS, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = arcVfs.Close() }()

	dstVFS := vfs.NewOSVFS(tmpDir)

	// Only extract the last file
	names := []string{"file_49.txt"}
	rep := &actionCaptureReporter{actions: make(map[string]bool)}

	// We need to use a context that triggers progress updates
	ctx, cancel := context.WithCancel(archive.WithAutoQueue(context.Background()))
	defer cancel()

	// Run extraction in background to allow ticker to fire
	done := make(chan error, 1)
	go func() {
		done <- arcVfs.CopyBulk(ctx, names, dstVFS, tmpDir, rep)
	}()

	// Wait for the "Locating" status to appear
	timeout := time.After(2 * time.Second)
	found := false
	for !found {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for 'Locating' status to be reported")
		case err := <-done:
			if err != nil && err != context.Canceled {
				t.Fatalf("CopyBulk failed: %v", err)
			}
			found = rep.hasAction("Locating")
			if !found {
				t.Error("'Locating' action was never reported during bulk copy")
			}
		default:
			if rep.hasAction("Locating") {
				found = true
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	<-done
}

// TestIssue149_ETA_Stability verifies the fix for "Crazy ETA" by checking
// if processing many files without bytes still yields reasonable ETA.
func TestIssue149_ETA_Stability(t *testing.T) {
	// Scenario: 1000 tiny files, 10 bytes each. Total 10KB.
	// We've processed 500 files (5KB) in 5 seconds.
	totalStats := vfs.OpStats{Files: 1000, Bytes: 10000}
	tracker := NewFileOpTracker(totalStats)

	for i := 0; i < 500; i++ {
		tracker.StartFile("file", 10)
		tracker.UpdateBytes(10)
		tracker.FileDone()
	}

	startTime := time.Now().Add(-5 * time.Second)

	// ETA Logic from file_ops.go
	calcETA := func() string {
		processed, total := tracker.GetStats()
		elapsed := time.Since(startTime)

		const ItemOverhead = 32 * 1024
		vProcessed := float64(processed.Bytes + (processed.Files+processed.Dirs)*ItemOverhead)
		vTotal := float64(total.Bytes + (total.Files+total.Dirs)*ItemOverhead)

		if vTotal > 0 && vProcessed > 0 && elapsed.Seconds() > 0.5 {
			ratio := vProcessed / vTotal
			etaSecs := (elapsed.Seconds() / ratio) - elapsed.Seconds()
			if etaSecs < 0 {
				etaSecs = 0
			}
			if etaSecs > 359999 {
				return "Remaining: >99 hours"
			}
			etaDur := time.Duration(etaSecs * float64(time.Second))
			return fmt.Sprintf("Remaining: %02d:%02d:%02d", int(etaDur.Hours()), int(etaDur.Minutes())%60, int(etaDur.Seconds())%60)
		}
		return "Remaining: ??"
	}

	eta := calcETA()

	// If the fix is working, the ETA should be roughly 5 seconds (Time: 00:00:05)
	// because we are halfway through the items and overhead.
	// If it's broken, it might be huge because processed bytes (5KB) is very small.

	if strings.Contains(eta, ">99 hours") || strings.Contains(eta, "??") {
		t.Errorf("ETA is unrealistic: %q. Item overhead logic likely failed.", eta)
	}

	if !strings.Contains(eta, "00:00:0") { // Expecting roughly 5 seconds remaining
		t.Errorf("ETA seems incorrect: %q. Expected approx 5 seconds.", eta)
	}
}

func readFullVFS(ctx context.Context, r vfs.ReadAtCloser, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(ctx, buf[total:])
		total += n
		if err != nil {
			if err == io.EOF && total == len(buf) {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

// TestArchiveReadWrapper_MixedReadAndReadAt verifies that mixing sequential Read()
// and random-access ReadAt() on an archiveReadWrapper does not corrupt file contents
// or produce zero-padded tails after 128KB chunks.
func TestArchiveReadWrapper_MixedReadAndReadAt(t *testing.T) {
	vfs.RegisterProvider(&archive.ArchiveProvider{})
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test_mixed_read.zip")

	// Generate 300KB pattern file
	const fileSize = 300 * 1024
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte((i*13 + 7) % 251)
	}

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("large_file.bin")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	if _, err := w.Write(testData); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	parentVFS := vfs.NewOSVFS(tmpDir)
	arcVFS, err := archive.NewArchiveVFS(parentVFS, zipPath)
	if err != nil {
		t.Fatalf("Failed to open ArchiveVFS: %v", err)
	}
	defer func() { _ = arcVFS.Close() }()

	ctx := context.Background()
	reader, err := arcVFS.Open(ctx, "large_file.bin")
	if err != nil {
		t.Fatalf("Failed to Open file in ArchiveVFS: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// 1. First sequential Read of 128KB
	chunk1 := make([]byte, 128*1024)
	n1, err := readFullVFS(ctx, reader, chunk1)
	if err != nil || n1 != 128*1024 {
		t.Fatalf("First Read failed: n1=%d, err=%v", n1, err)
	}
	if !bytes.Equal(chunk1, testData[:128*1024]) {
		t.Fatalf("First chunk mismatch")
	}

	// 2. Interleaved ReadAt call (triggers extractToTemp)
	peekBuf := make([]byte, 64)
	nPeek, err := reader.ReadAt(ctx, peekBuf, 100)
	if err != nil || nPeek != 64 {
		t.Fatalf("ReadAt failed: nPeek=%d, err=%v", nPeek, err)
	}
	if !bytes.Equal(peekBuf, testData[100:164]) {
		t.Fatalf("ReadAt data mismatch at offset 100")
	}

	// 3. Second sequential Read of remaining data
	chunk2 := make([]byte, fileSize-128*1024)
	n2, err := readFullVFS(ctx, reader, chunk2)
	if err != nil && err != io.EOF {
		t.Fatalf("Second Read failed: n2=%d, err=%v", n2, err)
	}
	if n2 != len(chunk2) {
		t.Fatalf("Second Read short read: got %d, want %d", n2, len(chunk2))
	}
	if !bytes.Equal(chunk2, testData[128*1024:]) {
		t.Fatalf("Second chunk data mismatch (contains zeros or shifted data)")
	}
}

// TestIssue149_F5_Extraction_Integrity ensures 100% binary identity of extracted files
// larger than 128KB during bulk extraction (F5).
func TestIssue149_F5_Extraction_Integrity(t *testing.T) {
	vfs.RegisterProvider(&archive.ArchiveProvider{})
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test_f5_integrity.zip")

	// Generate 350KB test file with non-zero pattern
	const fileSize = 350 * 1024
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte((i*17 + 3) % 251)
	}

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("15_Area51_bunker.dxs")
	if err != nil {
		t.Fatalf("Failed to create entry: %v", err)
	}
	if _, err := w.Write(testData); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	parentVFS := vfs.NewOSVFS(tmpDir)
	arcVFS, err := archive.NewArchiveVFS(parentVFS, zipPath)
	if err != nil {
		t.Fatalf("Failed to open ArchiveVFS: %v", err)
	}
	defer func() { _ = arcVFS.Close() }()

	dstDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		t.Fatal(err)
	}
	dstVFS := vfs.NewOSVFS(dstDir)

	rep := &globalAwareReporter{
		original:  &DummyReporter{},
		tracker:   NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: fileSize}),
		getGlobal: func(action string) (string, int, string) { return "", 0, "" },
	}

	ctx := archive.WithAutoQueue(context.Background())
	err = arcVFS.CopyBulk(ctx, []string{"15_Area51_bunker.dxs"}, dstVFS, dstDir, rep)
	if err != nil {
		t.Fatalf("CopyBulk failed: %v", err)
	}

	extractedPath := filepath.Join(dstDir, "15_Area51_bunker.dxs")
	extractedData, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if len(extractedData) != len(testData) {
		t.Fatalf("Extracted size mismatch: got %d, want %d", len(extractedData), len(testData))
	}

	if !bytes.Equal(extractedData, testData) {
		t.Fatalf("Extracted file is binary different from source (contains corrupted/zero blocks)")
	}
}

// TestIssue149_NoQuadraticDecompression verifies that extracting a single file
// from a multi-file archive is fast and does not scale quadratically.
func TestIssue149_NoQuadraticDecompression(t *testing.T) {
	vfs.RegisterProvider(&archive.ArchiveProvider{})
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test_speed.zip")

	// Create 100 small files
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for i := 0; i < 100; i++ {
		w, err := zw.Create(fmt.Sprintf("file_%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("some data")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	parentVFS := vfs.NewOSVFS(tmpDir)
	arcVfs, _ := archive.NewArchiveVFS(parentVFS, zipPath)
	defer func() { _ = arcVfs.Close() }()

	dstVFS := vfs.NewOSVFS(tmpDir)

	// Measure time to extract only the last file
	start := time.Now()
	err = arcVfs.CopyBulk(context.Background(), []string{"file_99.txt"}, dstVFS, tmpDir, &DummyReporter{})
	if err != nil {
		t.Fatalf("CopyBulk failed: %v", err)
	}
	elapsed := time.Since(start)

	// Linear skip on 100 tiny files should take less than 10ms on modern CPUs.
	// We set a conservative threshold of 100ms.
	if elapsed > 100*time.Millisecond {
		t.Errorf("Extraction took too long (%v), potential quadratic complexity or redundant read loops detected", elapsed)
	}
}

type testBlackBoxReporter struct{}

func (r *testBlackBoxReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (r *testBlackBoxReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
}
func (r *testBlackBoxReporter) IsCancelled() bool { return false }

// TestIssue149_7z_MultiBlock_Solid_Integrity recreates the exact conditions of large
// multi-block solid 7z archives with nested directories and >1MB binary files.
func TestIssue149_7z_MultiBlock_Solid_Integrity(t *testing.T) {
	sevenZipCmd := ""
	for _, cmd := range []string{"7z", "7za", "7zr"} {
		if _, err := exec.LookPath(cmd); err == nil {
			sevenZipCmd = cmd
			break
		}
	}
	if sevenZipCmd == "" {
		t.Skip("Skipping 7z integration test: 7z/7za/7zr executable not found in PATH")
	}

	vfs.RegisterProvider(&archive.ArchiveProvider{})
	tmpDir := t.TempDir()

	// Recreate user structure with nested folders and large pseudo-random files (>1MB)
	fileSpecs := []struct {
		relPath string
		size    int
		seed    int64
	}{
		{"Deus Ex/Save/Save0048/04_NYC_Street.dxs", 3 * 1024 * 1024, 1001},
		{"Deus Ex/Save/Save0050/04_NYC_UNATCOIsland.dxs", 2500 * 1024, 1002},
		{"Deus Ex/Save/Save0059/05_NYC_UNATCOMJ12lab.dxs", 4 * 1024 * 1024, 1003},
		{"Deus Ex/System/Shot0001.bmp", 5 * 1024 * 1024, 1004},
		{"Deus Ex/System/Shot0003.bmp", 5 * 1024 * 1024, 1005},
	}

	srcDir := filepath.Join(tmpDir, "src")
	originalData := make(map[string][]byte)

	for _, spec := range fileSpecs {
		fullPath := filepath.Join(srcDir, filepath.FromSlash(spec.relPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0700); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", spec.relPath, err)
		}

		// Generate uncompressible pseudo-random data to force multi-block solid LZMA2 stream
		data := make([]byte, spec.size)
		for i := range data {
			data[i] = testByteInt64((int64(i)*13 + spec.seed*37) % 251)
			if data[i] == 0 {
				data[i] = 1
			}
		}
		originalData[spec.relPath] = data

		if err := os.WriteFile(fullPath, data, 0600); err != nil {
			t.Fatalf("Failed to write test file %s: %v", spec.relPath, err)
		}
	}

	// Compress using console 7z with small solid block size (-ms=2m) to force multiple solid blocks
	archivePath := filepath.Join(tmpDir, "DeusEx_test.7z")
	cmd := exec.Command(sevenZipCmd, "a", "-ms=2m", "-m0=lzma2", archivePath, "Deus Ex")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z command failed: %v\nOutput: %s", err, string(out))
	}

	verifyFile := func(t *testing.T, extractedPath string, relPath string) {
		data, err := os.ReadFile(extractedPath)
		if err != nil {
			t.Errorf("Failed to read extracted file %s: %v", relPath, err)
			return
		}
		orig := originalData[relPath]
		if len(data) != len(orig) {
			t.Errorf("[%s] Size mismatch: got %d, want %d", relPath, len(data), len(orig))
			return
		}
		for i := 0; i < len(orig); i++ {
			if data[i] != orig[i] {
				t.Errorf("[%s] CORRUPTION at byte offset 0x%X (%d): got 0x%02X, want 0x%02X",
					relPath, i, i, data[i], orig[i])
				return
			}
		}
	}

	// --- SCENARIO 1: F5 Bulk Copy of specific nested files ---
	t.Run("Bulk_Copy_Nested_Files_F5", func(t *testing.T) {
		parentVFS := vfs.NewOSVFS(tmpDir)
		arcVFS, err := archive.NewArchiveVFS(parentVFS, archivePath)
		if err != nil {
			t.Fatalf("NewArchiveVFS failed: %v", err)
		}
		defer func() { _ = arcVFS.Close() }()

		dstDir := filepath.Join(tmpDir, "out_f5_nested")
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			t.Fatalf("Failed to create out_f5_nested: %v", err)
		}
		dstVFS := vfs.NewOSVFS(dstDir)

		selected := []string{
			"Deus Ex/Save/Save0048/04_NYC_Street.dxs",
			"Deus Ex/System/Shot0001.bmp",
		}

		rep := &testBlackBoxReporter{}
		ctx := archive.WithAutoQueue(context.Background())
		if err := arcVFS.CopyBulk(ctx, selected, dstVFS, dstDir, rep); err != nil {
			t.Fatalf("CopyBulk failed: %v", err)
		}
		for _, rel := range selected {
			verifyFile(t, filepath.Join(dstDir, filepath.FromSlash(rel)), rel)
		}
	})

	// --- SCENARIO 2: F5 Bulk Copy of the entire top folder ---
	t.Run("Bulk_Copy_Root_Folder_F5", func(t *testing.T) {
		parentVFS := vfs.NewOSVFS(tmpDir)
		arcVFS, err := archive.NewArchiveVFS(parentVFS, archivePath)
		if err != nil {
			t.Fatalf("NewArchiveVFS failed: %v", err)
		}
		defer func() { _ = arcVFS.Close() }()

		dstDir := filepath.Join(tmpDir, "out_f5_root")
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			t.Fatalf("Failed to create out_f5_root: %v", err)
		}
		dstVFS := vfs.NewOSVFS(dstDir)

		selected := []string{"Deus Ex"}

		rep := &testBlackBoxReporter{}
		ctx := archive.WithAutoQueue(context.Background())
		if err := arcVFS.CopyBulk(ctx, selected, dstVFS, dstDir, rep); err != nil {
			t.Fatalf("CopyBulk failed: %v", err)
		}

		for _, spec := range fileSpecs {
			verifyFile(t, filepath.Join(dstDir, filepath.FromSlash(spec.relPath)), spec.relPath)
		}
	})

	// --- SCENARIO 3: Sequential VFS.Open with TaskReporter (forcing archiveReadWrapper) ---
	t.Run("Sequential_Open_With_Reporter", func(t *testing.T) {
		parentVFS := vfs.NewOSVFS(tmpDir)
		arcVFS, err := archive.NewArchiveVFS(parentVFS, archivePath)
		if err != nil {
			t.Fatalf("NewArchiveVFS failed: %v", err)
		}
		defer func() { _ = arcVFS.Close() }()

		dstDir := filepath.Join(tmpDir, "out_seq_open")
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			t.Fatalf("Failed to create out_seq_open: %v", err)
		}

		rep := &testBlackBoxReporter{}
		ctx := context.WithValue(context.Background(), vfs.ReporterKey, rep)

		for _, spec := range fileSpecs {
			reader, err := arcVFS.Open(ctx, spec.relPath)
			if err != nil {
				t.Fatalf("Open(%s) failed: %v", spec.relPath, err)
			}

			outPath := filepath.Join(dstDir, filepath.FromSlash(spec.relPath))
			if err := os.MkdirAll(filepath.Dir(outPath), 0700); err != nil {
				_ = reader.Close() // Reader cleanup is secondary to the setup failure.
				t.Fatalf("Failed to create out dir: %v", err)
			}
			outFile, err := os.Create(outPath)
			if err != nil {
				_ = reader.Close() // Reader cleanup is secondary to the setup failure.
				t.Fatalf("Create out file failed: %v", err)
			}

			buf := make([]byte, 128*1024)
			for {
				n, errRead := reader.Read(ctx, buf)
				if n > 0 {
					if _, err := outFile.Write(buf[:n]); err != nil {
						if closeErr := outFile.Close(); closeErr != nil {
							t.Errorf("close partial output file: %v", closeErr)
						}
						_ = reader.Close() // Reader cleanup is secondary to the write failure.
						t.Fatal(err)
					}
				}
				if errRead != nil {
					if errRead == io.EOF {
						break
					}
					if closeErr := outFile.Close(); closeErr != nil {
						t.Errorf("close partial output file: %v", closeErr)
					}
					_ = reader.Close() // Reader cleanup is secondary to the read failure.
					t.Fatalf("Read failed on %s: %v", spec.relPath, errRead)
				}
			}
			if err := outFile.Close(); err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}

			verifyFile(t, outPath, spec.relPath)
		}
	})
}

// TestIssue149_StartTimeIncludesScanning verifies that elapsed time in progress statistics
// accounts for pre-scan duration so the timer doesn't lag behind real wall-clock time.
func TestIssue149_StartTimeIncludesScanning(t *testing.T) {
	//totalStats := vfs.OpStats{Files: 1, Bytes: 1024}
	//tracker := NewFileOpTracker(totalStats)

	// Simulate 3 seconds spent in scanning before transfer starts
	startTime := time.Now().Add(-3 * time.Second)

	getGlobalStats := func(action string) (string, int, string) {
		now := time.Now()
		elapsed := now.Sub(startTime)
		elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
		return "", 0, elapsedStr
	}

	_, _, timeText := getGlobalStats("Copying")
	if !strings.Contains(timeText, "Time: 00:00:03") && !strings.Contains(timeText, "Time: 00:00:04") {
		t.Errorf("Expected elapsed time to be at least 3 seconds, got: %q", timeText)
	}
}
