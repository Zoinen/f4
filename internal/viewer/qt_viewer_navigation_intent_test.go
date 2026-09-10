package viewer

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"strings"
	"sync"
	"testing"
	"time"
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
		Backend: &ViewerBackend{
			File: file, size: int64(len(data)), totalLines: -1,
			totalForSize: -1, ctx: ctx, cancelCtx: cancel,
		},
		WrapMode: true, SemanticLayoutRevision: 1,
		NativeViewportColumns: columns, NativeViewportRows: rows,
		NativeViewportRevision: 1,
	}
	viewer.ScrollBar = vtui.NewScrollBar(0, 0, 0)
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
	if viewer.TopOffset != 0 || viewer.SemanticPendingScroll {
		t.Fatalf("one Home press was overwritten: top=%d pending=%v",
			viewer.TopOffset, viewer.SemanticPendingScroll)
	}
	if got := semantic.AppInt64(node["viewportStart"]); got != 0 {
		t.Fatalf("published viewport starts at %d after Home, want 0", got)
	}
	if viewer.semanticWindowGeneration != 13 || viewer.semanticWindowRequestGeneration != 13 {
		t.Fatalf("Home did not publish after native generation 12: acknowledged=%d high=%d",
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
	if got := semantic.AppInt64(node["windowGeneration"]); got != 13 {
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
	if viewer.TopOffset != 0 || viewer.SemanticPendingScroll {
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
	if viewer.TopOffset != want || viewer.SemanticPendingScroll {
		t.Fatalf("one End press lost to native request: top=%d want=%d pending=%v",
			viewer.TopOffset, want, viewer.SemanticPendingScroll)
	}
	node := viewer.SemanticNode(nil)
	if got := semantic.AppInt64(node["viewportStart"]); got != want {
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
	viewer.SemanticPendingScroll = true
	viewer.semanticPendingOffset = 200
	viewer.SemanticPendingGeneration = 11

	if !pressViewerNavigationKey(viewer, vtinput.VK_HOME) {
		t.Fatal("Home was not handled")
	}
	node := viewer.SemanticNode(nil)
	if viewer.TopOffset != 0 || viewer.SemanticPendingScroll {
		t.Fatalf("Home retained in-flight native destination: top=%d pending=%v",
			viewer.TopOffset, viewer.SemanticPendingScroll)
	}
	if got := semantic.AppInt64(node["windowGeneration"]); got != 12 {
		t.Fatalf("Home scene generation=%d, want 12 after canceled generation 11", got)
	}
	if got := semantic.AppInt64(node["windowRequestGeneration"]); got != 12 {
		t.Fatalf("Home request high-water=%d, want 12", got)
	}
}

func TestViewerNonNavigationKeyPreservesPendingNativeWindow(t *testing.T) {
	viewer := cachedSemanticViewer([]byte(strings.Repeat("row\n", 80)))
	viewer.semanticWindowGeneration = 4
	viewer.semanticWindowRequestGeneration = 5
	viewer.SemanticPendingScroll = true
	viewer.semanticPendingOffset = 40
	viewer.SemanticPendingGeneration = 5

	if pressViewerNavigationKey(viewer, vtinput.VK_F9) {
		t.Fatal("unbound F9 was unexpectedly handled")
	}
	if !viewer.SemanticPendingScroll || viewer.SemanticPendingGeneration != 5 ||
		viewer.semanticWindowGeneration != 4 || viewer.semanticWindowRequestGeneration != 5 {
		t.Fatalf("non-navigation key mutated pending window state: pending=%v generation=%d ack=%d high=%d",
			viewer.SemanticPendingScroll, viewer.SemanticPendingGeneration,
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
	if got := semantic.AppInt64(node["viewportStart"]); got != 0 {
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
	semantic.ApplyNativeDocumentViewport(viewer, semantic.NativeDocumentGeometry{
		Columns: 20, Rows: 4, Revision: 2,
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

func TestViewerPageDownSupersedesPendingNativeWindow(t *testing.T) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}
	viewer := cachedSemanticViewer(data)
	viewer.HexMode = true
	viewer.TopOffset = 0
	viewer.semanticWindowGeneration = 10
	viewer.semanticWindowRequestGeneration = 10

	// A native scroll request was accepted as pending.
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": int64(16), "generation": uint64(11),
	}) {
		t.Fatal("native scroll request was not handled")
	}

	if !pressViewerNavigationKey(viewer, vtinput.VK_NEXT) {
		t.Fatal("PageDown was not handled")
	}

	wantOffset := int64(16 * viewer.viewportHeight())
	if viewer.TopOffset != wantOffset || viewer.SemanticPendingScroll {
		t.Fatalf("PageDown was not applied: top=%d want=%d pending=%v",
			viewer.TopOffset, wantOffset, viewer.SemanticPendingScroll)
	}
	node := viewer.SemanticNode(nil)
	if got := semantic.AppInt64(node["viewportStart"]); got != wantOffset {
		t.Fatalf("published viewport starts at %d after PageDown, want %d", got, wantOffset)
	}
	if viewer.semanticWindowGeneration != 12 || viewer.semanticWindowRequestGeneration != 12 {
		t.Fatalf("PageDown did not publish fresh generation 12: acknowledged=%d high=%d",
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
	if got := semantic.AppInt64(node["windowGeneration"]); got != 12 {
		t.Fatalf("PageDown scene generation=%d, want 12", got)
	}
}

func TestViewerPageUpSupersedesPendingNativeWindow(t *testing.T) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}
	viewer := cachedSemanticViewer(data)
	viewer.HexMode = true
	pageBytes := int64(16 * viewer.viewportHeight())
	viewer.TopOffset = pageBytes * 2
	viewer.semanticWindowGeneration = 20
	viewer.semanticWindowRequestGeneration = 20

	// A native scroll request was accepted as pending.
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": pageBytes * 3, "generation": uint64(21),
	}) {
		t.Fatal("native scroll request was not handled")
	}

	if !pressViewerNavigationKey(viewer, vtinput.VK_PRIOR) {
		t.Fatal("PageUp was not handled")
	}

	wantOffset := pageBytes
	if viewer.TopOffset != wantOffset || viewer.SemanticPendingScroll {
		t.Fatalf("PageUp was not applied: top=%d want=%d pending=%v",
			viewer.TopOffset, wantOffset, viewer.SemanticPendingScroll)
	}
	node := viewer.SemanticNode(nil)
	if got := semantic.AppInt64(node["viewportStart"]); got != wantOffset {
		t.Fatalf("published viewport starts at %d after PageUp, want %d", got, wantOffset)
	}
	if viewer.semanticWindowGeneration != 22 || viewer.semanticWindowRequestGeneration != 22 {
		t.Fatalf("PageUp did not publish fresh generation 22: acknowledged=%d high=%d",
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
	if got := semantic.AppInt64(node["windowGeneration"]); got != 22 {
		t.Fatalf("PageUp scene generation=%d, want 22", got)
	}
}

func TestViewerDecodeModePageDownSupersedesPendingNativeWindow(t *testing.T) {
	// NOP instructions (0x90), 1 byte each
	data := make([]byte, 1024)
	for i := range data {
		data[i] = 0x90
	}
	viewer := cachedSemanticViewer(data)
	viewer.DecodeMode = true
	viewer.TopOffset = 0
	viewer.semanticWindowGeneration = 10
	viewer.semanticWindowRequestGeneration = 10

	// A native scroll request was accepted as pending.
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": int64(10), "generation": uint64(11),
	}) {
		t.Fatal("native scroll request was not handled")
	}

	if !pressViewerNavigationKey(viewer, vtinput.VK_NEXT) {
		t.Fatal("PageDown was not handled")
	}

	wantOffset := int64(viewer.viewportHeight())
	if viewer.TopOffset != wantOffset || viewer.SemanticPendingScroll {
		t.Fatalf("PageDown was not applied: top=%d want=%d pending=%v",
			viewer.TopOffset, wantOffset, viewer.SemanticPendingScroll)
	}
	node := viewer.SemanticNode(nil)
	if got := semantic.AppInt64(node["viewportStart"]); got != wantOffset {
		t.Fatalf("published viewport starts at %d after PageDown, want %d", got, wantOffset)
	}
	if viewer.semanticWindowGeneration != 12 || viewer.semanticWindowRequestGeneration != 12 {
		t.Fatalf("PageDown did not publish fresh generation 12: acknowledged=%d high=%d",
			viewer.semanticWindowGeneration, viewer.semanticWindowRequestGeneration)
	}
	if got := semantic.AppInt64(node["windowGeneration"]); got != 12 {
		t.Fatalf("PageDown scene generation=%d, want 12", got)
	}
}

func TestViewerArrowAndWheelNavigationSupersedePendingNativeWindow(t *testing.T) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	for _, tc := range []struct {
		name string
		key  uint16
		from int64
		want int64
	}{
		{name: "up", key: vtinput.VK_UP, from: 32, want: 16},
		{name: "down", key: vtinput.VK_DOWN, from: 16, want: 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viewer := cachedSemanticViewer(data)
			viewer.HexMode = true
			viewer.TopOffset = tc.from
			viewer.semanticWindowGeneration = 10
			viewer.semanticWindowRequestGeneration = 10
			if !viewer.HandleSemanticAction(map[string]any{
				"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
				"offset": int64(128), "generation": uint64(11),
			}) {
				t.Fatal("native scroll request was not handled")
			}
			if !pressViewerNavigationKey(viewer, tc.key) {
				t.Fatalf("%s was not handled", tc.name)
			}
			if viewer.TopOffset != tc.want || viewer.SemanticPendingScroll {
				t.Fatalf("%s was overwritten: top=%d want=%d pending=%v",
					tc.name, viewer.TopOffset, tc.want, viewer.SemanticPendingScroll)
			}
			node := viewer.SemanticNode(nil)
			if got := semantic.AppInt64(node["viewportStart"]); got != tc.want {
				t.Fatalf("%s published viewport starts at %d, want %d",
					tc.name, got, tc.want)
			}
			if got := viewer.semanticWindowGeneration; got != 12 {
				t.Fatalf("%s did not publish successor generation: %d", tc.name, got)
			}
		})
	}

	originalConfig := config.App
	t.Cleanup(func() { config.App = originalConfig })
	config.App.WheelViewerDown = 1
	viewer := cachedSemanticViewer(data)
	viewer.HexMode = true
	viewer.TopOffset = 16
	viewer.semanticWindowGeneration = 20
	viewer.semanticWindowRequestGeneration = 20
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": int64(128), "generation": uint64(21),
	}) {
		t.Fatal("native wheel destination was not handled")
	}
	if !viewer.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, WheelDirection: -1,
	}) {
		t.Fatal("wheel event was not handled")
	}
	if viewer.TopOffset != 32 || viewer.SemanticPendingScroll {
		t.Fatalf("wheel navigation was overwritten: top=%d want=32 pending=%v",
			viewer.TopOffset, viewer.SemanticPendingScroll)
	}
}

func TestViewerGotoLineRejectsOlderCompletionAfterNavigation(t *testing.T) {
	data := []byte(strings.Repeat("row\n", 1000))
	viewer, file := navigationTestViewer(t, data, 40, 8)
	viewer.WrapMode = false
	viewer.gotoPosition(500)
	select {
	case <-file.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Go to line calculation did not start")
	}
	if !pressViewerNavigationKey(viewer, vtinput.VK_HOME) {
		t.Fatal("Home was not handled while Go to line was pending")
	}
	file.unblock()
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("Go to line completion was not delivered")
	}
	if viewer.TopOffset != 0 {
		t.Fatalf("stale Go to line completion overwrote Home: top=%d", viewer.TopOffset)
	}
}
