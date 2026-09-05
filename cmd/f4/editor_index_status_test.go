package main

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// pumpUntil drains UI tasks until cond holds, which is how these tests wait for
// work the indexer posts back to the UI thread.
func pumpUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !cond() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatalf("timeout waiting for %s", what)
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// A fully read file (non-UTF-8) has no scan at all — the restore must still
// resolve, or Loading stays on screen until a key press aborts it.
func TestIndexRestore_ResolvesForFullyReadFile(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	pt := piecetable.New([]byte("line one\nline two\nline three\nline four\nline five\n"))
	ev := newEditorView(pt, nil, "", false, true)
	ev.targetLine = 3
	ev.targetPos = 2
	ev.targetTopRow = 3
	ev.targetLeft = 0

	ev.StartIndexing()

	if ev.targetLine != -1 {
		t.Fatalf("StartIndexing on a fully read file must resolve the restore, targetLine = %d", ev.targetLine)
	}
	if ev.CursorLine != 3 || ev.CursorPos != 2 || ev.ScrollTopRow != 3 {
		t.Errorf("restore = line %d pos %d top %d, want 3, 2, 3", ev.CursorLine, ev.CursorPos, ev.ScrollTopRow)
	}
}

// TestIndexStatus_ReachesCompleteAndNotifies covers the state machine end to
// end: a scan announces itself, reports progress, and settles on complete,
// telling subscribers about every phase it passes through.
func TestIndexStatus_ReachesCompleteAndNotifies(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, _, _ := bigSearchCorpus()
	pt, buf := lazyEditorBuffer(t, content)
	ev := newEditorView(pt, nil, "", false, true)
	defer ev.Close()
	ev.asyncBuf = buf

	var phases []IndexPhase
	unsubscribe := ev.SubscribeIndex(func(s IndexStatus) {
		if len(phases) == 0 || phases[len(phases)-1] != s.Phase {
			phases = append(phases, s.Phase)
		}
	})
	defer unsubscribe()

	if got := ev.IndexState().Phase; got != IndexIdle {
		t.Fatalf("phase before indexing = %v, want idle", got)
	}

	ev.StartIndexing()
	if got := ev.IndexState().Phase; got != IndexScanning {
		t.Fatalf("phase after StartIndexing = %v, want scanning", got)
	}

	pumpUntil(t, "the index to complete", func() bool { return ev.indexIsComplete() })

	st := ev.IndexState()
	if st.Lines != strings.Count(content, "\n")+1 {
		t.Errorf("indexed %d lines, want %d", st.Lines, strings.Count(content, "\n")+1)
	}
	if st.Percent() != 100 {
		t.Errorf("percent = %d at completion, want 100", st.Percent())
	}
	if len(phases) < 2 || phases[0] != IndexScanning || phases[len(phases)-1] != IndexComplete {
		t.Errorf("phases seen = %v, want scanning through to complete", phases)
	}
}

// TestIndexStatus_ResumesAfterAnEdit is the behaviour that replaced cancelling
// the scan for good: an edit stops the run, and the index picks up from the
// line it had reached rather than staying short for the rest of the session.
func TestIndexStatus_ResumesAfterAnEdit(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, _, _ := bigSearchCorpus()
	pt, buf := lazyEditorBuffer(t, content)
	ev := newEditorView(pt, nil, "", false, true)
	ev.asyncBuf = buf

	ev.StartIndexing()
	pumpUntil(t, "the index to complete", func() bool { return ev.indexIsComplete() })
	before := ev.li.LineCount()

	// Interrupt it the way a keystroke does, then let the debounce fire.
	ev.noteBufferEdit()
	ev.pt.Insert(0, []byte("typed\n"))
	ev.li.UpdateAfterInsert(0, []byte("typed\n"))
	ev.setIndexStatus(IndexStatus{Phase: IndexIdle, Total: int64(ev.pt.Size())})

	pumpUntil(t, "the scan to resume and finish", func() bool { return ev.indexIsComplete() })

	if got, want := ev.li.LineCount(), before+1; got != want {
		t.Errorf("line count after the edit = %d, want %d", got, want)
	}
	if got := ev.IndexState().Lines; got != ev.li.LineCount() {
		t.Errorf("status reports %d lines, index holds %d", got, ev.li.LineCount())
	}
}

// TestEnsureIndexedTo_ResolvesAMatchPastTheScan covers the wrong-cursor bug: a
// search reads the whole buffer and can land on text the index has not reached,
// where asking for its line used to answer with the last line it knew and a
// column counted from there.
func TestEnsureIndexedTo_ResolvesAMatchPastTheScan(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	const lines = 4000
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		if _, err := sb.WriteString("a line of text\n"); err != nil {
			t.Fatal(err)
		}
	}
	content := sb.String()
	needleOff := len(content)
	content += "NEEDLE here\n"

	ev := newEditorView(piecetable.New([]byte(content)), nil, "", false, true)
	defer ev.Close()
	// An index that stopped early, which is what a scan in progress looks like.
	ev.li.Rebuild(piecetable.New([]byte(content[:1000])))
	ev.setIndexStatus(IndexStatus{Phase: IndexScanning, Total: int64(len(content))})

	shortLines := ev.li.LineCount()
	if shortLines >= lines {
		t.Fatalf("precondition: index should be short, holds %d lines", shortLines)
	}

	ev.selectFoundPattern(needleOff, len("NEEDLE"))

	if got, want := ev.CursorLine, lines; got != want {
		t.Errorf("cursor line = %d, want %d", got, want)
	}
	if got, want := ev.CursorPos, len("NEEDLE"); got != want {
		t.Errorf("cursor column = %d, want %d", got, want)
	}
	if ev.li.LineCount() <= shortLines {
		t.Error("the index was not extended to cover the match")
	}
}

// TestIndexRebuilt_ShortRebuildIsNotCalledComplete: Undo, Redo and SetText all
// rebuild the index from the piece table, and on a lazily loaded file that
// walk stops at the first chunk that has not arrived. Recording that as a
// complete index is the one answer that must not be given — every reader of
// the index believes it, so a search would report lines that are not there and
// nothing would ever go back for the rest.
func TestIndexRebuilt_ShortRebuildIsNotCalledComplete(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, _, _ := bigSearchCorpus()
	pt, buf := lazyEditorBuffer(t, content)
	ev := newEditorView(pt, nil, "", false, false)
	ev.asyncBuf = buf
	t.Cleanup(func() { ev.Close() })

	// Only the prewarmed head is in hand, so the rebuild cannot finish.
	if complete := ev.li.Rebuild(ev.pt); complete {
		t.Skip("the whole buffer happened to be loaded; nothing to assert")
	}
	ev.noteIndexRebuilt(false)

	if ev.indexIsComplete() {
		t.Error("a rebuild that stopped short was recorded as a complete index")
	}
	if st := ev.IndexState(); st.Lines != ev.li.LineCount() {
		t.Errorf("status reports %d lines, index holds %d", st.Lines, ev.li.LineCount())
	}
	// And the scan it handed the rest to finishes the job.
	pumpUntil(t, "the scan to finish what the rebuild could not", func() bool { return ev.indexIsComplete() })
	if got, want := ev.li.LineCount(), strings.Count(content, "\n")+1; got != want {
		t.Errorf("index holds %d lines after the scan, want %d", got, want)
	}
}

// TestAwaitOffset_ScanPlacesThePositionItCouldNotResolve: switching in from
// the viewer, and leaving hex mode, both know where the cursor belongs only as
// a byte offset. On a file the index has not reached, that offset cannot be
// turned into a line yet — and the answer used to be the last line the index
// knew, with a column counted from there, which on a large file is a column
// measured in gigabytes. The scan places it instead, when it reads past.
func TestAwaitOffset_ScanPlacesThePositionItCouldNotResolve(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, _, _ := bigSearchCorpus()
	pt, buf := lazyEditorBuffer(t, content)
	ev := newEditorView(pt, nil, "", false, false)
	ev.asyncBuf = buf
	t.Cleanup(func() { ev.Close() })

	// A byte well past anything a prewarmed head can describe.
	target := len(content) - len(content)/4
	wanted := strings.Count(content[:target], "\n")

	if !ev.awaitOffset(target) {
		t.Fatal("precondition: the index should not be able to place that offset yet")
	}
	if ev.CursorLine != 0 || ev.CursorPos != 0 {
		t.Errorf("while waiting the cursor sits at %d,%d; want the top of the file",
			ev.CursorLine, ev.CursorPos)
	}

	pumpUntil(t, "the scan to place the waiting position", func() bool { return ev.targetOffset < 0 })

	if got := ev.CursorLine; got != wanted {
		t.Errorf("cursor landed on line %d, want %d", got, wanted)
	}
	if got, want := ev.li.GetLineOffset(ev.CursorLine)+ev.CursorPos, target; got != want {
		t.Errorf("cursor is at byte %d, want %d", got, want)
	}
}
