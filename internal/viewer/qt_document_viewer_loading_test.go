package viewer

import (
	"bytes"
	"context"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"testing"
	"time"
)

func constructionTestViewer(t *testing.T, content []byte, columns, rows int, wrap bool) (*ViewerView, *documentRangeReader) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &documentRangeReader{data: content, profile: vfs.ReadAccessUnknownExpensive}
	ctx, cancel := context.WithCancel(context.Background())
	viewer := &ViewerView{Backend: &ViewerBackend{File: file, size: file.Size(), ctx: ctx, cancelCtx: cancel},
		WrapMode: wrap, SemanticLayoutRevision: 1, NativeViewportRevision: 1,
		NativeViewportColumns: columns, NativeViewportRows: rows}
	viewer.SetPosition(0, 0, columns, rows)
	t.Cleanup(viewer.Close)
	return viewer, file
}

func awaitConstructionTask(t *testing.T) {
	t.Helper()
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(3 * time.Second):
		t.Fatal("unfinished viewer projection did not receive source readiness")
	}
}

func finishViewerConstruction(t *testing.T, render func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if render() {
			return
		}
		awaitConstructionTask(t)
	}
	t.Fatal("viewer construction restarted instead of making bounded forward progress")
}

func TestViewerAsyncLongRowConstructionResumesAcrossSourceWindows(t *testing.T) {
	content := append(bytes.Repeat([]byte{'x'}, 700*1024), []byte("\nlast\n")...)
	viewer, file := constructionTestViewer(t, content, 80, 8, false)
	var window semantic.SemanticSurfaceWindow
	finishViewerConstruction(t, func() bool { window = viewer.semanticWindow(); return window.Ready })
	if len(window.Rows) != 2 || window.Rows[0].EndOffset != 700*1024+1 || window.End != int64(len(content)) {
		t.Fatalf("incoherent completed rows=%d end=%d first=%+v", len(window.Rows), window.End, window.Rows)
	}
	if viewer.SemanticProjection != nil {
		t.Fatal("completed window retained its construction state")
	}
	file.mu.Lock()
	defer file.mu.Unlock()
	if len(file.reads) > 6 || file.maxActive != 1 {
		t.Fatalf("source scan restarted or fanned out: reads=%+v maxActive=%d", file.reads, file.maxActive)
	}
	for _, read := range file.reads {
		if read.length > 256*1024 {
			t.Fatalf("source buffer enlarged: %+v", read)
		}
	}
}

func TestViewerConsoleLongRowsConstructionResumes(t *testing.T) {
	line := append(bytes.Repeat([]byte{'a'}, 360*1024), '\n')
	viewer, _ := constructionTestViewer(t, append(append([]byte(nil), line...), line...), 80, 8, false)
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(81, 10)
	finishViewerConstruction(t, func() bool {
		viewer.renderTextRows(screen, 80, 8, false)
		return viewer.EofVisible && viewer.ConsoleProjection == nil
	})
	if len(viewer.LineOffsets) != 2 || viewer.LineOffsets[1] != int64(len(line)) {
		t.Fatalf("console lost source rows: %v", viewer.LineOffsets)
	}
}

func TestViewerWideAsyncWindowDiscardsSupersededConstruction(t *testing.T) {
	content := bytes.Repeat([]byte{'x'}, 1024*1024)
	viewer, _ := constructionTestViewer(t, content, 10000, 40, true)
	if window := viewer.semanticWindow(); window.Ready {
		t.Fatal("first expensive read unexpectedly ready")
	}
	// Readiness is independent from the global UI queue: other package tests
	// may have left unrelated tasks ahead of this source's completion.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := viewer.Backend.ReadContext(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	if window := viewer.semanticWindow(); window.Ready {
		t.Fatal("viewport larger than source window unexpectedly completed in one source fill")
	}
	previous := viewer.SemanticProjection
	if previous == nil || len(previous.rows) == 0 {
		t.Fatal("test did not retain an unfinished multi-source-window projection")
	}
	// A resize plus a reversed destination must not splice ready rows from the
	// abandoned geometry/generation into the replacement window.
	viewer.TopOffset = 300000
	viewer.semanticWindowRequestGeneration = 2
	semantic.ApplyNativeDocumentViewport(viewer, semantic.NativeDocumentGeometry{Columns: 5000, Rows: 20, Revision: 2})
	viewer.SemanticNeedsReflow = false // chosen target is already a fragment boundary
	var window semantic.SemanticSurfaceWindow
	finishViewerConstruction(t, func() bool {
		window = viewer.semanticWindow()
		if viewer.SemanticProjection == previous {
			t.Fatal("superseded geometry retained old construction")
		}
		return window.Ready
	})
	if window.ViewportRows != 20 || window.Rows[window.ViewportRow].Offset != 300000 {
		t.Fatalf("wrong final viewport: rows=%d offset=%d", window.ViewportRows, window.Rows[window.ViewportRow].Offset)
	}
	for _, row := range window.Rows {
		if row.EndOffset-row.Offset != 5000 {
			t.Fatalf("mixed old/new layout row: %+v", row)
		}
	}
	viewer.Backend.mu.Lock()
	defer viewer.Backend.mu.Unlock()
	if len(viewer.Backend.cacheData) > 256*1024 || viewer.SemanticProjection != nil {
		t.Fatal("completed projection retained a larger source buffer or construction")
	}
}

func TestViewerLargeLocalWrappedSeekYieldsBoundedWork(t *testing.T) {
	viewer, file := constructionTestViewer(t, bytes.Repeat([]byte{'x'}, 16*1024*1024), 80, 8, true)
	file.profile = vfs.ReadAccessDirectLocal
	const target = int64(8 * 1024 * 1024)
	viewer.SemanticPendingScroll, viewer.semanticPendingOffset = true, target
	viewer.SemanticPendingGeneration, viewer.semanticWindowRequestGeneration = 1, 1
	if _, ready := viewer.semanticWrappedRowStart(target, 80); ready {
		t.Fatal("large local seek consumed the complete prefix in one UI turn")
	}
	viewer.Backend.mu.Lock()
	curr := viewer.Backend.lineSeekCurr
	viewer.Backend.mu.Unlock()
	if consumed := target - curr; consumed <= 0 || consumed > viewerProjectionWorkBytes {
		t.Fatalf("backward seek consumed %d bytes in its first turn", consumed)
	}
	if !viewer.projectionContinuationPending || len(file.reads) != 1 {
		t.Fatalf("local work did not schedule one continuation: pending=%v reads=%d", viewer.projectionContinuationPending, len(file.reads))
	}
	awaitConstructionTask(t)
	// Isolate the forward phase to prove it is bounded independently from the
	// physical-line lookup, even when its source reads are all immediate.
	viewer.SemanticWrapSeek.reset(target, 80, 9)
	viewer.SemanticWrapSeek.lineStartReady = true
	if _, ready := viewer.semanticWrappedRowStart(target, 80); ready {
		t.Fatal("large wrapped prefix was fully scanned in one UI turn")
	}
	if scanned := viewer.SemanticWrapSeek.curr; scanned <= 0 || scanned > viewerProjectionWorkBytes+80 {
		t.Fatalf("forward seek consumed %d bytes in one turn", scanned)
	}
	// A queued continuation must not redraw after a newer destination owns the
	// document. Drain prior coalesced redraws before executing the stale task.
	for len(vtui.FrameManager.RedrawChan) > 0 {
		<-vtui.FrameManager.RedrawChan
	}
	viewer.semanticWindowRequestGeneration = 2
	viewer.SemanticPendingGeneration = 2
	viewer.semanticPendingOffset = 0
	awaitConstructionTask(t)
	if len(vtui.FrameManager.RedrawChan) != 0 {
		t.Fatal("superseded local continuation scheduled a redraw")
	}
}
