package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func constructionTestViewer(t *testing.T, content []byte, columns, rows int, wrap bool) (*ViewerView, *documentRangeReader) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &documentRangeReader{data: content, profile: vfs.ReadAccessUnknownExpensive}
	ctx, cancel := context.WithCancel(context.Background())
	viewer := &ViewerView{backend: &ViewerBackend{file: file, size: file.Size(), ctx: ctx, cancelCtx: cancel},
		WrapMode: wrap, semanticLayoutRevision: 1, nativeViewportRevision: 1,
		nativeViewportColumns: columns, nativeViewportRows: rows}
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
	var window semanticSurfaceWindow
	finishViewerConstruction(t, func() bool { window = viewer.semanticWindow(); return window.ready })
	if len(window.rows) != 2 || window.rows[0].EndOffset != 700*1024+1 || window.end != int64(len(content)) {
		t.Fatalf("incoherent completed rows=%d end=%d first=%+v", len(window.rows), window.end, window.rows)
	}
	if viewer.semanticProjection != nil {
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
		return viewer.eofVisible && viewer.consoleProjection == nil
	})
	if len(viewer.lineOffsets) != 2 || viewer.lineOffsets[1] != int64(len(line)) {
		t.Fatalf("console lost source rows: %v", viewer.lineOffsets)
	}
}

func TestViewerWideAsyncWindowDiscardsSupersededConstruction(t *testing.T) {
	content := bytes.Repeat([]byte{'x'}, 1024*1024)
	viewer, _ := constructionTestViewer(t, content, 10000, 40, true)
	if window := viewer.semanticWindow(); window.ready {
		t.Fatal("first expensive read unexpectedly ready")
	}
	// Readiness is independent from the global UI queue: other package tests
	// may have left unrelated tasks ahead of this source's completion.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := viewer.backend.ReadContext(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	if window := viewer.semanticWindow(); window.ready {
		t.Fatal("viewport larger than source window unexpectedly completed in one source fill")
	}
	previous := viewer.semanticProjection
	if previous == nil || len(previous.rows) == 0 {
		t.Fatal("test did not retain an unfinished multi-source-window projection")
	}
	// A resize plus a reversed destination must not splice ready rows from the
	// abandoned geometry/generation into the replacement window.
	viewer.TopOffset = 300000
	viewer.semanticWindowRequestGeneration = 2
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{columns: 5000, rows: 20, revision: 2})
	viewer.semanticNeedsReflow = false // chosen target is already a fragment boundary
	var window semanticSurfaceWindow
	finishViewerConstruction(t, func() bool {
		window = viewer.semanticWindow()
		if viewer.semanticProjection == previous {
			t.Fatal("superseded geometry retained old construction")
		}
		return window.ready
	})
	if window.viewportRows != 20 || window.rows[window.viewportRow].Offset != 300000 {
		t.Fatalf("wrong final viewport: rows=%d offset=%d", window.viewportRows, window.rows[window.viewportRow].Offset)
	}
	for _, row := range window.rows {
		if row.EndOffset-row.Offset != 5000 {
			t.Fatalf("mixed old/new layout row: %+v", row)
		}
	}
	viewer.backend.mu.Lock()
	defer viewer.backend.mu.Unlock()
	if len(viewer.backend.cacheData) > 256*1024 || viewer.semanticProjection != nil {
		t.Fatal("completed projection retained a larger source buffer or construction")
	}
}

func TestViewerLargeLocalWrappedSeekYieldsBoundedWork(t *testing.T) {
	viewer, file := constructionTestViewer(t, bytes.Repeat([]byte{'x'}, 16*1024*1024), 80, 8, true)
	file.profile = vfs.ReadAccessDirectLocal
	const target = int64(8 * 1024 * 1024)
	viewer.semanticPendingScroll, viewer.semanticPendingOffset = true, target
	viewer.semanticPendingGeneration, viewer.semanticWindowRequestGeneration = 1, 1
	if _, ready := viewer.semanticWrappedRowStart(target, 80); ready {
		t.Fatal("large local seek consumed the complete prefix in one UI turn")
	}
	viewer.backend.mu.Lock()
	curr := viewer.backend.lineSeekCurr
	viewer.backend.mu.Unlock()
	if consumed := target - curr; consumed <= 0 || consumed > viewerProjectionWorkBytes {
		t.Fatalf("backward seek consumed %d bytes in its first turn", consumed)
	}
	if !viewer.projectionContinuationPending || len(file.reads) != 1 {
		t.Fatalf("local work did not schedule one continuation: pending=%v reads=%d", viewer.projectionContinuationPending, len(file.reads))
	}
	awaitConstructionTask(t)
	// Isolate the forward phase to prove it is bounded independently from the
	// physical-line lookup, even when its source reads are all immediate.
	viewer.semanticWrapSeek.reset(target, 80, 9)
	viewer.semanticWrapSeek.lineStartReady = true
	if _, ready := viewer.semanticWrappedRowStart(target, 80); ready {
		t.Fatal("large wrapped prefix was fully scanned in one UI turn")
	}
	if scanned := viewer.semanticWrapSeek.curr; scanned <= 0 || scanned > viewerProjectionWorkBytes+80 {
		t.Fatalf("forward seek consumed %d bytes in one turn", scanned)
	}
	// A queued continuation must not redraw after a newer destination owns the
	// document. Drain prior coalesced redraws before executing the stale task.
	for len(vtui.FrameManager.RedrawChan) > 0 {
		<-vtui.FrameManager.RedrawChan
	}
	viewer.semanticWindowRequestGeneration = 2
	viewer.semanticPendingGeneration = 2
	viewer.semanticPendingOffset = 0
	awaitConstructionTask(t)
	if len(vtui.FrameManager.RedrawChan) != 0 {
		t.Fatal("superseded local continuation scheduled a redraw")
	}
}
