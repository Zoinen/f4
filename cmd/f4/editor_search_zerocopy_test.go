package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// The searches in this file are about how the buffer they scan is obtained,
// not about what they find: every pass used to assemble a fresh copy of the
// whole text, so a large file paid its own size in allocation per F7 and per
// confirmation of an interactive Replace.

func TestPieceTableView_AliasesBufferAndStopsAtPieceBoundary(t *testing.T) {
	text := []byte("hello world\nsecond line\n")
	pt := piecetable.New(text)

	data, ok := pt.View(0, pt.Size())
	if !ok {
		t.Fatal("View over an unedited memory buffer must succeed")
	}
	if len(data) != len(text) {
		t.Fatalf("View length = %d, want %d", len(data), len(text))
	}
	// Identity, not equality: the point is that nothing was copied.
	if &data[0] != &text[0] {
		t.Error("View returned a copy instead of a window into the buffer")
	}

	// An edit splits the original piece, so a whole-buffer window no longer
	// exists and callers must fall back to GetRange.
	pt.Insert(5, []byte("XX"))
	if _, ok := pt.View(0, pt.Size()); ok {
		t.Error("View must fail once the range crosses a piece boundary")
	}
	// A range inside one piece still works.
	if _, ok := pt.View(0, 4); !ok {
		t.Error("View inside a single piece must still succeed")
	}
}

func TestSearchBuffer_ScansMemoryBufferInPlace(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	content := strings.Repeat("the quick brown fox\n", 200000) // ~4 MB
	ev := newEditorView(piecetable.New([]byte(content)), nil, "", false, true)

	// Warm anything lazily built on the first pass so the measurement below
	// sees the steady state.
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
	allocated := after.TotalAlloc - before.TotalAlloc

	if off != 4 {
		t.Errorf("match at %d, want 4", off)
	}
	if len(data) != len(content) {
		t.Fatalf("buffer length = %d, want %d", len(data), len(content))
	}
	// The whole point: no proportional-to-file allocation. A small constant
	// of bookkeeping is fine; a copy of the text is not.
	if allocated > 64*1024 {
		t.Errorf("one search pass allocated %d bytes over a %d byte buffer; "+
			"it should scan in place", allocated, len(content))
	}
}

func TestSearchBuffer_ReusesSnapshotUntilTheTextChanges(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	// A lazily loaded buffer cannot be scanned in place, so this is the path
	// that assembles a snapshot — and used to reassemble it for every pass.
	content, _, _ := bigSearchCorpus()
	pt, _ := lazyEditorBuffer(t, content)
	ev := newEditorView(pt, nil, "", false, true)

	stop := pumpUITasks()
	defer stop()

	first, err := ev.searchBuffer(nil, ev.editSession)
	if err != nil {
		t.Fatalf("first searchBuffer: %v", err)
	}
	second, err := ev.searchBuffer(nil, ev.editSession)
	if err != nil {
		t.Fatalf("second searchBuffer: %v", err)
	}
	if &first[0] != &second[0] {
		t.Error("a second pass over unchanged text reassembled the buffer")
	}

	// An edit retires the session, and with it the cached buffer.
	ev.retireEditSession()
	ev.searchSnapMu.Lock()
	cached := ev.searchSnapshot
	ev.searchSnapMu.Unlock()
	if cached != nil {
		t.Error("retiring the edit session must release the cached buffer")
	}

	third, err := ev.searchBuffer(nil, ev.editSession)
	if err != nil {
		t.Fatalf("third searchBuffer: %v", err)
	}
	if len(third) != len(content) {
		t.Fatalf("rebuilt buffer length = %d, want %d", len(third), len(content))
	}
	if &third[0] == &first[0] {
		t.Error("the buffer from the retired session was handed out again")
	}
}
