package vfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpStats_Add(t *testing.T) {
	s1 := OpStats{Bytes: 100, Files: 2, Dirs: 1}
	s2 := OpStats{Bytes: 50, Files: 1, Dirs: 0}
	s1.Add(s2)

	if s1.Bytes != 150 || s1.Files != 3 || s1.Dirs != 1 {
		t.Errorf("OpStats.Add failed: %+v", s1)
	}
}

func TestGenericScan_FlatFiles(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "f1.txt"), []byte("abc"), 0644)  // 3 bytes
	os.WriteFile(filepath.Join(tmpDir, "f2.txt"), []byte("defg"), 0644) // 4 bytes

	stats, err := GenericScan(context.Background(), v, tmpDir, []string{"f1.txt", "f2.txt"}, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if stats.Bytes != 7 {
		t.Errorf("Expected 7 bytes, got %d", stats.Bytes)
	}
	if stats.Files != 2 {
		t.Errorf("Expected 2 files, got %d", stats.Files)
	}
	if stats.Dirs != 0 {
		t.Errorf("Expected 0 dirs, got %d", stats.Dirs)
	}
}

func TestGenericScan_Recursive(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	// Structure:
	// /root_dir (Dir)
	// /root_dir/file1.txt (5 bytes)
	// /root_dir/sub_dir (Dir)
	// /root_dir/sub_dir/file2.txt (10 bytes)

	rootDir := filepath.Join(tmpDir, "root_dir")
	subDir := filepath.Join(rootDir, "sub_dir")
	os.MkdirAll(subDir, 0755)

	os.WriteFile(filepath.Join(rootDir, "file1.txt"), make([]byte, 5), 0644)
	os.WriteFile(filepath.Join(subDir, "file2.txt"), make([]byte, 10), 0644)

	var lastReportedPath string
	callCount := 0
	cb := func(currentPath string, currentStats OpStats) {
		lastReportedPath = currentPath
		callCount++
	}

	stats, err := GenericScan(context.Background(), v, tmpDir, []string{"root_dir"}, cb)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// 2 files, 2 directories (root_dir, sub_dir)
	if stats.Bytes != 15 {
		t.Errorf("Expected 15 bytes, got %d", stats.Bytes)
	}
	if stats.Files != 2 {
		t.Errorf("Expected 2 files, got %d", stats.Files)
	}
	if stats.Dirs != 2 {
		t.Errorf("Expected 2 dirs, got %d", stats.Dirs)
	}

	// Callback should be called for root_dir, sub_dir, and both files
	if callCount != 4 {
		t.Errorf("Expected callback to be called 4 times, got %d", callCount)
	}
	if lastReportedPath == "" {
		t.Error("Callback path was not populated")
	}
}

func TestGenericScan_Cancellation(t *testing.T) {
	// We use NullVFS to ensure predictable deep structure without disk IO
	// But NullVFS currently doesn't simulate deep folders easily,
	// so let's use OSVFS and just create a few folders, then cancel.
	tmpDir := t.TempDir()
	v := NewOSVFS(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "dir1", "dir2", "dir3"), 0755)

	ctx, cancel := context.WithCancel(context.Background())

	cb := func(currentPath string, currentStats OpStats) {
		// Cancel immediately after seeing the first item
		cancel()
	}

	_, err := GenericScan(ctx, v, tmpDir, []string{"dir1"}, cb)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestCalculateStats_FastScannerSupport(t *testing.T) {
	// A dummy VFS that implements FastScanner
	fastVFS := &mockFastVFS{
		OSVFS: *NewOSVFS("."),
	}

	stats, err := CalculateStats(context.Background(), fastVFS, "/", []string{"dummy"}, nil)
	if err != nil {
		t.Fatalf("FastScan failed: %v", err)
	}

	if stats.Bytes != 999 {
		t.Errorf("Expected FastScanner to be used (Bytes=999), got %d", stats.Bytes)
	}
	if !fastVFS.scanCalled {
		t.Error("FastScanner.Scan was not called")
	}
}
func TestGenericScan_Empty(t *testing.T) {
	v := NewOSVFS(t.TempDir())
	stats, err := GenericScan(context.Background(), v, "/", []string{}, nil)
	if err != nil {
		t.Errorf("Empty scan should not return error, got %v", err)
	}
	if stats.Bytes != 0 || stats.Files != 0 || stats.Dirs != 0 {
		t.Errorf("Empty scan should return zero stats, got %+v", stats)
	}
}

func TestGenericScan_DepthLimit(t *testing.T) {
	mv := &mockScannerVFS{}
	// Setup a self-referencing "infinite" directory structure in the mock
	mv.onStat = func(p string) VFSItem {
		return VFSItem{Name: "dir", IsDir: true}
	}
	mv.onReadDir = func(p string) []VFSItem {
		return []VFSItem{{Name: "subdir", IsDir: true}}
	}

	_, err := GenericScan(context.Background(), mv, "/", []string{"root"}, nil)
	if err == nil || !strings.Contains(err.Error(), "maximum recursion depth exceeded") {
		t.Errorf("Expected depth limit error, got %v", err)
	}
}

func TestGenericScan_Errors(t *testing.T) {
	mv := &mockScannerVFS{}

	t.Run("Stat error", func(t *testing.T) {
		mv.err = fmt.Errorf("stat failed")
		_, err := GenericScan(context.Background(), mv, "/", []string{"badfile"}, nil)
		if err == nil || err.Error() != "stat failed" {
			t.Errorf("Expected stat error, got %v", err)
		}
	})

	t.Run("ReadDir error", func(t *testing.T) {
		mv.err = nil // Stat works
		mv.readDirErr = fmt.Errorf("readdir failed")
		// Simulate a directory
		mv.onStat = func(p string) VFSItem {
			return VFSItem{Name: "dir", IsDir: true}
		}

		// ReadDir errors are swallowed by the scanner — matches
		// far2l's ScanTree, which silently steps over permission-
		// denied subtrees so the walk returns partial totals rather
		// than aborting on the first sudo-only directory. The scan
		// should return no error and just count the parent dir.
		stats, err := GenericScan(context.Background(), mv, "/", []string{"dir"}, nil)
		if err != nil {
			t.Errorf("ReadDir failure should be swallowed, got %v", err)
		}
		if stats.Dirs != 1 {
			t.Errorf("Dirs = %d, want 1 (root dir counted even when ReadDir fails)", stats.Dirs)
		}
	})
}

// --- Additional Mocks ---

type mockScannerVFS struct {
	OSVFS
	err        error
	readDirErr error
	onStat     func(string) VFSItem
	onReadDir  func(string) []VFSItem
}

func (m *mockScannerVFS) Stat(ctx context.Context, p string) (VFSItem, error) {
	if m.err != nil {
		return VFSItem{}, m.err
	}
	if m.onStat != nil {
		return m.onStat(p), nil
	}
	return VFSItem{Name: "item", IsDir: false}, nil
}

func (m *mockScannerVFS) ReadDir(ctx context.Context, p string, onChunk func([]VFSItem)) error {
	if m.readDirErr != nil {
		return m.readDirErr
	}
	if m.onReadDir != nil {
		onChunk(m.onReadDir(p))
	}
	return nil
}

func (m *mockScannerVFS) Join(e ...string) string { return filepath.Join(e...) }

// --- Mocks ---

type mockFastVFS struct {
	OSVFS
	scanCalled bool
}

func (m *mockFastVFS) Scan(ctx context.Context, basePath string, names []string, cb ScanCallback) (OpStats, error) {
	m.scanCalled = true
	return OpStats{Bytes: 999, Files: 1, Dirs: 1}, nil
}

// TestGenericScan_SymlinkDirLeafVsFollow verifies both walk modes on
// a tree with a symlink-to-directory. Layout:
//
//	root/
//	  real/
//	    a.bin (100)
//	    b.bin (50)
//	  link -> real
//
// FollowSymlinkDirs=true (default; copy/move pre-scan) walks
// root/link/* a second time, so files/bytes are doubled. false
// (QuickView) counts the link once as a leaf — same numbers `find`
// and far2l would report.
func TestGenericScan_SymlinkDirLeafVsFollow(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.Mkdir(real, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.bin"), make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "b.bin"), make([]byte, 50), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(tmp, "link")); err != nil {
		t.Skipf("cannot create symlink (fs may not support it): %v", err)
	}

	v := NewOSVFS(tmp)

	// Follow-through mode: two file entries under real/ plus two more
	// under link/ (walked as if it were a directory) — 4 files, 300 B.
	follow, err := CalculateStatsWithOptions(context.Background(), v, tmp, []string{"."}, ScanOptions{FollowSymlinkDirs: true}, nil)
	if err != nil {
		t.Fatalf("follow scan: %v", err)
	}
	if follow.Files != 4 || follow.Bytes != 300 {
		t.Errorf("follow: Files=%d Bytes=%d, want 4/300", follow.Files, follow.Bytes)
	}

	// Leaf mode: link counted once as a plain entry — 2 file entries
	// (a.bin + b.bin) + 1 for the link, and only real's bytes.
	leaf, err := CalculateStatsWithOptions(context.Background(), v, tmp, []string{"."}, ScanOptions{FollowSymlinkDirs: false}, nil)
	if err != nil {
		t.Fatalf("leaf scan: %v", err)
	}
	if leaf.Files != 3 {
		t.Errorf("leaf: Files=%d, want 3 (2 real files + 1 symlink counted as leaf)", leaf.Files)
	}
	// Symlink's Size is the length of the target path — some bytes but
	// certainly less than the sum of the two files it points at (150).
	// The important assertion is that we did NOT double-count real/'s
	// contents through the link.
	if leaf.Bytes >= follow.Bytes {
		t.Errorf("leaf.Bytes (%d) should be well below follow.Bytes (%d)", leaf.Bytes, follow.Bytes)
	}
	if leaf.Bytes < 150 {
		t.Errorf("leaf.Bytes (%d) should include the two real files (150) + symlink target length", leaf.Bytes)
	}
}

// TestGenericScan_FileSymlinkDoesNotDoublePhysical guards against the
// bug that shipped briefly on this branch: the scanner's lazy-Stat
// fallback would resolve a symlink and add the target's blocks to
// PhysicalBytes — but the same target was already counted through its
// direct path, so every file-symlink inflated physical by its target's
// footprint.
//
// Uses a RELATIVE short symlink target so ext4's "fast symlink" path
// stores the link inside the inode (Blocks=0). That's the pathological
// case for the bug — Lstat leaves PhysicalSize at 0, the gate opens,
// and v.Stat then resolves the link and returns the target's blocks.
// A long (>60 char) symlink target would allocate a block for the
// link text (Blocks>0) and legitimately skip the gate.
func TestGenericScan_FileSymlinkDoesNotDoublePhysical(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "t.bin")
	// 64 KiB — big enough that a duplicate contribution is unmistakable
	// against the noise of a symlink's own inode blocks.
	const targetSize = 64 * 1024
	if err := os.WriteFile(target, make([]byte, targetSize), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewOSVFS(tmp)
	before, err := CalculateStatsWithOptions(context.Background(), v, tmp, []string{"."}, ScanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before.PhysicalBytes <= 0 {
		t.Skip("filesystem doesn't report per-file blocks (no PhysicalSize path)")
	}

	// Relative short target => fast symlink (Blocks=0 on ext4).
	if err := os.Symlink("t.bin", filepath.Join(tmp, "l")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	for _, follow := range []bool{true, false} {
		got, err := CalculateStatsWithOptions(context.Background(), v, tmp, []string{"."}, ScanOptions{FollowSymlinkDirs: follow}, nil)
		if err != nil {
			t.Fatalf("follow=%v: %v", follow, err)
		}
		delta := got.PhysicalBytes - before.PhysicalBytes
		if delta >= int64(targetSize) {
			t.Errorf("follow=%v: adding a link to a %d-byte target grew PhysicalBytes by %d — target was double-counted through the link",
				follow, targetSize, delta)
		}
	}
}

// TestGenericScan_HardLinkDedup checks that DedupInodes matches
// far2l's ScannedINodes behaviour: a file reachable via N hard
// links counts once, not N times, in every OpStats field.
//
// Layout:
//
//	root/
//	  original (8192)
//	  link1   (hard link -> original)
//	  link2   (hard link -> original)
func TestGenericScan_HardLinkDedup(t *testing.T) {
	tmp := t.TempDir()
	orig := filepath.Join(tmp, "original")
	if err := os.WriteFile(orig, make([]byte, 8192), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(orig, filepath.Join(tmp, "link1")); err != nil {
		t.Skipf("cannot create hard link: %v", err)
	}
	if err := os.Link(orig, filepath.Join(tmp, "link2")); err != nil {
		t.Skipf("cannot create hard link: %v", err)
	}

	v := NewOSVFS(tmp)

	// Without dedup: 3 files, 3× the bytes. Preserves historical
	// copy/move pre-scan behaviour.
	nodedup, err := CalculateStatsWithOptions(context.Background(), v, tmp, []string{"."}, ScanOptions{FollowSymlinkDirs: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if nodedup.Files != 3 {
		t.Errorf("no-dedup: Files=%d, want 3 (each hard link counted)", nodedup.Files)
	}
	if nodedup.Bytes != 3*8192 {
		t.Errorf("no-dedup: Bytes=%d, want %d (3×)", nodedup.Bytes, 3*8192)
	}

	// With dedup: 1 file, one contribution. Matches far2l's Ctrl+Q.
	dedup, err := CalculateStatsWithOptions(context.Background(), v, tmp, []string{"."}, ScanOptions{DedupInodes: true, FollowSymlinkDirs: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dedup.Files != 1 {
		t.Errorf("dedup: Files=%d, want 1 (three hard links to same inode counted once)", dedup.Files)
	}
	if dedup.Bytes != 8192 {
		t.Errorf("dedup: Bytes=%d, want 8192 (only original's bytes)", dedup.Bytes)
	}
}
