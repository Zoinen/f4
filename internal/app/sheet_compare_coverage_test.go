package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sheet"
	"github.com/unxed/f4/vfs"
)

func TestSheetFileHelpersRoundTripCSVAndText(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.csv")
	if err := os.WriteFile(input, []byte("name,value\nalpha,42\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	doc, err := loadSheetCSV(input)
	if err != nil {
		t.Fatalf("loadSheetCSV() error = %v", err)
	}
	if got := doc.Cell(0, 1).DisplayText(); got != "alpha" {
		t.Fatalf("loaded text = %q, want alpha", got)
	}
	if got := doc.Cell(1, 1).DisplayText(); got != "42" {
		t.Fatalf("loaded value = %q, want 42", got)
	}

	csvPath := filepath.Join(dir, "out.csv")
	if err := exportSheetCSV(doc, csvPath); err != nil {
		t.Fatalf("exportSheetCSV() error = %v", err)
	}
	csvBytes, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(csvBytes); got != "name,value\nalpha,42\n" {
		t.Fatalf("CSV output = %q", got)
	}

	textPath := filepath.Join(dir, "out.txt")
	if err := exportSheetText(doc, textPath); err != nil {
		t.Fatalf("exportSheetText() error = %v", err)
	}
	textBytes, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(textBytes), "name") || !strings.Contains(string(textBytes), "alpha") {
		t.Fatalf("text output = %q", textBytes)
	}

	if _, err := loadSheetCSV(filepath.Join(dir, "missing.csv")); err == nil {
		t.Fatal("loadSheetCSV() accepted a missing file")
	}
	missingDir := filepath.Join(dir, "missing")
	if err := exportSheetCSV(doc, filepath.Join(missingDir, "out.csv")); err == nil {
		t.Fatal("exportSheetCSV() accepted an unwritable path")
	}
	if err := exportSheetText(doc, filepath.Join(missingDir, "out.txt")); err == nil {
		t.Fatal("exportSheetText() accepted an unwritable path")
	}
}

func TestSheetExportNamesAndMenuEntries(t *testing.T) {
	newSheetFrameForTest(t)
	withPath := &SheetFrame{path: filepath.Join("reports", "book.f4s.sqlite")}
	if got := defaultExportName(withPath, ".csv"); got != filepath.Join("reports", "book.csv") {
		t.Fatalf("defaultExportName(native) = %q", got)
	}
	plain := &SheetFrame{path: "book.xlsx"}
	if got := defaultExportName(plain, ".txt"); got != "book.txt" {
		t.Fatalf("defaultExportName(plain) = %q", got)
	}
	empty := &SheetFrame{}
	if got := defaultExportName(empty, ".csv"); !filepath.IsAbs(got) || filepath.Base(got) != "sheet.csv" {
		t.Fatalf("defaultExportName(empty) = %q, want absolute sheet.csv", got)
	}

	entries := sheetMenuEntries()
	if len(entries) != 23 {
		t.Fatalf("sheetMenuEntries() returned %d entries, want 23", len(entries))
	}
	for i, entry := range entries {
		if entry.label == "" || entry.run == nil {
			t.Errorf("entry %d is incomplete: %#v", i, entry)
		}
	}
}

func TestSheetSearchHelpersFindReplaceAgainAndNotFound(t *testing.T) {
	sf := newSheetFrameForTest(t)
	sf.Document().SetText(0, 0, "alpha beta")
	sf.Document().SetText(0, 1, "target")

	sf.search = sheet.SearchOptions{Pattern: "beta"}
	sheetSearchNext(sf, sheet.Point{})
	if got := sf.Cursor(); got != (sheet.Point{}) {
		t.Fatalf("search cursor = %#v, want first matching cell", got)
	}
	if !strings.Contains(sf.status, "A1") {
		t.Fatalf("search status = %q, want a found-cell message", sf.status)
	}

	sf.search = sheet.SearchOptions{Pattern: "alpha"}
	sf.replace = "omega"
	sf.replacing = true
	sheetSearchNext(sf, sheet.Point{})
	if got := sf.Document().Cell(0, 0).Text; got != "omega beta" {
		t.Fatalf("replaced cell = %q", got)
	}
	if !strings.Contains(sf.status, i18n.Msg("Sheet.Replaced")) {
		t.Fatalf("replace status = %q", sf.status)
	}

	sf.replacing = false
	sf.search = sheet.SearchOptions{Pattern: "target"}
	sf.cur = sheet.Point{}
	sheetSearchAgain(sf)
	if got := sf.Cursor(); got != (sheet.Point{Row: 1}) {
		t.Fatalf("search-again cursor = %#v, want row 1", got)
	}

	sf.search = sheet.SearchOptions{Pattern: "missing"}
	sheetSearchNext(sf, sheet.Point{})
	if !strings.Contains(sf.status, i18n.Msg("Sheet.NotFound")) {
		t.Fatalf("not-found status = %q", sf.status)
	}
}

func TestComparePanelSnapshotCapturesMarkedStateAndStaleness(t *testing.T) {
	if _, ok := captureComparePanel(nil, config.CompareOptions{}); ok {
		t.Fatal("nil panel was captured")
	}
	nilVFS := &panel.FileSystemPanel{}
	if _, ok := captureComparePanel(nilVFS, config.CompareOptions{}); ok {
		t.Fatal("panel without VFS was captured")
	}

	dir := t.TempDir()
	fsp := &panel.FileSystemPanel{
		Vfs:            vfs.NewOSVFS(dir),
		DirectoryEpoch: 7,
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "marked.txt"}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "other.txt"}},
		},
	}
	snap, ok := captureComparePanel(fsp, config.CompareOptions{MarkedOnly: true})
	if !ok || len(snap.allow) != 1 || !snap.allow["marked.txt"] {
		t.Fatalf("marked snapshot = %#v, ok=%v", snap, ok)
	}
	if !snap.stillCurrent() {
		t.Fatal("fresh snapshot is not current")
	}
	fsp.DirectoryEpoch++
	if snap.stillCurrent() {
		t.Fatal("snapshot remained current after directory epoch changed")
	}

	emptyMarks := &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(dir)}
	empty, ok := captureComparePanel(emptyMarks, config.CompareOptions{MarkedOnly: true})
	if !ok || empty.allow != nil {
		t.Fatalf("empty marked snapshot = %#v, ok=%v", empty, ok)
	}
}

func TestComparePanelSnapshotAppliesMarksAndRejectsDifferentVFS(t *testing.T) {
	dir := t.TempDir()
	fsp := &panel.FileSystemPanel{
		Vfs: vfs.NewOSVFS(dir),
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "old.txt"}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "new.txt"}},
		},
	}
	snap := comparePanelSnapshot{pnl: fsp, fs: fsp.Vfs, root: dir}
	snap.applyCompareMarks(map[string]bool{"new.txt": true})
	if fsp.Entries[0].Selected || !fsp.Entries[1].Selected {
		t.Fatalf("applied marks = old:%v new:%v", fsp.Entries[0].Selected, fsp.Entries[1].Selected)
	}
	if !fsp.Entries[0].PrevSelected || fsp.Entries[1].PrevSelected {
		t.Fatalf("previous selection was not saved: old:%v new:%v", fsp.Entries[0].PrevSelected, fsp.Entries[1].PrevSelected)
	}

	other := vfs.NewOSVFS(dir)
	snap.fs = other
	if snap.stillCurrent() {
		t.Fatal("snapshot accepted a different VFS instance")
	}
	comparePanelSnapshot{}.applyCompareMarks(nil)
}
