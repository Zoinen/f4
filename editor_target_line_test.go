package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// drainPendingTasks empties the frame manager queue without running anything.
// Tests in this package share one global queue, so a test that counts what it
// posted has to start from an empty one.
func drainPendingTasks() {
	for {
		select {
		case <-vtui.FrameManager.TaskChan:
		default:
			return
		}
	}
}

// collectQueuedTasks receives everything handed to the UI thread without
// running any of it, until nothing new has arrived for idle. Taking a task off
// the channel is what releases the goroutine that posted it, so a background
// scan runs to its end while its batches pile up here — which is the ordering a
// burst of FISH+ chunks produces on a real session. Counting what is queued
// instead would assume the queue is buffered, and it is not.
func collectQueuedTasks(idle time.Duration) []func() {
	var tasks []func()
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			tasks = append(tasks, task)
		case <-time.After(idle):
			return tasks
		}
	}
}

func TestEditorView_InMemoryStartIndexingAppliesSavedPosition(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte("zero\none\ntwo\n")), nil, "small.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 79, 24)
	ev.SetVisible(true)

	ev.targetLine = 2
	ev.targetPos = 1
	ev.targetTopRow = 1
	ev.targetLeft = 0

	ev.StartIndexing()

	if ev.targetLine != -1 {
		t.Fatalf("in-memory saved position remained pending: targetLine=%d", ev.targetLine)
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

// TestEditorView_IndexerRestoresTargetLineAfterLateDrain covers the FISH+ bug
// where reopening a file at a saved position landed somewhere else entirely.
//
// The whole file sits in one already loaded chunk, so the indexing goroutine
// runs to the end of it without ever needing the UI thread again. Every batch
// it posted is therefore executed after the scan is over, which is what
// happens over FISH+ once a burst of chunks has been stored: the batches are
// drained in one pass while the goroutine is already finished.
func TestEditorView_IndexerRestoresTargetLineAfterLateDrain(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	// 16384 lines of 8 bytes each: two 64 KB indexer reads, so the saved line
	// can sit in the second batch and be invisible to the first.
	const lineCount = 16384
	const targetLine = 12000

	var sb strings.Builder
	for i := 0; i < lineCount; i++ {
		fmt.Fprintf(&sb, "L%06d\n", i)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "restore.txt")
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
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
	ev.asyncBuf = buf
	ev.file = f

	if ev.li.LineCount() != 1 {
		t.Fatalf("index should still be empty, got %d lines", ev.li.LineCount())
	}

	// Let the chunk land. After this the scan needs nothing from the UI thread.
	deadline := time.After(5 * time.Second)
	for {
		if _, err := buf.Read(0, 1); err == nil {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the first chunk never arrived")
		}
	}
	drainPendingTasks()

	ev.targetLine = targetLine
	ev.targetPos = 0
	ev.targetTopRow = targetLine
	ev.targetLeft = 0

	ev.StartIndexing()

	// Two batches queued means the scan has reached the end of the file.
	tasks := collectQueuedTasks(300 * time.Millisecond)
	if len(tasks) < 2 {
		t.Fatalf("the indexer posted %d batches, expected at least 2", len(tasks))
	}

	for _, task := range tasks {
		task()
	}
	if ev.targetLine != -1 {
		t.Fatal("the saved position was never applied")
	}

	if ev.CursorLine != targetLine {
		t.Fatalf("cursor restored to line %d, want %d (index held %d lines at the end)",
			ev.CursorLine, targetLine, ev.li.LineCount())
	}
}

// TestEditorView_IndexerAppliesTargetLineWhenFileShrank keeps the clamp
// honest: a file that really is shorter than the saved line must still open,
// at its last line.
func TestEditorView_IndexerAppliesTargetLineWhenFileShrank(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content := "one\ntwo\nthree\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "shrank.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	buf := NewAsyncBuffer(context.Background(), f)
	pt := piecetable.NewWithBuffer(buf)
	ev := NewEditorView(pt, v, path)
	defer ev.Close()
	ev.asyncBuf = buf
	ev.file = f

	ev.targetLine = 500
	ev.targetPos = 0
	ev.targetTopRow = 500
	ev.targetLeft = 0

	ev.StartIndexing()

	deadline := time.After(5 * time.Second)
	for ev.targetLine != -1 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the saved position was never applied to a shorter file")
		}
	}

	if want := ev.li.LineCount() - 1; ev.CursorLine != want {
		t.Fatalf("cursor at line %d, want the last line %d", ev.CursorLine, want)
	}
}
