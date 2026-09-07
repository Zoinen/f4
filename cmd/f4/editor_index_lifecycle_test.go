package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/textlayout"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func indexLifecycleEditor(t *testing.T, data []byte) *EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()
	reader := &documentRangeReader{data: data}
	buf := NewAsyncBuffer(context.Background(), reader)
	buf.seedPrefix(data)
	ev := NewEditorView(piecetable.NewWithBuffer(buf), nil, "")
	ev.asyncBuf = buf
	ev.li = piecetable.NewLineIndex()
	ev.engine = textlayout.NewWrapEngine(ev.pt, ev.li)
	ev.nativeViewportColumns, ev.nativeViewportRows = 2, 1
	ev.WordWrap = true
	ev.ensureEngineWidth()
	t.Cleanup(func() {
		if ev.indexCancel != nil {
			ev.indexCancel()
		}
		for ev.indexing {
			applyNextIndexTask(t)
		}
		ev.Close()
	})
	return ev
}

func applyNextIndexTask(t *testing.T) {
	t.Helper()
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("index batch was not published")
	}
}

func TestEditorIndexBatchInvalidatesTailBeforeRestoringWrappedTarget(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "remote"}[remote], func(t *testing.T) {
			ev := indexLifecycleEditor(t, []byte("abcdefgh\nxy\nz"))
			if remote {
				ev.vfs = &indexLifecycleRemote{offsets: []int64{9, 12}, total: 3}
			}
			// The first viewport can lay out the first logical line while the newline
			// index still has only its initial offset. That cached tail is provisional.
			ev.engine.GetFragments(0)
			if rows, complete := ev.engine.KnownVisualRows(); rows != 4 || complete {
				t.Fatalf("test requires a provisional wrapped tail: %d, %v", rows, complete)
			}
			ev.targetLine, ev.targetPos, ev.targetTopRow, ev.targetLeft = 1, 1, 0, 0
			ev.StartIndexing()
			applyNextIndexTask(t)
			if ev.targetLine != -1 || ev.CursorLine != 1 || ev.CursorPos != 1 {
				t.Fatalf("saved source position was not restored: target=%d cursor=%d:%d", ev.targetLine, ev.CursorLine, ev.CursorPos)
			}
			if ev.ScrollTopRow != 4 || ev.DesiredVisualCol != 1 {
				t.Fatalf("restoration used stale prefix layout: top=%d column=%d", ev.ScrollTopRow, ev.DesiredVisualCol)
			}
		})
	}
}

type indexLifecycleRemote struct {
	vfs.VFS
	offsets []int64
	total   int64
}

func (r *indexLifecycleRemote) LineIndex(ctx context.Context, _ string, first, count int64) (vfs.LineIndexResult, error) {
	if err := ctx.Err(); err != nil {
		return vfs.LineIndexResult{}, err
	}
	return vfs.LineIndexResult{First: first, Offsets: r.offsets, Total: r.total}, nil
}

func TestEditorIndexBatchDoesNotRestoreIncompleteFinalLine(t *testing.T) {
	ev := indexLifecycleEditor(t, []byte("head\n"+strings.Repeat("x", 100000)+"\nend"))
	ev.targetLine, ev.targetPos, ev.targetTopRow, ev.targetLeft = 1, 70000, 0, 0
	ev.StartIndexing()
	for ev.li.LineCount() < 2 {
		applyNextIndexTask(t)
	}
	if ev.li.LineCount() != 2 {
		t.Fatalf("first source batch must end in target's partial line, got %d lines", ev.li.LineCount())
	}
	if ev.targetLine != 1 || ev.CursorPos == 70000 {
		t.Fatalf("incomplete target restored before its line end: target=%d cursor=%d", ev.targetLine, ev.CursorPos)
	}
}

func TestEditorMappingMutationsCancelOldGesturesAndScroll(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*EditorView)
	}{
		{"edit", func(ev *EditorView) { ev.noteBufferEdit() }},
		{"replace", func(ev *EditorView) { ev.SetText("replacement") }},
		{"undo-layout", func(ev *EditorView) { ev.clearCaches() }},
		{"columns", func(ev *EditorView) { ev.nativeViewportColumns++; ev.ensureEngineWidth() }},
		{"tabs", func(ev *EditorView) { ev.TabSize++; ev.ensureEngineWidth() }},
		{"wrap", func(ev *EditorView) { ev.WordWrap = !ev.WordWrap; ev.ensureEngineWidth() }},
	} {
		t.Run(change.name, func(t *testing.T) {
			ev := projectionTestEditor(t, "123\tline")
			ev.nativeViewportColumns = 8
			ev.ensureEngineWidth()
			revision := ev.semanticLayoutRevision
			ev.semanticPointerActive, ev.semanticPendingScroll = true, true
			ev.semanticPendingGeneration = 42
			change.apply(ev)
			if ev.semanticLayoutRevision <= revision || ev.semanticPointerActive || ev.semanticPendingScroll || ev.semanticPendingGeneration != 0 {
				t.Fatalf("old coordinates survived mapping mutation: revision=%d pointer=%v pending=%v generation=%d", ev.semanticLayoutRevision, ev.semanticPointerActive, ev.semanticPendingScroll, ev.semanticPendingGeneration)
			}
		})
	}
}

func TestEditorCanceledHighlightTasksCannotMutateOrRedrawReplacement(t *testing.T) {
	ev := projectionTestEditor(t, "one\ntwo\n")
	drainPendingTasks()
	highlighter := &mockSlowHighlighter{}
	ev.highlighter = highlighter
	ev.startHighlighting()
	var oldSlice, oldCompletion func()
	select {
	case oldSlice = <-vtui.FrameManager.TaskChan:
	case <-time.After(2 * time.Second):
		t.Fatal("old highlight slice missing")
	}
	ev.invalidateStates(0)
	select {
	case oldCompletion = <-vtui.FrameManager.TaskChan:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled highlight completion missing")
	}
	ev.startHighlighting()
	for len(vtui.FrameManager.RedrawChan) > 0 {
		<-vtui.FrameManager.RedrawChan
	}
	oldSlice()
	oldCompletion()
	if !ev.highlighting || highlighter.calls != 0 || len(ev.lineStates) != 0 || len(vtui.FrameManager.RedrawChan) != 0 {
		t.Fatalf("obsolete highlight task changed replacement: active=%v calls=%d states=%d redraws=%d", ev.highlighting, highlighter.calls, len(ev.lineStates), len(vtui.FrameManager.RedrawChan))
	}
	ev.cancelHighlighting()
}
