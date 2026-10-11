package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

// entryTotalsTestPanel builds a panel with a fixed, hand-picked set of
// entries, the same way groupTestPanel does for grouping tests: a NullVFS
// keeps ReadDirectory (called by NewFileSystemPanel itself) from touching a
// real filesystem, and the entries are then replaced directly so the test
// controls exactly what panelEntryTotals sees.
func entryTotalsTestPanel(t *testing.T, entries []*FileEntry) *FileSystemPanel {
	t.Helper()
	fp := NewFileSystemPanel(0, 0, 60, 16, vfs.NewNullVFS(0))
	fp.Entries = entries
	return fp
}

func sampleTotalsEntries() []*FileEntry {
	return []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "dir1", IsDir: true, Size: 999}}, // a dir's own Size never counts
		{VFSItem: vfs.VFSItem{Name: "a.txt", Size: 100}},
		{VFSItem: vfs.VFSItem{Name: "b.txt", Size: 250}},
	}
}

// TestPanelEntryTotalsCacheUnchangedEntries asserts that calling
// panelEntryTotals repeatedly for the same, unchanged entries only walks
// fp.Entries once -- the cache hit path a render frame that changed nothing
// about the directory should take, instead of re-summing every file on every
// single frame (f4#884's third occurrence of the pattern #1511 and #1536
// already fixed for the menu bar and the highlight rules).
func TestPanelEntryTotalsCacheUnchangedEntries(t *testing.T) {
	fp := entryTotalsTestPanel(t, sampleTotalsEntries())

	for i := 0; i < 5; i++ {
		totSize, totCount, totFiles, totDirs := fp.panelEntryTotals()
		if totSize != 350 || totCount != 3 || totFiles != 2 || totDirs != 1 {
			t.Fatalf("call %d: got size=%d count=%d files=%d dirs=%d, want size=350 count=3 files=2 dirs=1",
				i, totSize, totCount, totFiles, totDirs)
		}
	}

	if fp.entryTotalsComputeCount != 1 {
		t.Fatalf("entryTotalsComputeCount = %d, want 1 (only the first call should have rescanned fp.Entries)", fp.entryTotalsComputeCount)
	}
}

// TestPanelEntryTotalsCacheInvalidatesOnRefresh covers the one place a file's
// own Size changes after the entries were loaded without fp.Entries itself
// being replaced: a background "calculate directory size" scan landing on an
// entry (internal/app's actionCalcDirSize), which always calls fsp.Refresh()
// once it applies the result. The cached total must not keep serving the
// pre-scan number afterwards.
func TestPanelEntryTotalsCacheInvalidatesOnRefresh(t *testing.T) {
	entries := sampleTotalsEntries()
	fp := entryTotalsTestPanel(t, entries)

	if totSize, _, _, _ := fp.panelEntryTotals(); totSize != 350 {
		t.Fatalf("initial totSize = %d, want 350", totSize)
	}
	if fp.entryTotalsComputeCount != 1 {
		t.Fatalf("entryTotalsComputeCount after first call = %d, want 1", fp.entryTotalsComputeCount)
	}

	// Simulate actionCalcDirSize completing a recursive scan of dir1 (a
	// directory's own Size is never folded into the panel-wide totSize, only
	// its count as a directory is -- matched below) together with a file's
	// size actually growing, standing in for anything else that mutates an
	// entry in place before calling Refresh.
	entries[1].Size = 5000
	entries[1].SizeCalculated = true
	entries[2].Size = 1000

	if totSize, totCount, totFiles, totDirs := fp.panelEntryTotals(); totSize != 350 || totCount != 3 || totFiles != 2 || totDirs != 1 {
		t.Fatalf("stale read before Refresh: got size=%d count=%d files=%d dirs=%d, want the pre-mutation size=350 count=3 files=2 dirs=1 (cache must not have been invalidated yet)",
			totSize, totCount, totFiles, totDirs)
	}

	fp.Refresh()

	totSize, totCount, totFiles, totDirs := fp.panelEntryTotals()
	if totSize != 1250 || totCount != 3 || totFiles != 2 || totDirs != 1 {
		t.Fatalf("after Refresh: got size=%d count=%d files=%d dirs=%d, want size=1250 count=3 files=2 dirs=1",
			totSize, totCount, totFiles, totDirs)
	}
	if fp.entryTotalsComputeCount != 2 {
		t.Fatalf("entryTotalsComputeCount after Refresh = %d, want 2 (Refresh must invalidate the cache)", fp.entryTotalsComputeCount)
	}
}

// TestPanelEntryTotalsCacheInvalidatesOnSetEntries covers a real directory
// (re)load and an autofilter query narrowing the visible rows: both replace
// fp.Entries via setEntries/refilterEntries, and the cached total from the
// previous directory (or the previous, wider filter) must not leak into the
// new one.
func TestPanelEntryTotalsCacheInvalidatesOnSetEntries(t *testing.T) {
	fp := entryTotalsTestPanel(t, sampleTotalsEntries())

	if totSize, totCount, _, _ := fp.panelEntryTotals(); totSize != 350 || totCount != 3 {
		t.Fatalf("initial totals = (%d, %d), want (350, 3)", totSize, totCount)
	}

	fp.setEntries([]*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "only.txt", Size: 42}},
	})

	totSize, totCount, totFiles, totDirs := fp.panelEntryTotals()
	if totSize != 42 || totCount != 1 || totFiles != 1 || totDirs != 0 {
		t.Fatalf("after setEntries: got size=%d count=%d files=%d dirs=%d, want size=42 count=1 files=1 dirs=0",
			totSize, totCount, totFiles, totDirs)
	}
	if fp.entryTotalsComputeCount != 2 {
		t.Fatalf("entryTotalsComputeCount after setEntries = %d, want 2 (setEntries must invalidate the cache)", fp.entryTotalsComputeCount)
	}
}
