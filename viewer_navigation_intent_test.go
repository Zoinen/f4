package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type viewerNavigationBlockingRead struct {
	vfs.ReadAtCloser
	started     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
}

func (*viewerNavigationBlockingRead) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessUnknownExpensive
}

func (f *viewerNavigationBlockingRead) ReadAt(ctx context.Context, dst []byte, off int64) (int, error) {
	blocked := false
	f.once.Do(func() {
		blocked = true
		close(f.started)
	})
	if blocked {
		select {
		case <-f.release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return f.ReadAtCloser.ReadAt(ctx, dst, off)
}

func (f *viewerNavigationBlockingRead) unblock() {
	f.releaseOnce.Do(func() { close(f.release) })
}

func navigationTestViewer(t *testing.T, data []byte, columns, rows int) (*ViewerView, *viewerNavigationBlockingRead) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &viewerNavigationBlockingRead{
		ReadAtCloser: &vfs.MemoryReadAtCloser{Data: data},
		started:      make(chan struct{}),
		release:      make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	viewer := &ViewerView{
		backend: &ViewerBackend{
			file: file, size: int64(len(data)), totalLines: -1,
			totalForSize: -1, ctx: ctx, cancelCtx: cancel,
		},
		WrapMode: true, semanticLayoutRevision: 1,
		nativeViewportColumns: columns, nativeViewportRows: rows,
		nativeViewportRevision: 1,
	}
	viewer.scrollBar = vtui.NewScrollBar(0, 0, 0)
	viewer.SetPosition(0, 0, columns, rows)
	viewer.SetVisible(true)
	t.Cleanup(func() {
		file.unblock()
		viewer.Close()
	})
	return viewer, file
}

func pressViewerNavigationKey(viewer *ViewerView, key uint16) bool {
	return viewer.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key,
	})
}

func runViewerTasksUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !condition() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("timed out waiting for viewer navigation task")
		}
	}
}

func TestViewerHomeSupersedesPendingNativeWindowOnFirstPress(t *testing.T) {
	var content strings.Builder
	var offsets []int64
	for row := 0; row < 160; row++ {
		offsets = append(offsets, int64(content.Len()))
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	viewer := cachedSemanticViewer([]byte(content.String()))
	viewer.TopOffset = offsets[80]
	viewer.semanticWindowGeneration = 10
	viewer.semanticWindowRequestGeneration = 10

	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": offsets[96], "generation": uint64(11),
	}) {
		t.Fatal("native scroll request was not handled")
	}
	// Model the replaceable destination queued behind the first request. Home
	// must retire the latest accepted generation, not merely the first one.
	viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": offsets[112], "generation": uint64(12),
	})
	if !pressViewerNavigationKey(viewer, vtinput.VK_HOME) {
		t.Fatal("Home was not handled")
	}
	node := viewer.SemanticNode(nil)
	if viewer.TopOffset != 0 || viewer.semanticPendingScroll {
		t.Fatalf("one Home press was overwritten: top=%d pending=%v",
			viewer.TopOffset, viewer.semanticPendingScroll)
	}
	if got := appInt64(node["viewportStart"]); got != 0 {
		t.Fatalf("published viewport starts at %d after Home, want 0", got)
	}
	if viewer.semanticWindowGeneration != 13 || viewer.semanticWindowRequestGeneration != 13 {
		t.Fatalf("Home did not publish after native generation 12: acknowledged=%d high=%d",
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
	if got := appInt64(node["windowGeneration"]); got != 13 {
		t.Fatalf("Home scene generation=%d, want fresh generation 13",
			got)
	}
	if viewer.semanticWindowGeneration < 12 {
		t.Fatalf("Home did not retire latest native generation 12: acknowledged=%d",
			viewer.semanticWindowGeneration)
	}
	viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": offsets[96], "generation": uint64(11),
	})
	if viewer.TopOffset != 0 || viewer.semanticPendingScroll {
		t.Fatal("retired native generation became pending again after Home")
	}
}

func TestViewerEndSupersedesPendingNativeWindowOnFirstPress(t *testing.T) {
	const line = "row\n"
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	viewer := cachedSemanticViewer([]byte(strings.Repeat(line, 100)))
	viewer.WrapMode = false
	viewer.semanticWindowGeneration = 20
	viewer.semanticWindowRequestGeneration = 20
	viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": int64(40), "generation": uint64(21),
	})
	if !pressViewerNavigationKey(viewer, vtinput.VK_END) {
		t.Fatal("End was not handled")
	}
	want := int64((100 - viewer.viewportHeight()) * len(line))
	runViewerTasksUntil(t, func() bool { return !viewer.Busy })
	if viewer.TopOffset != want || viewer.semanticPendingScroll {
		t.Fatalf("one End press lost to native request: top=%d want=%d pending=%v",
			viewer.TopOffset, want, viewer.semanticPendingScroll)
	}
	node := viewer.SemanticNode(nil)
	if got := appInt64(node["viewportStart"]); got != want {
		t.Fatalf("published End viewport starts at %d, want %d", got, want)
	}
	if viewer.semanticWindowGeneration != 22 || viewer.semanticWindowRequestGeneration != 22 {
		t.Fatalf("End did not publish after native generation 21: acknowledged=%d high=%d",
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
}

func TestViewerHomePublishesAfterCanceledInFlightNativeGeneration(t *testing.T) {
	viewer := cachedSemanticViewer([]byte(strings.Repeat("row\n", 80)))
	viewer.TopOffset = 160
	viewer.semanticWindowGeneration = 10
	viewer.semanticWindowRequestGeneration = 11
	viewer.semanticPendingScroll = true
	viewer.semanticPendingOffset = 200
	viewer.semanticPendingGeneration = 11

	if !pressViewerNavigationKey(viewer, vtinput.VK_HOME) {
		t.Fatal("Home was not handled")
	}
	node := viewer.SemanticNode(nil)
	if viewer.TopOffset != 0 || viewer.semanticPendingScroll {
		t.Fatalf("Home retained in-flight native destination: top=%d pending=%v",
			viewer.TopOffset, viewer.semanticPendingScroll)
	}
	if got := appInt64(node["windowGeneration"]); got != 12 {
		t.Fatalf("Home scene generation=%d, want 12 after canceled generation 11", got)
	}
	if got := appInt64(node["windowRequestGeneration"]); got != 12 {
		t.Fatalf("Home request high-water=%d, want 12", got)
	}
}

func TestViewerNonNavigationKeyPreservesPendingNativeWindow(t *testing.T) {
	viewer := cachedSemanticViewer([]byte(strings.Repeat("row\n", 80)))
	viewer.semanticWindowGeneration = 4
	viewer.semanticWindowRequestGeneration = 5
	viewer.semanticPendingScroll = true
	viewer.semanticPendingOffset = 40
	viewer.semanticPendingGeneration = 5

	if pressViewerNavigationKey(viewer, vtinput.VK_F9) {
		t.Fatal("unbound F9 was unexpectedly handled")
	}
	if !viewer.semanticPendingScroll || viewer.semanticPendingGeneration != 5 ||
		viewer.semanticWindowGeneration != 4 || viewer.semanticWindowRequestGeneration != 5 {
		t.Fatalf("non-navigation key mutated pending window state: pending=%v generation=%d ack=%d high=%d",
			viewer.semanticPendingScroll, viewer.semanticPendingGeneration,
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
}

func TestViewerHomeRejectsEarlierEndCompletion(t *testing.T) {
	data := []byte(strings.Repeat("abcdefghij\n", 1000))
	viewer, file := navigationTestViewer(t, data, 40, 8)
	viewer.WrapMode = false
	if !pressViewerNavigationKey(viewer, vtinput.VK_END) {
		t.Fatal("End was not handled")
	}
	select {
	case <-file.started:
	case <-time.After(2 * time.Second):
		t.Fatal("End calculation did not start")
	}
	if !pressViewerNavigationKey(viewer, vtinput.VK_HOME) {
		t.Fatal("Home was not handled while End was pending")
	}
	file.unblock()
	runViewerTasksUntil(t, func() bool { return !viewer.Busy })
	// The stale result callback may be queued immediately before its cleanup.
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(25 * time.Millisecond):
	}
	if viewer.TopOffset != 0 {
		t.Fatalf("earlier End completion overwrote Home: top=%d", viewer.TopOffset)
	}
	node := viewer.SemanticNode(nil)
	if got := appInt64(node["viewportStart"]); got != 0 {
		t.Fatalf("visible semantic window jumped to %d after stale End", got)
	}
}

func TestViewerEndRetargetsWhenViewportChangesDuringCalculation(t *testing.T) {
	const line = "abcdefghij\n"
	data := []byte(strings.Repeat(line, 1000))
	viewer, file := navigationTestViewer(t, data, 40, 8)
	viewer.WrapMode = false
	if !pressViewerNavigationKey(viewer, vtinput.VK_END) {
		t.Fatal("End was not handled")
	}
	select {
	case <-file.started:
	case <-time.After(2 * time.Second):
		t.Fatal("End calculation did not start")
	}
	applyNativeDocumentViewport(viewer, nativeDocumentGeometry{
		columns: 20, rows: 4, revision: 2,
	})
	file.unblock()
	want := int64((1000 - 4) * len(line))
	runViewerTasksUntil(t, func() bool {
		return !viewer.Busy && viewer.TopOffset != 0
	})
	if viewer.TopOffset != want {
		t.Fatalf("single End used superseded viewport: top=%d, want %d",
			viewer.TopOffset, want)
	}
}
