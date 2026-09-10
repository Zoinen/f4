package editor

import (
	"bytes"
	"context"
	"errors"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"strings"
	"testing"
	"time"
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
			ev.Pt = piecetable.NewWithBuffer(buf)
			ev.Engine.SetPointers(ev.Pt, ev.Li)
			ev.WordWrap = wrap
			ev.NativeViewportColumns, ev.NativeViewportRows = 80, 8
			ev.EnsureEngineWidth()
			window := ev.semanticWindowMetadata()
			if !window.Ready || len(window.Rows) == 0 {
				t.Fatalf("first window not coherent: %+v", window)
			}
			rows := semanticStyledEditorWindowRows(ev, window, 80)
			if len(rows) != len(window.Rows) {
				t.Fatalf("projected %d of %d rows", len(rows), len(window.Rows))
			}
			if buf.bytes > 24*1024 || buf.maxRead > 4096 {
				t.Fatalf("first viewport read whole source: bytes=%d maxRead=%d", buf.bytes, buf.maxRead)
			}
			if wrap {
				if _, complete := ev.Engine.KnownVisualRows(); complete {
					t.Fatal("first viewport falsely completed the extent")
				}
			}
			ev.CursorPos = 90 * 1024
			ev.updateDesiredVisualCol()
			ev.EnsureCursorVisible()
			if ev.CursorPos != 90*1024 {
				t.Fatalf("caret clamped beyond 64KiB: %d", ev.CursorPos)
			}
			row, column := ev.Engine.LogicalToVisual(ev.CursorPos)
			if offset := ev.Engine.VisualToLogical(row, column); offset != ev.CursorPos {
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
	ev.Highlighter = nil
	ev.WordWrap = false
	ev.TabSize = 4
	ev.NativeViewportColumns, ev.NativeViewportRows = 8, 1
	ev.ScrollLeft = 1 // The first visible cell is the second half of 界.
	ev.EnsureEngineWidth()

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
			cells := vtui.AppendCluster(nil, tc.cluster, vtui.StringWidth(tc.cluster), attr)
			cells = append(cells, vtui.CharInfo{Char: 'Z', Attributes: attr})

			var text strings.Builder
			for _, run := range semantic.RunsFromCells(cells) {
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
	ev.NativeViewportColumns, ev.NativeViewportRows = 45, 40
	ev.EnsureEngineWidth()
	if ev.viewportWidth() != 45 || ev.ViewportHeight() != 40 {
		t.Fatalf("native dimensions clamped to console: %dx%d", ev.viewportWidth(), ev.ViewportHeight())
	}
	frags := ev.Engine.GetFragments(0)
	if len(frags) != 2 || frags[0].VisualWidth != 45 {
		t.Fatalf("first native layout did not use native width: %+v", frags)
	}
	ev.ScrollTopRow = 1
	anchor := ev.Engine.VisualToLogical(ev.ScrollTopRow, 0)
	ev.NativeViewportColumns = 15
	ev.EnsureEngineWidth()
	if got := ev.Engine.VisualToLogical(ev.ScrollTopRow, 0); got != anchor {
		t.Fatalf("reflow moved source anchor from %d to %d", anchor, got)
	}
}

func TestEditorNativeProjectionNotClippedByConsoleFrame(t *testing.T) {
	ev := projectionTestEditor(t, strings.Repeat("x", 45)+"\n"+strings.Repeat("y", 45))
	ev.NativeViewportColumns, ev.NativeViewportRows = 45, 2
	ev.EnsureEngineWidth()
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
	ev.NativeViewportColumns = 60
	ev.EnsureEngineWidth()
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
		if !ev.processDocumentPointer(move, int64(ev.Li.GetLineOffset(line)), 8, 50, revision) {
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
	ev.NativeViewportColumns = 61
	ev.EnsureEngineWidth()
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
		rowOffset := int64(ev.Li.GetLineOffset(line))
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
	oldMarkOccurrences := config.App.EditorMarkOccurrences
	config.App.EditorMarkOccurrences = false
	t.Cleanup(func() { config.App.EditorMarkOccurrences = oldMarkOccurrences })
	previousCrosshair := config.App.EditorCrosshair
	config.App.EditorCrosshair = false
	t.Cleanup(func() { config.App.EditorCrosshair = previousCrosshair })
	ev := projectionTestEditor(t, "abcdefghij\nsecond row\n")
	ev.Highlighter = nil
	renderer := &editorSurfaceRecorder{}
	vtui.FrameManager.Screen().Renderer = renderer
	beforeRows := ev.semanticStyledRowsRendered
	action := map[string]any{
		"action": "editor.mouse", "target": vtui.SemanticID(ev),
		"documentKey": vtui.SemanticID(ev), "layoutRevision": ev.semanticLayoutRevision,
		"rowOffset": int64(0), "scrollLeft": 0, "column": 3,
		"button": "middle", "phase": "press",
	}
	if !ev.HandleSemanticAction(action) || ev.CursorPos != 3 {
		t.Fatal("cursor-only document pointer action was not applied")
	}
	patch, err := renderer.takePatch()
	if err != nil {
		t.Fatalf("cursor-only pointer did not send its scalar state: %v", err)
	}
	if patch["type"] != "scene_patch" || semanticValueContainsKey(patch, "rows") || semanticValueContainsKey(patch, "windowRows") {
		t.Fatalf("cursor-only pointer rebuilt document content: %#v", patch)
	}
	if ev.semanticStyledRowsRendered != beforeRows {
		t.Fatal("cursor-only pointer reprojected document rows")
	}
	if !ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) || ev.CursorPos != 4 {
		t.Fatal("keyboard caret navigation was not applied")
	}
	patch, err = renderer.takePatch()
	if err != nil || semanticValueContainsKey(patch, "rows") || semanticValueContainsKey(patch, "windowRows") || ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("keyboard caret update did not remain row-free: patch=%#v error=%v", patch, err)
	}
	// Shift extends regular selection entirely through the scalar overlay. The
	// immutable base rows and their styled-run delegates stay untouched.
	if !ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.ShiftPressed}) ||
		ev.CursorPos != 5 || !ev.SelActive {
		t.Fatal("Shift+Right did not extend stream selection")
	}
	patch, err = renderer.takePatch()
	selectionSet := semantic.AppMap(semantic.AppMap(patch["surface"])["set"])
	if err != nil || semanticValueContainsKey(patch, "rows") ||
		semanticValueContainsKey(patch, "windowRows") ||
		!semantic.Bool(selectionSet["selection"]) ||
		semantic.Int(selectionSet["selectionAnchorColumn"]) != 4 ||
		semantic.Int(selectionSet["cursorAbsoluteColumn"]) != 5 ||
		ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("Shift selection did not remain row-free: patch=%#v error=%v", patch, err)
	}
	// An ordinary move clears the selection explicitly; compact overrides are
	// replaced rather than merged, so inactive state must travel too.
	if !ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_LEFT}) || ev.CursorPos != 4 || ev.SelActive {
		t.Fatal("plain Left did not clear stream selection")
	}
	patch, err = renderer.takePatch()
	selectionSet = semantic.AppMap(semantic.AppMap(patch["surface"])["set"])
	selectionValue, selectionPresent := selectionSet["selection"]
	if err != nil || !selectionPresent || semantic.Bool(selectionValue) ||
		semanticValueContainsKey(patch, "rows") || ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("selection clear did not remain explicit and row-free: patch=%#v error=%v", patch, err)
	}

	// Left-button drag uses the same scalar selection publication.
	action["button"], action["column"] = "left", 4
	ev.HandleSemanticAction(action)
	patch, err = renderer.takePatch()
	if !ev.SelActive || err != nil || semanticValueContainsKey(patch, "rows") {
		t.Fatalf("selection start did not use row-free publication: patch=%#v error=%v", patch, err)
	}
	action["moved"], action["column"] = true, 7
	ev.HandleSemanticAction(action)
	patch, err = renderer.takePatch()
	selectionSet = semantic.AppMap(semantic.AppMap(patch["surface"])["set"])
	installedSurface := selectionSet
	if ev.CursorPos != 7 || err != nil || !semantic.Bool(installedSurface["selection"]) ||
		semantic.Int(selectionSet["cursorAbsoluteColumn"]) != 7 ||
		semanticValueContainsKey(patch, "rows") ||
		ev.semanticStyledRowsRendered != beforeRows {
		t.Fatalf("drag endpoint did not stay row-free: patch=%#v error=%v", patch, err)
	}
	// The explicit release phase wins over a stale client's button field.
	action["phase"], action["layoutRevision"] = "release", uint64(0)
	ev.HandleSemanticAction(action)
	if ev.semanticPointerActive || !ev.SelActive {
		t.Fatal("stale release lost selection or retained pointer capture")
	}
	// The transport checks this layout identity against its installed surface;
	// the editor must include the new identity in its next scalar update.
	ev.semanticLayoutRevision++
	if !ev.queueSemanticCursorState() {
		t.Fatal("cursor state did not report its new layout identity")
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
	ev.WordWrap, ev.TabSize, ev.NativeViewportColumns = true, 4, 2
	ev.EnsureEngineWidth()
	frag := ev.Engine.GetFragments(0)[1]
	data, _ := ev.Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
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
	ev.WordWrap, ev.NativeViewportColumns = true, 10
	ev.EnsureEngineWidth()
	ev.Engine.GetLogLineAtVisualRow(10)
	if _, complete := ev.Engine.KnownVisualRows(); complete {
		t.Fatal("first viewport synchronously laid out the entire document")
	}
	ev.scheduleNativeVisualExtent()
	deadline := time.After(2 * time.Second)
	for {
		if rows, complete := ev.Engine.KnownVisualRows(); complete {
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
	testutil.DrainPendingTasks()
	ev.WordWrap, ev.NativeViewportColumns = true, 10
	ev.EnsureEngineWidth()
	ev.Engine.GetLogLineAtVisualRow(10)
	ev.scheduleNativeVisualExtent()
	var task func()
	select {
	case task = <-vtui.FrameManager.TaskChan:
	case <-time.After(2 * time.Second):
		t.Fatal("extent task not posted")
	}
	ev.NativeViewportColumns = 5
	ev.EnsureEngineWidth()
	before, _ := ev.Engine.KnownVisualRows()
	task()
	if after, _ := ev.Engine.KnownVisualRows(); after != before {
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
	testutil.DrainPendingTasks()
	data := []byte(strings.Repeat(strings.Repeat("x", 64)+"\n", 3000))
	file := failingEditorIndexReader{ReadAtCloser: &documentRangeReader{data: data}, failAt: 64 * 1024}
	buf := NewAsyncBuffer(context.Background(), file)
	buf.SeedPrefix(data[:buf.ChunkSize])
	ev := NewEditorView(piecetable.NewWithBuffer(buf), nil, "")
	t.Cleanup(ev.Close)
	ev.AsyncBuf, ev.File = buf, file
	ev.TargetLine, ev.TargetPos = 2000, 0
	initialLines := ev.Li.LineCount()
	ev.StartIndexing()
	deadline := time.After(3 * time.Second)
	for ev.Indexing {
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
	if ev.Li.LineCount() <= initialLines+400 {
		t.Fatalf("ready offsets discarded: before=%d after=%d", initialLines, ev.Li.LineCount())
	}
	if ev.TargetLine != 2000 {
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

func TestEditorNativePointerAddsAndClearsSecondaryCaret(t *testing.T) {
	ev := projectionTestEditor(t, "first\nsecond")
	ev.EnsureEngineWidth()
	guard := ev.editorCursorStateGuard()
	click := &vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, ControlKeyState: vtinput.LeftAltPressed}
	if !ev.processDocumentPointer(click, 6, 2, 0, ev.semanticLayoutRevision) {
		t.Fatal("Alt+click did not change native caret state")
	}
	if len(ev.extraCursors) != 1 || ev.extraCursors[0].off != 8 || ev.caretOffset() != 0 {
		t.Fatalf("native Alt+click moved primary or missed secondary: primary=%d secondary=%v", ev.caretOffset(), extraCaretOffsets(ev))
	}
	if !guard.canPublish(ev, true) {
		t.Fatal("secondary caret cannot publish compact state")
	}
	style := ev.textProjectionStyle()
	style.paintStreamSelection = true
	found := false
	ev.projectTextRows(0, 20, 2, style, func(row editorProjectedTextRow) {
		if row.visualRow == 1 {
			found = row.cells[2].Attributes == style.selected
		}
	})
	if !found {
		t.Fatal("secondary caret missing from console projection")
	}
	click.ControlKeyState = 0
	ev.processDocumentPointer(click, 6, 1, 0, ev.semanticLayoutRevision)
	if len(ev.extraCursors) != 0 || ev.caretOffset() != 7 {
		t.Fatal("plain click did not restore a single caret")
	}
}
