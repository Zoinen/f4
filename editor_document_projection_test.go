package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type editorProjectionReadBudget struct {
	data           []byte
	bytes, maxRead int
}

func (b *editorProjectionReadBudget) Size() int { return len(b.data) }
func (b *editorProjectionReadBudget) Read(offset, length int) ([]byte, error) {
	b.bytes += length
	b.maxRead = max(b.maxRead, length)
	return b.data[offset:min(len(b.data), offset+length)], nil
}

func TestEditorLongLineFirstNativeWindowUsesBoundedLayout(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		t.Run(map[bool]string{false: "unwrapped", true: "wrapped"}[wrap], func(t *testing.T) {
			data := bytes.Repeat([]byte("a"), 200*1024)
			ev := projectionTestEditor(t, string(data))
			buf := &editorProjectionReadBudget{data: data}
			ev.pt = piecetable.NewWithBuffer(buf)
			ev.engine.SetPointers(ev.pt, ev.li)
			ev.WordWrap = wrap
			ev.nativeViewportColumns, ev.nativeViewportRows = 80, 8
			ev.ensureEngineWidth()
			window := ev.semanticWindowMetadata()
			if !window.ready || len(window.rows) == 0 {
				t.Fatalf("first window not coherent: %+v", window)
			}
			rows := semanticStyledEditorWindowRows(ev, window, 80)
			if len(rows) != len(window.rows) {
				t.Fatalf("projected %d of %d rows", len(rows), len(window.rows))
			}
			if buf.bytes > 24*1024 || buf.maxRead > 4096 {
				t.Fatalf("first viewport read whole source: bytes=%d maxRead=%d", buf.bytes, buf.maxRead)
			}
			if wrap {
				if _, complete := ev.engine.KnownVisualRows(); complete {
					t.Fatal("first viewport falsely completed the extent")
				}
			}
			ev.CursorPos = 90 * 1024
			ev.updateDesiredVisualCol()
			ev.ensureCursorVisible()
			if ev.CursorPos != 90*1024 {
				t.Fatalf("caret clamped beyond 64KiB: %d", ev.CursorPos)
			}
			row, column := ev.engine.LogicalToVisual(ev.CursorPos)
			if offset := ev.engine.VisualToLogical(row, column); offset != ev.CursorPos {
				t.Fatalf("far caret mapped to %d", offset)
			}
		})
	}
}

func projectionTestEditor(t *testing.T, text string) *EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "")
	t.Cleanup(ev.Close)
	ev.SetPosition(0, 0, 20, 6)
	ev.SetVisible(true)
	return ev
}

func TestEditorSemanticProjectionPreservesClippedWideContinuation(t *testing.T) {
	data := append([]byte("界B\tC"), 0xff)
	data = append(data, 'D')
	ev := projectionTestEditor(t, string(data))
	ev.highlighter = nil
	ev.WordWrap = false
	ev.TabSize = 4
	ev.nativeViewportColumns, ev.nativeViewportRows = 8, 1
	ev.ScrollLeft = 1 // The first visible cell is the second half of 界.
	ev.ensureEngineWidth()

	window := ev.semanticWindowMetadata()
	rows := semanticRenderStyledEditorWindowRows(ev, window, ev.viewportWidth())
	if len(rows) != 1 {
		t.Fatalf("projected %d rows, want 1", len(rows))
	}
	var text strings.Builder
	for _, run := range rows[0].Runs {
		text.WriteString(run.Text)
	}
	got := text.String()
	if !strings.HasPrefix(got, " B C?D") {
		t.Fatalf("clipped wide/tab/invalid-byte row shifted: %q", got)
	}
	if width := vtui.StringWidth(got); width != ev.viewportWidth() {
		t.Fatalf("semantic row occupies %d columns, want %d: %q", width, ev.viewportWidth(), got)
	}
}

func TestSemanticRunsFromCellsPreservesCompositeClustersAndContinuations(t *testing.T) {
	for _, tc := range []struct {
		name, cluster string
	}{
		{name: "emoji-grapheme", cluster: "👨\u200d👩\u200d👦"},
		// RegisterCluster accepts an already-shaped multi-column cell. Its
		// continuation count is not inherently limited to one, even though
		// ordinary CJK and emoji glyphs occupy two columns.
		{name: "multiple-continuations", cluster: "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const attr = uint64(7)
			cells := vtui.AppendCluster(nil, tc.cluster, vtui.ClusterWidth(tc.cluster), attr)
			cells = append(cells, vtui.CharInfo{Char: 'Z', Attributes: attr})

			var text strings.Builder
			for _, run := range semanticRunsFromCells(cells) {
				text.WriteString(run.Text)
			}
			if got := text.String(); got != tc.cluster+"Z" {
				t.Fatalf("composite cluster continuations shifted the row: %q", got)
			}
			if width := vtui.StringWidth(text.String()); width != len(cells) {
				t.Fatalf("semantic row occupies %d columns, want %d", width, len(cells))
			}
		})
	}
}

func TestEditorNativeDimensionsExceedConsole(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("a", 90))
	ev.WordWrap = true
	ev.nativeViewportColumns, ev.nativeViewportRows = 45, 40
	ev.ensureEngineWidth()
	if ev.viewportWidth() != 45 || ev.viewportHeight() != 40 {
		t.Fatalf("native dimensions clamped to console: %dx%d", ev.viewportWidth(), ev.viewportHeight())
	}
	frags := ev.engine.GetFragments(0)
	if len(frags) != 2 || frags[0].VisualWidth != 45 {
		t.Fatalf("first native layout did not use native width: %+v", frags)
	}
	ev.ScrollTopRow = 1
	anchor := ev.engine.VisualToLogical(ev.ScrollTopRow, 0)
	ev.nativeViewportColumns = 15
	ev.ensureEngineWidth()
	if got := ev.engine.VisualToLogical(ev.ScrollTopRow, 0); got != anchor {
		t.Fatalf("reflow moved source anchor from %d to %d", anchor, got)
	}
}

func TestEditorNativeProjectionNotClippedByConsoleFrame(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("x", 45)+"\n"+strings.Repeat("y", 45))
	ev.nativeViewportColumns, ev.nativeViewportRows = 45, 2
	ev.ensureEngineWidth()
	x2, y2 := ev.X2, ev.Y2
	window := ev.semanticWindow()
	rows := semanticStyledEditorWindowRows(ev, window, 45)
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	for index, row := range rows {
		var text strings.Builder
		for _, run := range row.Runs {
			text.WriteString(run.Text)
		}
		want := strings.Repeat(string(rune('x'+index)), 45)
		if text.String() != want {
			t.Fatalf("row %d clipped to console: %q", index, text.String())
		}
	}
	if ev.X2 != x2 || ev.Y2 != y2 {
		t.Fatal("offscreen projection leaked its dimensions into console state")
	}
}

func TestEditorDocumentPointerUsesDisplayedSourceAndHorizontalOrigin(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("x", 120)+"\nshort\n\n"+strings.Repeat("y", 120))
	ev.nativeViewportColumns = 60
	ev.ensureEngineWidth()
	ev.ScrollLeft = 50
	ev.semanticPendingScroll, ev.semanticPendingTop = true, 500
	revision := ev.semanticLayoutRevision
	down := &vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed}
	if !ev.processDocumentPointer(down, 0, 35, 50, revision) || ev.CursorPos != 85 {
		t.Fatalf("native hit clipped to console, cursor=%d", ev.CursorPos)
	}
	if ev.semanticPendingScroll {
		t.Fatal("pointer left an obsolete scrollbar destination pending")
	}
	move := &vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved}
	for _, line := range []int{1, 2, 3} {
		if !ev.processDocumentPointer(move, int64(ev.li.GetLineOffset(line)), 8, 50, revision) {
			t.Fatalf("line %d drag rejected", line)
		}
		if ev.ScrollLeft != 50 {
			t.Fatalf("drag through line %d changed horizontal origin to %d", line, ev.ScrollLeft)
		}
	}
	if ev.CursorLine != 3 || ev.CursorPos != 58 {
		t.Fatalf("wrong source hit after short lines: %d:%d", ev.CursorLine, ev.CursorPos)
	}
	// A newer server scroll must not reinterpret a pointer tied to the frame
	// still displayed by Qt.
	ev.ScrollLeft = 70
	if !ev.processDocumentPointer(move, 0, 8, 50, revision) || ev.CursorPos != 58 {
		t.Fatalf("used live scroll instead of displayed origin: pos=%d", ev.CursorPos)
	}
	ev.nativeViewportColumns = 61
	ev.ensureEngineWidth()
	if ev.processDocumentPointer(move, 0, 9, 50, revision) {
		t.Fatal("accepted a drag after source layout changed")
	}
	ev.processDocumentPointer(&vtinput.InputEvent{Type: vtinput.MouseEventType}, -1, 0, 0, revision)
	if ev.semanticPointerActive {
		t.Fatal("stale release did not release native pointer capture")
	}
}

func TestEditorDocumentPointerNoOpDoesNotRedraw(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("x", 120)+"\nshort\n\n")
	ev.ScrollLeft = 50
	revision := ev.semanticLayoutRevision
	down := &vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed}
	move := &vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved}
	ev.processDocumentPointer(down, 0, 8, 50, revision)
	for _, line := range []int{1, 2} {
		rowOffset := int64(ev.li.GetLineOffset(line))
		if !ev.processDocumentPointer(move, rowOffset, 8, 50, revision) {
			t.Fatalf("selection endpoint on line %d was not row-bearing", line)
		}
		for len(vtui.FrameManager.RedrawChan) > 0 {
			<-vtui.FrameManager.RedrawChan
		}
		// Different displayed columns still clamp to this same short/empty
		// source line; neither selection nor viewport changed.
		if ev.processDocumentPointer(move, rowOffset, 10, 50, revision) || len(vtui.FrameManager.RedrawChan) != 0 {
			t.Fatalf("identical source endpoint on line %d redrew", line)
		}
		if ev.ScrollLeft != 50 || !ev.semanticPointerActive {
			t.Fatal("no-op suppression changed horizontal origin or capture")
		}
	}
	if ev.processDocumentPointer(&vtinput.InputEvent{Type: vtinput.MouseEventType}, -1, 0, 0, revision-1) || ev.semanticPointerActive {
		t.Fatal("stale release must clear capture without repainting an unchanged selection")
	}
}

func TestEditorDocumentPointerCursorOnlyUsesScalarPatch(t *testing.T) {
	previousCrosshair := AppConfig.EditorCrosshair
	AppConfig.EditorCrosshair = false
	t.Cleanup(func() { AppConfig.EditorCrosshair = previousCrosshair })
	ev := projectionTestEditor(t, "abcdefghij\nsecond row\n")
	ev.highlighter = nil
	initial := ev.SemanticNode(&vtui.SemanticContext{})
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send: &extUiMessageSender{w: &wire}, nativeSemanticSurfaceEnabled: true,
	}
	vtui.FrameManager.Screen().Renderer = renderer
	renderer.SetSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": extui.SceneVersion,
		"width": 21, "height": 7, "surface": initial,
	})
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)
	beforeRows := ev.semanticStyledRowsRendered
	action := map[string]any{
		"action": "editor.mouse", "target": vtui.SemanticID(ev),
		"documentKey": vtui.SemanticID(ev), "layoutRevision": ev.semanticLayoutRevision,
		"rowOffset": int64(0), "scrollLeft": 0, "column": 3,
		"button": "middle", "phase": "press",
	}
	renderer.BeginSemanticSceneUpdate()
	if !ev.HandleSemanticAction(action) || ev.CursorPos != 3 {
		t.Fatal("cursor-only document pointer action was not applied")
	}
	patch, err := extUiReadMessage(&wire)
	if err != nil {
		t.Fatalf("cursor-only pointer did not send its scalar state: %v", err)
	}
	if patch["type"] != "scene_patch" || semanticValueContainsKey(patch, "rows") || semanticValueContainsKey(patch, "windowRows") {
		t.Fatalf("cursor-only pointer rebuilt document content: %#v", patch)
	}
	if ev.semanticStyledRowsRendered != beforeRows {
		t.Fatal("cursor-only pointer reprojected document rows")
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(1)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(1) {
		t.Fatal("cursor-only pointer did not suppress its redundant full render")
	}
	renderer.BeginSemanticSceneUpdate()
	if !ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) || ev.CursorPos != 4 {
		t.Fatal("keyboard caret navigation was not applied")
	}
	patch, err = extUiReadMessage(&wire)
	if err != nil || semanticValueContainsKey(patch, "rows") || semanticValueContainsKey(patch, "windowRows") || ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("keyboard caret update did not remain row-free: patch=%#v error=%v", patch, err)
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(2)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(2) {
		t.Fatal("keyboard caret update did not defer the document render")
	}
	// Shift extends regular selection entirely through the scalar overlay. The
	// immutable base rows and their styled-run delegates stay untouched.
	renderer.BeginSemanticSceneUpdate()
	if !ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.ShiftPressed}) ||
		ev.CursorPos != 5 || !ev.selActive {
		t.Fatal("Shift+Right did not extend stream selection")
	}
	patch, err = extUiReadMessage(&wire)
	selectionSet := appMap(appMap(patch["surface"])["set"])
	if err != nil || semanticValueContainsKey(patch, "rows") ||
		semanticValueContainsKey(patch, "windowRows") ||
		!semanticBool(selectionSet["selection"]) ||
		semanticInt(selectionSet["selectionAnchorColumn"]) != 4 ||
		semanticInt(selectionSet["cursorAbsoluteColumn"]) != 5 ||
		ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("Shift selection did not remain row-free: patch=%#v error=%v", patch, err)
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(3)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(3) {
		t.Fatal("Shift selection did not defer the redundant row render")
	}
	// An ordinary move clears the selection explicitly; compact overrides are
	// replaced rather than merged, so inactive state must travel too.
	renderer.BeginSemanticSceneUpdate()
	if !ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_LEFT}) || ev.CursorPos != 4 || ev.selActive {
		t.Fatal("plain Left did not clear stream selection")
	}
	patch, err = extUiReadMessage(&wire)
	selectionSet = appMap(appMap(patch["surface"])["set"])
	selectionValue, selectionPresent := selectionSet["selection"]
	if err != nil || !selectionPresent || semanticBool(selectionValue) ||
		semanticValueContainsKey(patch, "rows") || ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("selection clear did not remain explicit and row-free: patch=%#v error=%v", patch, err)
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(4)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(4) {
		t.Fatal("selection clear did not defer the redundant row render")
	}

	// Left-button drag uses the same scalar selection publication.
	action["button"], action["column"] = "left", 4
	renderer.BeginSemanticSceneUpdate()
	ev.HandleSemanticAction(action)
	patch, err = extUiReadMessage(&wire)
	if !ev.selActive || err != nil || semanticValueContainsKey(patch, "rows") {
		t.Fatalf("selection start did not use row-free publication: patch=%#v error=%v", patch, err)
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(5)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(5) {
		t.Fatal("selection start did not defer the redundant row render")
	}
	action["moved"], action["column"] = true, 7
	renderer.BeginSemanticSceneUpdate()
	ev.HandleSemanticAction(action)
	patch, err = extUiReadMessage(&wire)
	selectionSet = appMap(appMap(patch["surface"])["set"])
	installedSurface := appMap(renderer.lastScene["surface"])
	if ev.CursorPos != 7 || err != nil || !semanticBool(installedSurface["selection"]) ||
		semanticInt(selectionSet["cursorAbsoluteColumn"]) != 7 ||
		semanticValueContainsKey(patch, "rows") ||
		ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("drag endpoint did not stay row-free: patch=%#v error=%v", patch, err)
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(6)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(6) {
		t.Fatal("selection drag did not defer the redundant row render")
	}
	// The explicit release phase wins over a stale client's button field.
	action["phase"], action["layoutRevision"] = "release", uint64(0)
	ev.HandleSemanticAction(action)
	if ev.semanticPointerActive || !ev.selActive {
		t.Fatal("stale release lost selection or retained pointer capture")
	}
	// Identity fields validate the currently installed layout; they are not
	// scalar mutations which could overwrite a newer document's identity.
	ev.semanticLayoutRevision++
	renderer.BeginSemanticSceneUpdate()
	if ev.queueSemanticCursorState() || wire.Len() != 0 {
		t.Fatal("cursor state crossed an uncommitted layout transition")
	}
	renderer.EndSemanticSceneUpdate()
}

func TestEditorWordWrapActionKeepsCursorVisibleAfterLongLines(t *testing.T) {
	longLine := strings.Repeat("1234567890\t界xy ", 800)
	ev := projectionTestEditor(t, longLine+"\nshort\n\n"+longLine+"\nshort\n")
	ev.highlighter = nil
	ev.nativeViewportColumns, ev.nativeViewportRows = 302, 46
	vtui.FrameManager.AddScreen(ev)
	ev.CursorLine, ev.CursorPos, ev.ScrollLeft = 4, 5, 500
	ev.ensureEngineWidth()
	anchor := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	for _, wrapped := range []bool{true, false} {
		if !RunAction("Editor.WordWrap") || ev.WordWrap != wrapped {
			t.Fatalf("wrap action did not set wrap=%v", wrapped)
		}
		node := ev.SemanticNode(&vtui.SemanticContext{})
		if got := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos; got != anchor {
			t.Fatalf("wrap=%v changed cursor source anchor: got %d want %d", wrapped, got, anchor)
		}
		row := semanticInt(node["cursorVisualRow"])
		if row < 0 || row >= ev.viewportHeight() || !semanticBool(node["cursorVisible"]) {
			t.Fatalf("wrap=%v left cursor outside native viewport: row=%d top=%d", wrapped, row, ev.ScrollTopRow)
		}
	}
}

func TestEditorConsolePointerPreservesHorizontalOrigin(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("x", 120)+"\na\n\n")
	ev.ScrollLeft = 50
	for row := 1; row <= 3; row++ {
		flags := uint32(0)
		if row > 1 {
			flags = vtinput.MouseMoved
		}
		ev.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType,
			ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: flags,
			MouseX: 8, MouseY: int16(row)})
		if ev.ScrollLeft != 50 {
			t.Fatalf("row %d reset scroll to %d", row, ev.ScrollLeft)
		}
	}
}

func TestEditorWrappedTabProjectionMatchesNavigation(t *testing.T) {
	ev := projectionTestEditor(t, "123\t")
	ev.WordWrap, ev.TabSize, ev.nativeViewportColumns = true, 4, 2
	ev.ensureEngineWidth()
	frag := ev.engine.GetFragments(0)[1]
	data, _ := ev.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
	cells := ev.fillCellsSpan(nil, data, 1, 2, frag.ByteOffsetStart, false, 0, 0, nil, 0, false, -1, 0, 0, 1, frag.VisualColumnStart, 0, 10)
	if len(cells) != frag.VisualWidth || len(cells) != 2 {
		t.Fatalf("tab rendered %d columns but wrapping/navigation say %d", len(cells), frag.VisualWidth)
	}
}

func TestEditorProjectionAllocatesOnlyVisibleColumns(t *testing.T) {
	ev := projectionTestEditor(t, "")
	data := []byte(strings.Repeat("x", 64*1024))
	cells := ev.fillCellsSpan(nil, data, 1, 2, 0, false, 0, 0, nil, 0, false, -1, 0, 0, 0, 0, 120, 200)
	if len(cells) != 80 || cap(cells) > 160 {
		t.Fatalf("projection allocated off-screen text: len=%d cap=%d", len(cells), cap(cells))
	}
}

func TestEditorNativeWrappedExtentEventuallyCompletes(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("12345678901234567890\n", 1000))
	ev.WordWrap, ev.nativeViewportColumns = true, 10
	ev.ensureEngineWidth()
	ev.engine.GetLogLineAtVisualRow(10)
	if _, complete := ev.engine.KnownVisualRows(); complete {
		t.Fatal("first viewport synchronously laid out the entire document")
	}
	ev.scheduleNativeVisualExtent()
	deadline := time.After(2 * time.Second)
	for {
		if rows, complete := ev.engine.KnownVisualRows(); complete {
			if rows != 2001 {
				t.Fatalf("wrapped extent=%d, want 2001", rows)
			}
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("wrapped extent never became available for the scrollbar")
		}
	}
}

func TestEditorNativeWrappedExtentRejectsStaleLayoutTask(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("12345678901234567890\n", 1000))
	drainPendingTasks()
	ev.WordWrap, ev.nativeViewportColumns = true, 10
	ev.ensureEngineWidth()
	ev.engine.GetLogLineAtVisualRow(10)
	ev.scheduleNativeVisualExtent()
	var task func()
	select {
	case task = <-vtui.FrameManager.TaskChan:
	case <-time.After(2 * time.Second):
		t.Fatal("extent task not posted")
	}
	ev.nativeViewportColumns = 5
	ev.ensureEngineWidth()
	before, _ := ev.engine.KnownVisualRows()
	task()
	if after, _ := ev.engine.KnownVisualRows(); after != before {
		t.Fatalf("old layout task advanced new layout from %d to %d", before, after)
	}
}

type failingEditorIndexReader struct {
	vfs.ReadAtCloser
	failAt int64
}

func (f failingEditorIndexReader) ReadAt(ctx context.Context, dst []byte, offset int64) (int, error) {
	if offset >= f.failAt {
		return 0, errors.New("index source disconnected")
	}
	return f.ReadAtCloser.ReadAt(ctx, dst, offset)
}

func TestEditorIndexerFailureKeepsReadyOffsetsAndPendingTarget(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()
	data := []byte(strings.Repeat(strings.Repeat("x", 64)+"\n", 3000))
	file := failingEditorIndexReader{ReadAtCloser: &documentRangeReader{data: data}, failAt: 64 * 1024}
	buf := NewAsyncBuffer(context.Background(), file)
	buf.seedPrefix(data[:buf.chunkSize])
	ev := NewEditorView(piecetable.NewWithBuffer(buf), nil, "")
	t.Cleanup(ev.Close)
	ev.asyncBuf, ev.file = buf, file
	ev.targetLine, ev.targetPos = 2000, 0
	initialLines := ev.li.LineCount()
	ev.StartIndexing()
	deadline := time.After(3 * time.Second)
	for ev.indexing {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("indexer did not report source failure")
		}
	}
	if ev.semanticLoadError == "" || ev.semanticExtentKnown {
		t.Fatalf("failure became a complete extent: error=%q known=%v", ev.semanticLoadError, ev.semanticExtentKnown)
	}
	if ev.li.LineCount() <= initialLines+400 {
		t.Fatalf("ready offsets discarded: before=%d after=%d", initialLines, ev.li.LineCount())
	}
	if ev.targetLine != 2000 {
		t.Fatalf("failed read incorrectly restored target at %d", ev.CursorLine)
	}
}

func BenchmarkEditorVisibleRowProjection(b *testing.B) {
	ev := &EditorView{TabSize: 4}
	data := []byte(strings.Repeat("x", 64*1024))
	for _, tc := range []struct {
		name string
		end  int
	}{
		{"full_line_before_clipping", len(data)}, {"visible_span", 200},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var cells []vtui.CharInfo
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				cells = ev.fillCellsSpan(cells, data, 1, 2, 0, false, 0, 0, nil, 0, false, -1, 0, 0, 0, 0, 120, tc.end)
			}
		})
	}
}
