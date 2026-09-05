package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// openMappedEditor opens a local file the way showEditor does for UTF-8 with
// mapping available, and fails if the mapping did not happen — these tests are
// about what the mapped path does, not about the fallback.
//
// The index is left empty, as showEditor leaves it: a mapped file is indexed by
// the background scan, not on the way in.
func openMappedEditor(t *testing.T, dir, path string) *EditorView {
	t.Helper()

	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mapped, err := MapEditorFile(v, f)
	if err != nil {
		t.Fatalf("MapEditorFile: %v", err)
	}

	ev := NewEditorViewIndexedLater(piecetable.New(mapped.Bytes()), v, path)
	ev.file = f
	ev.mapped = mapped
	ev.Codepage = 65001
	return ev
}

// TestMappedEditor_SearchAllocatesNothing is the whole point of the mapping:
// with the file itself as the piece table's original buffer, a search pass has
// nothing left to assemble. Without it, the first search over a lazily loaded
// buffer copies the file.
func TestMappedEditor_SearchAllocatesNothing(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	content := strings.Repeat("the quick brown fox\n", 200000) // ~4 MB
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openMappedEditor(t, dir, path)
	defer ev.Close()

	// Warm anything the first pass builds once.
	if _, err := ev.searchBuffer(nil, ev.editSession); err != nil {
		t.Fatalf("searchBuffer: %v", err)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	data, err := ev.searchBuffer(nil, ev.editSession)
	if err != nil {
		t.Fatalf("searchBuffer: %v", err)
	}
	off, _, err := findMatch(data, "quick", true, false, false, false, true, 0)
	if err != nil {
		t.Fatalf("findMatch: %v", err)
	}
	runtime.ReadMemStats(&after)

	if off != 4 {
		t.Errorf("match at %d, want 4", off)
	}
	if len(data) != len(content) {
		t.Fatalf("buffer length = %d, want %d", len(data), len(content))
	}
	if &data[0] != &ev.mapped.Bytes()[0] {
		t.Error("the search buffer is a copy, not the mapping")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 64*1024 {
		t.Errorf("one search pass allocated %d bytes over a %d byte file", allocated, len(content))
	}
}

// TestMappedEditor_IndexesWithoutAnAsyncBuffer covers the indexer's other
// source: it used to return immediately unless there was a chunk buffer to
// read from, which would have left a mapped file with no line index at all.
func TestMappedEditor_IndexesWithoutAnAsyncBuffer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	const lines = 5000
	if err := os.WriteFile(path, []byte(strings.Repeat("a line of text\n", lines)), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openMappedEditor(t, dir, path)
	defer ev.Close()
	if ev.asyncBuf != nil {
		t.Fatal("precondition: a mapped editor has no chunk buffer")
	}

	ev.StartIndexing()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for ev.li.LineCount() < lines+1 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatalf("timeout: indexed %d of %d lines", ev.li.LineCount(), lines+1)
		}
	}
	if got := ev.li.LineCount(); got != lines+1 {
		t.Errorf("line count = %d, want %d", got, lines+1)
	}
}

// TestMappedEditor_EditsAndSaves keeps the mapping honest about being the
// original buffer only: edits land in the add buffer, and the save reads the
// unchanged pieces back through the mapping.
func TestMappedEditor_EditsAndSaves(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openMappedEditor(t, dir, path)
	defer ev.Close()

	ev.insertTextAtCursor([]byte("XYZ "))
	ev.modified = true

	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	drainPendingTasks()

	if ev.modified {
		t.Error("editor still marked modified: the save reported failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "XYZ hello world\n" {
		t.Errorf("content = %q, want %q", got, "XYZ hello world\n")
	}
	// The save reopens the file, and a mapped editor stays mapped afterwards
	// rather than dropping to the chunk buffer.
	if ev.mapped == nil {
		t.Error("the editor lost its mapping across a save")
	}
	assertNoEditorTempSiblings(t, path)
}

// countingReadAtCloser records how much the editor asked the file for, so a
// test can tell reading the file apart from walking the mapping.
type countingReadAtCloser struct {
	vfs.ReadAtCloser
	mu    sync.Mutex
	calls int
	bytes int64
}

func (c *countingReadAtCloser) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	n, err := c.ReadAtCloser.ReadAt(ctx, p, off)
	c.mu.Lock()
	c.calls++
	c.bytes += int64(n)
	c.mu.Unlock()
	return n, err
}

func (c *countingReadAtCloser) counted() (int, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, c.bytes
}

// TestMappedEditor_IndexReadsTheFile is the difference between opening an 8 GB
// file in twenty seconds and opening it in four: the scan asks the file for
// megabytes at a time instead of faulting the mapping in a page at a time.
func TestMappedEditor_IndexReadsTheFile(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	const lines = 20000
	if err := os.WriteFile(path, []byte(strings.Repeat("a line of text\n", lines)), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openMappedEditor(t, dir, path)
	defer ev.Close()
	counter := &countingReadAtCloser{ReadAtCloser: ev.file}
	ev.file = counter

	ev.StartIndexing()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for ev.li.LineCount() < lines+1 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatalf("timeout: indexed %d of %d lines", ev.li.LineCount(), lines+1)
		}
	}

	calls, read := counter.counted()
	if calls == 0 {
		t.Fatal("the scan walked the mapping instead of reading the file")
	}
	if want := int64(lines * len("a line of text\n")); read < want {
		t.Errorf("the scan read %d bytes of a %d byte file", read, want)
	}
	for i := 0; i < 5; i++ {
		if got := ev.li.GetLineOffset(i); got != i*len("a line of text\n") {
			t.Errorf("line %d starts at %d, want %d", i, got, i*len("a line of text\n"))
		}
	}
}

// TestMappedEditor_EditedTextIsScannedFromTheBuffer keeps the shortcut honest:
// once something has been typed, the bytes on disk are no longer the bytes of
// the buffer, and the scan has to go back to reading the piece table for them.
func TestMappedEditor_EditedTextIsScannedFromTheBuffer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "edited.txt")
	const lines = 5000
	if err := os.WriteFile(path, []byte(strings.Repeat("original line\n", lines)), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openMappedEditor(t, dir, path)
	defer ev.Close()
	ev.StartIndexing()
	pumpUntil(t, "the first scan to finish", func() bool { return ev.indexIsComplete() })

	// Three lines that are in no file, spliced into the front of the buffer,
	// the way a keystroke splices one.
	typed := []byte("typed\ntyped\ntyped\n")
	ev.noteBufferEdit()
	ev.pt.Insert(0, typed)
	ev.li.UpdateAfterInsert(0, typed)
	ev.setIndexStatus(IndexStatus{Phase: IndexIdle, Total: int64(ev.pt.Size())})

	ev.StartIndexing()
	pumpUntil(t, "the scan to resume and finish", func() bool { return ev.indexIsComplete() })

	// The index has to describe the buffer, not the file it came from: nothing
	// on disk starts with the typed lines, and every line after them has moved.
	reference := piecetable.NewLineIndex()
	reference.Rebuild(ev.pt)
	if got, want := ev.li.LineCount(), reference.LineCount(); got != want {
		t.Fatalf("line count after the edit = %d, want %d", got, want)
	}
	for line := 0; line < reference.LineCount(); line++ {
		if got, want := ev.li.GetLineOffset(line), reference.GetLineOffset(line); got != want {
			t.Fatalf("line %d starts at %d, want %d", line, got, want)
		}
	}
	if got := ev.li.LineCount(); got != lines+3+1 {
		t.Errorf("line count = %d, want %d", got, lines+3+1)
	}
}

// TestNewEditorViewIndexedLater_LeavesTheIndexToTheScan: the editor for a file
// the indexer owns must appear without reading the file, which is the whole
// point of not building its index on the way in.
func TestNewEditorViewIndexedLater_LeavesTheIndexToTheScan(t *testing.T) {
	content := []byte(strings.Repeat("a line\n", 1000))

	later := NewEditorViewIndexedLater(piecetable.New(content), nil, "")
	if got := later.li.LineCount(); got != 1 {
		t.Errorf("index has %d lines before the scan, want 1", got)
	}

	eager := NewEditorView(piecetable.New(content), nil, "")
	if got := eager.li.LineCount(); got != 1001 {
		t.Errorf("an ordinary editor indexed %d lines, want 1001", got)
	}
}

// TestMappedEditor_SwappedBufferIsNotReadFromTheFile is the invariant behind
// reading the file instead of the mapping: a position in the piece table's
// original buffer is a position in the file only while that buffer *is* the
// mapping. Reloading in another codepage, or a Replace All, puts a buffer
// built in memory in its place and leaves the mapping and the descriptor where
// they were — and reading the file at the new buffer's offsets would hand back
// line starts that fall in the middle of lines, with nothing to show anything
// had gone wrong.
func TestMappedEditor_SwappedBufferIsNotReadFromTheFile(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "reload.txt")
	content := strings.Repeat("a line of the file\n", 500)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openMappedEditor(t, dir, path)
	defer ev.Close()

	dst := make([]byte, 64)
	const line = "a line of the file\n"
	n, ok := ev.chunkReader()(context.Background(), dst, len(line), len(line))
	if !ok {
		t.Fatal("a mapped file should be read from the file")
	}
	if got, want := string(dst[:n]), line; got != want {
		t.Fatalf("read %q from the file, want %q", got, want)
	}

	// The editor now holds text that is not what the file holds.
	ev.SetText(strings.Repeat("text that came from somewhere else entirely\n", 500))
	if ev.mapped == nil || ev.file == nil {
		t.Fatal("precondition: the mapping and the descriptor outlive the swap")
	}
	if _, ok := ev.chunkReader()(context.Background(), dst, len(line), len(line)); ok {
		t.Error("the file was read for a buffer it no longer describes")
	}
}
