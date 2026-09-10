package editor

import (
	"context"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/textlayout"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"strings"
	"testing"
	"time"
)

func indexLifecycleEditor(t *testing.T, data []byte) *EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()
	reader := &documentRangeReader{data: data}
	buf := NewAsyncBuffer(context.Background(), reader)
	buf.SeedPrefix(data)
	ev := NewEditorView(piecetable.NewWithBuffer(buf), nil, "")
	ev.AsyncBuf = buf
	ev.Li = piecetable.NewLineIndex()
	ev.Engine = textlayout.NewWrapEngine(ev.Pt, ev.Li)
	ev.NativeViewportColumns, ev.NativeViewportRows = 2, 1
	ev.WordWrap = true
	ev.EnsureEngineWidth()
	t.Cleanup(func() {
		if ev.IndexCancel != nil {
			ev.IndexCancel()
		}
		for ev.Indexing {
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
				ev.Vfs = &indexLifecycleRemote{offsets: []int64{9, 12}, total: 3}
			}
			// The first viewport can lay out the first logical line while the newline
			// index still has only its initial offset. That cached tail is provisional.
			ev.Engine.GetFragments(0)
			if rows, complete := ev.Engine.KnownVisualRows(); rows != 4 || complete {
				t.Fatalf("test requires a provisional wrapped tail: %d, %v", rows, complete)
			}
			ev.TargetLine, ev.TargetPos, ev.TargetTopRow, ev.TargetLeft = 1, 1, 0, 0
			ev.StartIndexing()
			applyNextIndexTask(t)
			if ev.TargetLine != -1 || ev.CursorLine != 1 || ev.CursorPos != 1 {
				t.Fatalf("saved source position was not restored: target=%d cursor=%d:%d", ev.TargetLine, ev.CursorLine, ev.CursorPos)
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
	ev.TargetLine, ev.TargetPos, ev.TargetTopRow, ev.TargetLeft = 1, 70000, 0, 0
	ev.StartIndexing()
	for ev.Li.LineCount() < 2 {
		applyNextIndexTask(t)
	}
	if ev.Li.LineCount() != 2 {
		t.Fatalf("first source batch must end in target's partial line, got %d lines", ev.Li.LineCount())
	}
	if ev.TargetLine != 1 || ev.CursorPos == 70000 {
		t.Fatalf("incomplete target restored before its line end: target=%d cursor=%d", ev.TargetLine, ev.CursorPos)
	}
}

func TestEditorMappingMutationsCancelOldGesturesAndScroll(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*EditorView)
	}{
		{"edit", func(ev *EditorView) { ev.noteBufferEdit() }},
		{"replace", func(ev *EditorView) { ev.SetText("replacement") }},
		{"undo-layout", func(ev *EditorView) { ev.ClearCaches() }},
		{"columns", func(ev *EditorView) { ev.NativeViewportColumns++; ev.EnsureEngineWidth() }},
		{"tabs", func(ev *EditorView) { ev.TabSize++; ev.EnsureEngineWidth() }},
		{"wrap", func(ev *EditorView) { ev.WordWrap = !ev.WordWrap; ev.EnsureEngineWidth() }},
	} {
		t.Run(change.name, func(t *testing.T) {
			ev := projectionTestEditor(t, "123\tline")
			ev.NativeViewportColumns = 8
			ev.EnsureEngineWidth()
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
	testutil.DrainPendingTasks()
	highlighter := &mockSlowHighlighter{}
	ev.Highlighter = highlighter
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
