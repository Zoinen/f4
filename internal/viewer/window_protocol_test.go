package viewer

import (
	context "context"
	fmt "fmt"
	semantic "github.com/unxed/f4/internal/semantic"
	vfs "github.com/unxed/f4/vfs"
	vtinput "github.com/unxed/vtinput"
	vtui "github.com/unxed/vtui"
	os "os"
	filepath "path/filepath"
	strings "strings"
	sync "sync"
	testing "testing"
	time "time"
)

type semanticDelayedViewerRead struct {
	vfs.ReadAtCloser
	started, release, completed chan struct{}
	once                        sync.Once
}

func (*semanticDelayedViewerRead) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessUnknownExpensive
}

func (f *semanticDelayedViewerRead) ReadAt(ctx context.Context, dst []byte, off int64) (int, error) {
	first := false
	f.once.Do(func() { first = true; close(f.started) })
	if first {
		<-f.release
		// Model an already-issued native read that cannot be canceled. Its
		// completion must still be rejected by the generation/lifecycle fence.
		n, err := f.ReadAtCloser.ReadAt(context.Background(), dst, off)
		close(f.completed)
		return n, err
	}
	return f.ReadAtCloser.ReadAt(ctx, dst, off)
}

func TestSemanticWindowProtocol_NativeViewportControlsViewerPageStep(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "viewport.bin")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 39, 10)
	target := vtui.SemanticID(viewer)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "document.viewport", "rows": 8,
	}) {
		t.Fatal("native viewer viewport was not handled")
	}
	if got := semantic.Int(viewer.SemanticNode(nil)["viewportRows"]); got != 8 {
		t.Fatalf("semantic viewer viewport rows=%d, want 8", got)
	}

	viewer.TopOffset = 0
	viewer.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT,
	})
	if viewer.TopOffset != 8*16 {
		t.Fatalf("native PgDn offset=%d, want %d", viewer.TopOffset, 8*16)
	}

	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "document.viewport", "rows": 0,
	})
	viewer.TopOffset = 0
	viewer.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT,
	})
	if viewer.TopOffset != 10*16 {
		t.Fatalf("terminal PgDn offset=%d, want %d", viewer.TopOffset, 10*16)
	}
}

func TestSemanticWindowProtocol_ViewerKeepsThreeScreensAndStableOverlap(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "overlap.txt")
	var content strings.Builder
	var offsets []int64
	for row := 0; row < 200; row++ {
		offsets = append(offsets, int64(content.Len()))
		fmt.Fprintf(&content, "line-%03d\n", row)
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 39, 8)
	viewer.TopOffset = offsets[80]
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := awaitSemanticViewerWindow(t, viewer, 24)
	viewport := semantic.Int(first["viewportRows"])
	if got, want := len(semantic.AppMapSlice(first["windowRows"])), 3*viewport; got != want {
		t.Fatalf("initial bounded rows=%d, want %d", got, want)
	}
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scrollWindow",
		"offset": offsets[84], "generation": uint64(1),
	}) {
		t.Fatal("viewer window request was not handled")
	}
	second := awaitSemanticViewerWindow(t, viewer, 24)
	if overlap := assertSemanticOverlapStable(t, first, second, "bytes"); overlap < 2*viewport {
		t.Fatalf("viewer overlap=%d rows, want at least %d", overlap, 2*viewport)
	}
	oldBoundaryOffset := offsets[88]
	if _, ok := semanticRowsByExtent(t, second, "bytes")[oldBoundaryOffset]; !ok {
		t.Fatalf("new viewer window lost live old-boundary anchor offset %d", oldBoundaryOffset)
	}
}

func TestSemanticWindowProtocol_SupersededAsyncViewerSeekCannotJumpBack(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "async.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 2*1024*1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	delayed := &semanticDelayedViewerRead{ReadAtCloser: viewer.Backend.File,
		started: make(chan struct{}), release: make(chan struct{}), completed: make(chan struct{})}
	viewer.Backend.File = delayed
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(delayed.release) }) }
	defer release()
	viewer.SetPosition(0, 0, 39, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	target := vtui.SemanticID(viewer)

	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "viewer.scrollWindow", "offset": int64(1024 * 1024),
		"generation": uint64(7),
	})
	if !viewer.SemanticPendingScroll || viewer.SemanticPendingGeneration != 7 {
		t.Fatalf("far request pending=%v generation=%d",
			viewer.SemanticPendingScroll, viewer.SemanticPendingGeneration)
	}
	first := viewer.SemanticNode(nil)
	if first["layoutPending"] != true || viewer.semanticWindowGeneration != 0 {
		t.Fatal("uncached generation 7 was acknowledged before its read")
	}
	select {
	case <-delayed.started:
	case <-time.After(2 * time.Second):
		t.Fatal("generation 7 source read did not start")
	}
	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "viewer.scrollWindow", "offset": int64(0),
		"generation": uint64(8),
	})
	if !viewer.SemanticPendingScroll || viewer.semanticWindowGeneration != 0 {
		t.Fatal("generation 8 was acknowledged before projection")
	}
	latest := viewer.SemanticNode(nil)
	if viewer.SemanticPendingScroll || viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("new request state pending=%v top=%d generation=%d",
			viewer.SemanticPendingScroll, viewer.TopOffset, viewer.semanticWindowGeneration)
	}

	release()
	select {
	case <-delayed.completed:
	case <-time.After(2 * time.Second):
		t.Fatal("superseded read did not finish")
	}
	deadline := time.After(2 * time.Second)
	for {
		viewer.Backend.mu.Lock()
		fetching := viewer.Backend.isFetching
		viewer.Backend.mu.Unlock()
		latest = viewer.SemanticNode(nil)
		if viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 8 {
			t.Fatalf("late read changed viewport: top=%d generation=%d", viewer.TopOffset, viewer.semanticWindowGeneration)
		}
		if !fetching && latest["layoutPending"] == false {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(time.Millisecond): // Test-only observation of a canceled no-redraw completion.
		case <-deadline:
			t.Fatal("latest window never became ready after supersession")
		}
	}
	if viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("late generation 7 completion jumped to top=%d generation=%d",
			viewer.TopOffset, viewer.semanticWindowGeneration)
	}
	viewer.HandleSemanticAction(map[string]any{
		"target": target, "action": "viewer.scrollWindow", "offset": int64(512 * 1024),
		"generation": uint64(7),
	})
	if viewer.TopOffset != 0 || viewer.SemanticPendingScroll || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("stale action mutated viewer: top=%d pending=%v generation=%d",
			viewer.TopOffset, viewer.SemanticPendingScroll, viewer.semanticWindowGeneration)
	}
}
