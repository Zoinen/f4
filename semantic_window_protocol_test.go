package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
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

func comparableSemanticRow(row map[string]any) map[string]any {
	result := make(map[string]any, len(row)-1)
	for key, value := range row {
		// Index is local to the bounded array. Absolute byte/visual extents are
		// the stable identity shared by two overlapping windows.
		if key != "index" {
			result[key] = value
		}
	}
	return result
}

func semanticRowsByExtent(t *testing.T, node map[string]any, unit string) map[int64]map[string]any {
	t.Helper()
	result := make(map[int64]map[string]any)
	for _, row := range appMapSlice(node["windowRows"]) {
		var extent int64
		if unit == "rows" {
			extent = appInt64(row["visualRow"])
		} else {
			extent = appInt64(row["offset"])
		}
		if _, duplicate := result[extent]; duplicate {
			t.Fatalf("duplicate %s extent %d in %#v", unit, extent, node["windowRows"])
		}
		result[extent] = row
	}
	return result
}

func assertSemanticOverlapStable(t *testing.T, first, second map[string]any, unit string) int {
	t.Helper()
	firstRows := semanticRowsByExtent(t, first, unit)
	secondRows := semanticRowsByExtent(t, second, unit)
	overlap := 0
	for extent, oldRow := range firstRows {
		newRow, ok := secondRows[extent]
		if !ok {
			continue
		}
		overlap++
		if !reflect.DeepEqual(comparableSemanticRow(oldRow), comparableSemanticRow(newRow)) {
			t.Fatalf("overlap changed at %s extent %d:\nold %#v\nnew %#v",
				unit, extent, oldRow, newRow)
		}
	}
	return overlap
}

func TestSemanticWindowProtocol_EditorKeepsThreeScreensAndStableOverlap(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for row := 0; row < 200; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "overlap.txt")
	editor.SetPosition(0, 0, 39, 8)
	editor.ScrollTopRow = 80

	first := editor.SemanticNode(nil)
	viewport := semanticInt(first["viewportRows"])
	buffer := semanticWindowBufferRows(viewport)
	if viewport != 8 || buffer != viewport {
		t.Fatalf("viewport=%d buffer=%d, want one full viewport of overscan", viewport, buffer)
	}
	if got, want := len(appMapSlice(first["windowRows"])), 3*viewport; got != want {
		t.Fatalf("initial bounded rows=%d, want %d", got, want)
	}

	// With three screens buffered, QML requests the next window after half a
	// screen of travel. Even if the response arrives only at the old ListView's
	// lower bound, that live top-row anchor is still present in the new window.
	requestTop := editor.ScrollTopRow + viewport/2
	oldBoundaryTop := appInt64(first["windowStart"]) + int64(2*viewport)
	if !editor.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(editor), "action": "editor.scroll",
		"visualRow": requestTop, "generation": uint64(1),
	}) {
		t.Fatal("editor window request was not handled")
	}
	second := editor.SemanticNode(nil)
	if overlap := assertSemanticOverlapStable(t, first, second, "rows"); overlap < 2*viewport {
		t.Fatalf("editor overlap=%d rows, want at least %d", overlap, 2*viewport)
	}
	if _, ok := semanticRowsByExtent(t, second, "rows")[oldBoundaryTop]; !ok {
		t.Fatalf("new window lost live old-boundary anchor row %d", oldBoundaryTop)
	}
}

func TestSemanticWindowProtocol_NativeViewportScrollsEditorAtFirstHiddenRow(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for row := 0; row < 40; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "viewport.txt")
	defer editor.Close()
	editor.SetPosition(0, 0, 39, 10)
	editor.SetVisible(true)
	target := vtui.SemanticID(editor)

	if !editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "document.viewport", "rows": 8,
	}) {
		t.Fatal("native editor viewport was not handled")
	}
	if got := semanticInt(editor.SemanticNode(nil)["viewportRows"]); got != 8 {
		t.Fatalf("semantic viewport rows=%d, want 8", got)
	}

	editor.ScrollTopRow = 0
	editor.CursorLine = 7
	editor.CursorPos = 0
	editor.ensureCursorVisible()
	if editor.ScrollTopRow != 0 {
		t.Fatalf("last visible row scrolled early to %d", editor.ScrollTopRow)
	}
	editor.CursorLine = 8
	editor.ensureCursorVisible()
	if editor.ScrollTopRow != 1 {
		t.Fatalf("first hidden row scrolled to %d, want 1", editor.ScrollTopRow)
	}
	cursor := editor.semanticCursorState(editor.semanticSurfaceWidth())
	if !cursor.visible || cursor.visualRow != 7 {
		t.Fatalf("cursor after immediate scroll=%#v, want visible row 7", cursor)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "document.viewport", "rows": 0,
	})
	if got := semanticInt(editor.SemanticNode(nil)["viewportRows"]); got != 10 {
		t.Fatalf("cleared native viewport rows=%d, want terminal height 10", got)
	}
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
	if got := semanticInt(viewer.SemanticNode(nil)["viewportRows"]); got != 8 {
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

func TestSemanticEditorMouseUsesCanonicalEditorPointerHandling(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	editor := NewEditorView(piecetable.New([]byte("zero\nalpha beta\ngamma\n")), nil, "mouse.txt")
	defer editor.Close()
	editor.SetPosition(3, 4, 42, 12)
	target := vtui.SemanticID(editor)

	if !editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "press",
		"button": "left", "column": 2, "row": 1,
	}) {
		t.Fatal("semantic editor mouse press was not handled")
	}
	if editor.CursorLine != 1 || editor.CursorPos != 2 || !editor.selActive {
		t.Fatalf("click cursor=(%d,%d) selection=%v, want (1,2) with anchor",
			editor.CursorLine, editor.CursorPos, editor.selActive)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "move",
		"button": "left", "column": 3, "row": 2, "moved": true,
	})
	start, end := editor.getSelectionRange()
	if editor.CursorLine != 2 || editor.CursorPos != 3 || start != 7 || end != 19 {
		t.Fatalf("drag cursor=(%d,%d) range=[%d:%d], want (2,3) [7:19]",
			editor.CursorLine, editor.CursorPos, start, end)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "press",
		"button": "left", "column": 8, "row": 1, "doubleClick": true,
	})
	start, end = editor.getSelectionRange()
	if start != 11 || end != 15 {
		t.Fatalf("double-click range=[%d:%d], want beta [11:15]", start, end)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "press",
		"button": "right", "column": 1, "row": 0,
	})
	if !editor.rectSelActive || editor.selActive {
		t.Fatalf("right click rectangular=%v stream=%v, want rectangular only",
			editor.rectSelActive, editor.selActive)
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
	viewport := semanticInt(first["viewportRows"])
	if got, want := len(appMapSlice(first["windowRows"])), 3*viewport; got != want {
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

func TestSemanticWindowProtocol_StaleGenerationsCannotMoveEditor(t *testing.T) {
	var content strings.Builder
	for row := 0; row < 100; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "generation.txt")
	editor.SetPosition(0, 0, 39, 8)
	target := vtui.SemanticID(editor)

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.scroll", "visualRow": 40,
		"generation": uint64(41),
	})
	if editor.ScrollTopRow != 0 || editor.semanticWindowGeneration != 0 || !editor.semanticPendingScroll {
		t.Fatalf("unpublished request changed top=%d generation=%d pending=%v", editor.ScrollTopRow,
			editor.semanticWindowGeneration, editor.semanticPendingScroll)
	}
	editor.SemanticNode(nil)
	if editor.ScrollTopRow != 40 || editor.semanticWindowGeneration != 41 {
		t.Fatalf("fresh request produced top=%d generation=%d", editor.ScrollTopRow,
			editor.semanticWindowGeneration)
	}
	for _, generation := range []uint64{40, 41} {
		editor.HandleSemanticAction(map[string]any{
			"target": target, "action": "editor.scroll", "visualRow": 3,
			"generation": generation,
		})
		if editor.ScrollTopRow != 40 || editor.semanticWindowGeneration != 41 {
			t.Fatalf("stale generation %d moved editor to top=%d generation=%d",
				generation, editor.ScrollTopRow, editor.semanticWindowGeneration)
		}
	}
	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.scroll", "visualRow": 1 << 20,
		"generation": uint64(42),
	})
	if editor.semanticWindowGeneration != 41 {
		t.Fatal("clamped request acknowledged before projection")
	}
	editor.SemanticNode(nil)
	if editor.semanticWindowGeneration != 42 {
		t.Fatalf("clamped no-op did not ACK exact generation 42: %d",
			editor.semanticWindowGeneration)
	}
}

func TestSemanticWindowProtocol_EditorEdgeNavigationFencesPendingScroll(t *testing.T) {
	var content strings.Builder
	for row := 0; row < 200; row++ {
		fmt.Fprintf(&content, "row-%03d alpha beta gamma\n", row)
	}
	for _, tc := range []struct {
		name  string
		key   uint16
		shift bool
		pos   int
	}{
		{name: "home", key: vtinput.VK_HOME, pos: 0},
		{name: "end", key: vtinput.VK_END, pos: len("row-045 alpha beta gamma")},
		{name: "shift-home", key: vtinput.VK_HOME, shift: true, pos: 0},
		{name: "shift-end", key: vtinput.VK_END, shift: true, pos: len("row-045 alpha beta gamma")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := NewEditorView(piecetable.New([]byte(content.String())), nil,
				"edge-navigation.txt")
			defer editor.Close()
			editor.highlighter = nil
			editor.SetPosition(0, 0, 39, 8)
			editor.SetVisible(true)
			editor.ScrollTopRow = 40
			editor.CursorLine, editor.CursorPos = 45, 8
			target := vtui.SemanticID(editor)
			initial := editor.SemanticNode(nil)
			if !semanticBool(initial["cursorVisible"]) {
				t.Fatal("initial caret is not visible")
			}

			const pendingGeneration = uint64(11)
			if !editor.HandleSemanticAction(map[string]any{
				"target": target, "action": "editor.scroll", "visualRow": 120,
				"generation": pendingGeneration,
			}) || !editor.semanticPendingScroll {
				t.Fatal("test scroll destination was not pending")
			}
			state := vtinput.ShiftPressed
			if !tc.shift {
				state = 0
			}
			if !editor.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tc.key, ControlKeyState: state,
			}) {
				t.Fatal("edge key was not handled")
			}
			if editor.semanticPendingScroll ||
				editor.semanticWindowGeneration != pendingGeneration+1 ||
				editor.semanticWindowRequestGeneration != pendingGeneration+1 {
				t.Fatalf("edge fence state: pending=%v window=%d request=%d",
					editor.semanticPendingScroll, editor.semanticWindowGeneration,
					editor.semanticWindowRequestGeneration)
			}
			if editor.CursorLine != 45 || editor.CursorPos != tc.pos ||
				editor.selActive != tc.shift {
				t.Fatalf("edge navigation result cursor=%d:%d selection=%v",
					editor.CursorLine, editor.CursorPos, editor.selActive)
			}

			ack := editor.SemanticNode(nil)
			if semanticInt64(ack["windowGeneration"]) != int64(pendingGeneration+1) ||
				!semanticBool(ack["cursorVisible"]) {
				t.Fatalf("successor ACK did not keep caret visible: generation=%v visible=%v",
					ack["windowGeneration"], ack["cursorVisible"])
			}
			if tc.shift {
				anchorRow, anchorColumn := editor.engine.LogicalToVisual(editor.selAnchorOffset)
				if !semanticBool(ack["selection"]) ||
					semanticInt64(ack["selectionAnchorRow"]) != int64(anchorRow) ||
					semanticInt(ack["selectionAnchorColumn"]) != anchorColumn ||
					semanticString(ack["selectionBackground"]) !=
						semanticAttrColor(vtui.Palette[vtui.ColDialogEditSelected], false) {
					t.Fatal("Shift edge ACK lost scalar selection geometry/style")
				}
			}

			// A delayed request/result in the generation canceled before the key
			// must not move either the viewport or selection endpoint.
			beforeTop, beforeLine, beforePos := editor.ScrollTopRow,
				editor.CursorLine, editor.CursorPos
			editor.HandleSemanticAction(map[string]any{
				"target": target, "action": "editor.scroll", "visualRow": 150,
				"generation": pendingGeneration,
			})
			late := editor.SemanticNode(nil)
			if editor.ScrollTopRow != beforeTop || editor.CursorLine != beforeLine ||
				editor.CursorPos != beforePos || editor.semanticPendingScroll ||
				semanticInt64(late["windowGeneration"]) != int64(pendingGeneration+1) ||
				editor.selActive != tc.shift {
				t.Fatalf("late scroll overrode edge result: top=%d cursor=%d:%d pending=%v generation=%v selection=%v",
					editor.ScrollTopRow, editor.CursorLine, editor.CursorPos,
					editor.semanticPendingScroll, late["windowGeneration"], editor.selActive)
			}
		})
	}
}

func TestSemanticWindowProtocol_ContentKeyIgnoresCursorAndStreamSelection(t *testing.T) {
	vtui.SetDefaultPalette()
	editor := NewEditorView(piecetable.New([]byte("alpha\nbeta\ngamma\ndelta\n")), nil,
		"content-key.txt")
	editor.SetPosition(0, 0, 39, 8)
	editor.CursorLine = 1
	first := semanticString(editor.SemanticNode(nil)["windowContentKey"])
	if first == "" {
		t.Fatal("editor did not export a window content key")
	}

	// Cursor geometry is exported separately and must not force QML to inspect
	// or replace an otherwise byte-for-byte identical row model.
	editor.CursorLine = 2
	second := semanticString(editor.SemanticNode(nil)["windowContentKey"])
	if second != first {
		t.Fatalf("cursor-only scene changed row content key: %q -> %q", first, second)
	}

	// Regular stream selection is a scalar overlay. Moving its endpoint must
	// leave the complete base-row fingerprint unchanged.
	editor.selActive = true
	editor.selAnchorOffset = editor.li.GetLineOffset(1)
	editor.CursorLine = 1
	editor.CursorPos = 3
	selected := semanticString(editor.SemanticNode(nil)["windowContentKey"])
	if selected != first {
		t.Fatalf("selection overlay changed row content key: %q -> %q", first, selected)
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
	delayed := &semanticDelayedViewerRead{ReadAtCloser: viewer.backend.file,
		started: make(chan struct{}), release: make(chan struct{}), completed: make(chan struct{})}
	viewer.backend.file = delayed
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
	if !viewer.semanticPendingScroll || viewer.semanticPendingGeneration != 7 {
		t.Fatalf("far request pending=%v generation=%d",
			viewer.semanticPendingScroll, viewer.semanticPendingGeneration)
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
	if !viewer.semanticPendingScroll || viewer.semanticWindowGeneration != 0 {
		t.Fatal("generation 8 was acknowledged before projection")
	}
	latest := viewer.SemanticNode(nil)
	if viewer.semanticPendingScroll || viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("new request state pending=%v top=%d generation=%d",
			viewer.semanticPendingScroll, viewer.TopOffset, viewer.semanticWindowGeneration)
	}

	release()
	select {
	case <-delayed.completed:
	case <-time.After(2 * time.Second):
		t.Fatal("superseded read did not finish")
	}
	deadline := time.After(2 * time.Second)
	for {
		viewer.backend.mu.Lock()
		fetching := viewer.backend.isFetching
		viewer.backend.mu.Unlock()
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
	if viewer.TopOffset != 0 || viewer.semanticPendingScroll || viewer.semanticWindowGeneration != 8 {
		t.Fatalf("stale action mutated viewer: top=%d pending=%v generation=%d",
			viewer.TopOffset, viewer.semanticPendingScroll, viewer.semanticWindowGeneration)
	}
}
