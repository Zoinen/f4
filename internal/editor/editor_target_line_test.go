package editor

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testutil.DrainPendingTasks empties the frame manager queue without running anything.
// Tests in this package share one global queue, so a test that counts what it
// posted has to start from an empty one.
// collectQueuedTasks receives everything handed to the UI thread without
// running any of it, until nothing new has arrived for idle. Taking a task off
// the channel is what releases the goroutine that posted it, so a background
// scan runs to its end while its batches pile up here — which is the ordering a
// burst of FISH+ chunks produces on a real session. Counting what is queued
// instead would assume the queue is buffered, and it is not.
func collectQueuedTasks(idle time.Duration) []func() {
	var tasks []func()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case task := <-vtui.FrameManager.PriorityTaskChan:
			tasks = append(tasks, task)
			timer.Reset(idle)
		case task := <-vtui.FrameManager.TaskChan:
			tasks = append(tasks, task)
			timer.Reset(idle)
		case <-timer.C:
			return tasks
		}
	}
}

// TestEditorView_IndexerRestoresTargetLineAfterLateDrain covers the FISH+ bug
// where reopening a file at a saved position landed somewhere else entirely.
//
// The whole file sits in one already loaded chunk, so the indexing goroutine
// runs to the end of it without ever needing the UI thread again. Every batch
// it posted is therefore executed after the scan is over, which is what
// happens over FISH+ once a burst of chunks has been stored: the batches are
// drained in one pass while the goroutine is already finished.
func TestEditorView_IndexerRestoresTargetLineAfterLateDrain(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	// 16384 lines of 8 bytes each: two 64 KB indexer reads, so the saved line
	// can sit in the second batch and be invisible to the first.
	const lineCount = 16384
	const TargetLine = 12000

	var sb strings.Builder
	for i := 0; i < lineCount; i++ {
		if _, err := fmt.Fprintf(&sb, "L%06d\n", i); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "restore.txt")
	if err := os.WriteFile(path, []byte(sb.String()), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// No prewarm on purpose: NewEditorView then fails to build the index up
	// front, exactly as it does on a file whose first chunk is still on the
	// wire, and the whole index has to come from the background scan.
	buf := NewAsyncBuffer(context.Background(), f)
	pt := piecetable.NewWithBuffer(buf)
	ev := NewEditorView(pt, v, path)
	defer ev.Close()
	ev.AsyncBuf = buf
	ev.File = f

	if ev.Li.LineCount() != 1 {
		t.Fatalf("index should still be empty, got %d lines", ev.Li.LineCount())
	}

	// Let the chunk land. After this the scan needs nothing from the UI thread.
	deadline := time.After(5 * time.Second)
	for {
		if _, err := buf.Read(0, 1); err == nil {
			break
		}
		select {
		case task := <-vtui.FrameManager.PriorityTaskChan:
			task()
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the first chunk never arrived")
		}
	}
	testutil.DrainPendingTasks()

	ev.TargetLine = TargetLine
	ev.TargetPos = 0
	ev.TargetTopRow = TargetLine
	ev.TargetLeft = 0

	ev.StartIndexing()

	// Hold the first publication instead of applying it immediately. Indexing
	// must not fill the input queue with the rest of the file in the meantime.
	tasks := collectQueuedTasks(300 * time.Millisecond)
	// The immutable index publication is one batch. A separate completion
	// callback may arrive in the same idle window to clear the worker's
	// indexing flag; it is bookkeeping, not another batch of offsets.
	if len(tasks) < 1 || len(tasks) > 2 {
		t.Fatalf("the indexer queued %d tasks, want one batch plus completion bookkeeping", len(tasks))
	}

	for _, task := range tasks {
		task()
	}
	deadline = time.After(5 * time.Second)
	for ev.TargetLine != -1 {
		select {
		case task := <-vtui.FrameManager.PriorityTaskChan:
			task()
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the saved position was never applied")
		}
	}

	if ev.CursorLine != TargetLine {
		t.Fatalf("cursor restored to line %d, want %d (index held %d lines at the end)",
			ev.CursorLine, TargetLine, ev.Li.LineCount())
	}
}

// TestEditorView_IndexerAppliesTargetLineWhenFileShrank keeps the clamp
// honest: a file that really is shorter than the saved line must still open,
// at its last line.
func TestEditorView_IndexerAppliesTargetLineWhenFileShrank(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	content := "one\ntwo\nthree\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "shrank.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	buf := NewAsyncBuffer(context.Background(), f)
	Pt := piecetable.NewWithBuffer(buf)
	ev := NewEditorView(Pt, v, path)
	defer ev.Close()
	ev.AsyncBuf = buf
	ev.File = f

	ev.TargetLine = 500
	ev.TargetPos = 0
	ev.TargetTopRow = 500
	ev.TargetLeft = 0

	ev.StartIndexing()

	deadline := time.After(5 * time.Second)
	for ev.TargetLine != -1 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the saved position was never applied to a shorter file")
		}
	}

	if want := ev.Li.LineCount() - 1; ev.CursorLine != want {
		t.Fatalf("cursor at line %d, want the last line %d", ev.CursorLine, want)
	}
}

func DrainPendingTasks() {
	for {
		select {
		case <-vtui.FrameManager.PriorityTaskChan:
			continue
		case <-vtui.FrameManager.TaskChan:
			continue
		default:
			return
		}
	}
}

func TestEditorView_InMemoryStartIndexingAppliesSavedPosition(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte("zero\none\ntwo\n")), nil, "small.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 79, 24)
	ev.SetVisible(true)

	ev.TargetLine = 2
	ev.TargetPos = 1
	ev.TargetTopRow = 1
	ev.TargetLeft = 0

	ev.StartIndexing()

	if ev.TargetLine != -1 {
		t.Fatalf("in-memory saved position remained pending: targetLine=%d", ev.TargetLine)
	}
	if ev.CursorLine != 2 || ev.CursorPos != 1 {
		t.Fatalf("cursor restored to %d:%d, want 2:1", ev.CursorLine, ev.CursorPos)
	}

	window := ev.semanticWindow()
	rows := semanticStyledEditorWindowRows(ev, window, ev.semanticSurfaceWidth())
	var rendered strings.Builder
	for _, row := range rows {
		for _, run := range row.Runs {
			rendered.WriteString(run.Text)
		}
	}
	if strings.Contains(rendered.String(), "Loading...") {
		t.Fatalf("resolved in-memory editor still rendered a loading placeholder: %q", rendered.String())
	}
}
