package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func cachedSemanticViewer(data []byte) *ViewerView {
	file := &vfs.MemoryReadAtCloser{Data: data}
	backend := &ViewerBackend{
		file:         file,
		size:         int64(len(data)),
		totalLines:   -1,
		totalForSize: -1,
		cacheData:    data,
		ctx:          context.Background(),
	}
	viewer := &ViewerView{
		backend:  backend,
		WrapMode: true,
	}
	viewer.scrollBar = vtui.NewScrollBar(0, 0, 0)
	viewer.SetPosition(0, 0, 39, 8)
	viewer.SetVisible(true)
	return viewer
}

func TestSemanticStyledViewerWindowRowsMatchesTextRendererAndRestoresState(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	var content strings.Builder
	var offsets []int64
	for index := 0; index < 48; index++ {
		offsets = append(offsets, int64(content.Len()))
		fmt.Fprintf(&content, "line-%02d\twide界-%02d\n", index, index)
	}
	viewer := cachedSemanticViewer([]byte(content.String()))
	viewer.TopOffset = offsets[20]
	width := viewer.semanticContentWidth()
	window := viewer.semanticWindow()

	expected := semanticRenderSurface(viewer.X1, viewer.Y1+1,
		viewer.X1+width-1, viewer.Y2, viewer.DisplayObject)
	viewer.TopOffset = offsets[20]
	viewer.lineOffsets = []int64{701, 709, 719}
	viewer.eofVisible = true
	viewer.lastKnownSize = int64(content.Len())
	viewer.scrollBar.Value = 17
	viewer.scrollBar.Min = 3
	viewer.scrollBar.Max = 91
	viewer.scrollBar.PgStep = 7
	viewer.scrollBar.SetVisible(false)

	styled := semanticStyledViewerWindowRows(viewer, window, width)
	if len(styled) != len(window.rows) {
		t.Fatalf("styled rows=%d, want %d", len(styled), len(window.rows))
	}
	for index, row := range styled {
		if len(row.Runs) == 0 {
			t.Fatalf("semantic text window row %d has no styled runs", index)
		}
	}
	for index := 0; index < window.viewportRows; index++ {
		rowIndex := window.viewportRow + index
		if rowIndex >= len(styled) || index >= len(expected.Rows) {
			break
		}
		if !reflect.DeepEqual(styled[rowIndex].Runs, expected.Rows[index]) {
			t.Fatalf("styled viewer row %d differs from DisplayObject\nwindow: %#v\nvisible: %#v",
				rowIndex, styled[rowIndex].Runs, expected.Rows[index])
		}
	}

	if viewer.TopOffset != offsets[20] ||
		!reflect.DeepEqual(viewer.lineOffsets, []int64{701, 709, 719}) ||
		!viewer.eofVisible || viewer.lastKnownSize != int64(content.Len()) {
		t.Fatalf("viewer render leaked state: top=%d offsets=%v eof=%v size=%d",
			viewer.TopOffset, viewer.lineOffsets, viewer.eofVisible, viewer.lastKnownSize)
	}
	if viewer.scrollBar.Value != 17 || viewer.scrollBar.Min != 3 ||
		viewer.scrollBar.Max != 91 || viewer.scrollBar.PgStep != 7 || viewer.scrollBar.IsVisible() {
		t.Fatalf("viewer scrollbar was changed: %#v", viewer.scrollBar)
	}
	if strings.Contains(styled[window.viewportRow].Runs[0].Text, "\t") {
		t.Fatal("viewer tab was not expanded by the canonical renderer")
	}
}

func TestSemanticStyledViewerWindowRowsPreservesHexRegions(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	data := make([]byte, 320)
	for index := range data {
		data[index] = byte(index)
	}
	viewer := cachedSemanticViewer(data)
	viewer.HexMode = true
	viewer.TopOffset = 16 * 9
	width := viewer.semanticContentWidth()
	window := viewer.semanticWindow()
	expected := semanticRenderSurface(viewer.X1, viewer.Y1+1,
		viewer.X1+width-1, viewer.Y2, viewer.DisplayObject)

	viewer.TopOffset = 16 * 9
	viewer.lineOffsets = []int64{991}
	viewer.eofVisible = false
	styled := semanticStyledViewerWindowRows(viewer, window, width)
	for index, row := range styled {
		if len(row.Runs) == 0 {
			t.Fatalf("semantic hex window row %d has no styled runs", index)
		}
	}
	for index := 0; index < window.viewportRows; index++ {
		rowIndex := window.viewportRow + index
		if rowIndex >= len(styled) || index >= len(expected.Rows) {
			break
		}
		if !reflect.DeepEqual(styled[rowIndex].Runs, expected.Rows[index]) {
			t.Fatalf("hex row %d differs from DisplayObject", rowIndex)
		}
	}
	if len(styled[0].Runs) < 2 {
		t.Fatalf("hex offset/data regions lost their distinct attributes: %#v", styled[0].Runs)
	}
	if viewer.TopOffset != 16*9 || !reflect.DeepEqual(viewer.lineOffsets, []int64{991}) || viewer.eofVisible {
		t.Fatalf("hex render leaked viewer state: top=%d offsets=%v eof=%v",
			viewer.TopOffset, viewer.lineOffsets, viewer.eofVisible)
	}
}

func TestSemanticStyledEditorWindowRowsMatchesDisplayObjectAndRestoresState(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	oldCrosshair, oldCrossMode := AppConfig.EditorCrosshair, AppConfig.EditorCrossMode
	AppConfig.EditorCrosshair = true
	AppConfig.EditorCrossMode = ColorerCrossBoth
	defer func() {
		AppConfig.EditorCrosshair = oldCrosshair
		AppConfig.EditorCrossMode = oldCrossMode
	}()

	var content strings.Builder
	for index := 0; index < 64; index++ {
		fmt.Fprintf(&content, "func fn%02d() {\tprintln(\"row-%02d\") }\n", index, index)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "window.go")
	defer editor.Close()
	editor.highlighter = (&chroma.ChromaProvider{}).Create("window.go", "")
	editor.SetPosition(0, 0, 59, 8)
	editor.SetVisible(true)
	editor.SetFocus(true)
	editor.ShowWhitespaces = true
	editor.ScrollTopRow = 24
	editor.CursorLine = 27
	editor.CursorPos = 8
	editor.selActive = true
	editor.selAnchorOffset = editor.li.GetLineOffset(26) + 5
	editor.ensureEngineWidth()
	width := editor.X2 - editor.X1
	window := editor.semanticWindow()

	expected := semanticRenderSurface(editor.X1, editor.Y1+1,
		editor.X1+width-1, editor.Y2, editor.DisplayObject)
	editor.ScrollTopRow = 24
	editor.scrollBar.Value = 13
	editor.scrollBar.Min = 2
	editor.scrollBar.Max = 111
	editor.scrollBar.PgStep = 5
	editor.scrollBar.SetVisible(false)
	oldX1, oldY1, oldX2, oldY2 := editor.GetPosition()
	oldCursorLine, oldCursorPos := editor.CursorLine, editor.CursorPos
	oldSelection, oldAnchor := editor.selActive, editor.selAnchorOffset

	styled := semanticStyledEditorWindowRows(editor, window, width)
	if len(styled) != len(window.rows) {
		t.Fatalf("styled rows=%d, want %d", len(styled), len(window.rows))
	}
	for index, row := range styled {
		if len(row.Runs) == 0 {
			t.Fatalf("semantic editor window row %d has no styled runs", index)
		}
	}
	for index := 0; index < window.viewportRows; index++ {
		rowIndex := window.viewportRow + index
		if rowIndex >= len(styled) || index >= len(expected.Rows) {
			break
		}
		if !reflect.DeepEqual(styled[rowIndex].Runs, expected.Rows[index]) {
			t.Fatalf("styled editor row %d differs from DisplayObject\nwindow: %#v\nvisible: %#v",
				rowIndex, styled[rowIndex].Runs, expected.Rows[index])
		}
	}

	gotX1, gotY1, gotX2, gotY2 := editor.GetPosition()
	if gotX1 != oldX1 || gotY1 != oldY1 || gotX2 != oldX2 || gotY2 != oldY2 ||
		editor.ScrollTopRow != 24 || editor.CursorLine != oldCursorLine ||
		editor.CursorPos != oldCursorPos || editor.selActive != oldSelection ||
		editor.selAnchorOffset != oldAnchor || !editor.IsVisible() || !editor.IsFocused() {
		t.Fatalf("editor render leaked state: pos=(%d,%d,%d,%d) top=%d cursor=%d:%d selection=%v@%d",
			gotX1, gotY1, gotX2, gotY2, editor.ScrollTopRow, editor.CursorLine,
			editor.CursorPos, editor.selActive, editor.selAnchorOffset)
	}
	if editor.scrollBar.Value != 13 || editor.scrollBar.Min != 2 ||
		editor.scrollBar.Max != 111 || editor.scrollBar.PgStep != 5 || editor.scrollBar.IsVisible() {
		t.Fatalf("editor scrollbar was changed: %#v", editor.scrollBar)
	}

	foregrounds := make(map[string]bool)
	whitespaceMarkerFound := false
	for _, row := range styled {
		for _, run := range row.Runs {
			if strings.TrimSpace(run.Text) != "" {
				foregrounds[run.Foreground] = true
			}
			whitespaceMarkerFound = whitespaceMarkerFound || strings.Contains(run.Text, "→")
		}
	}
	if len(foregrounds) < 2 {
		t.Fatalf("overscan syntax styling collapsed to one foreground: %#v", foregrounds)
	}
	if !whitespaceMarkerFound {
		t.Fatal("editor tab/whitespace rendering was not preserved in the semantic window")
	}
}

func TestSemanticStyledEditorWindowRowsRepaintsOnlyChangedOverlap(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	var content strings.Builder
	for index := 0; index < 200; index++ {
		fmt.Fprintf(&content, "row-%03d alpha beta gamma\n", index)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil,
		"semantic-row-cache.txt")
	defer editor.Close()
	// This test exercises document-window reuse itself. Syntax/highlighter
	// invalidation is covered by the render-context key and the existing exact
	// renderer parity test above.
	editor.highlighter = nil
	editor.SetPosition(0, 0, 79, 8)
	editor.SetVisible(true)
	editor.ScrollTopRow = 80
	width := editor.semanticSurfaceWidth()

	window := editor.semanticWindow()
	first := semanticStyledEditorWindowRows(editor, window, width)
	if got, want := editor.semanticStyledRowsRendered, uint64(len(window.rows)); got != want {
		t.Fatalf("initial rendered rows=%d, want %d", got, want)
	}
	if len(first) == 0 || first[0].ContentKey == "" {
		t.Fatal("styled rows did not receive O(1) content identities")
	}

	editor.ScrollTopRow++
	window = editor.semanticWindow()
	before := editor.semanticStyledRowsRendered
	shifted := semanticStyledEditorWindowRows(editor, window, width)
	if got := editor.semanticStyledRowsRendered - before; got != 1 {
		t.Fatalf("one-row edge scroll repainted %d rows, want 1", got)
	}
	wantShifted := semanticRenderStyledEditorWindowRows(editor, window, width)
	if !reflect.DeepEqual(shifted, wantShifted) {
		t.Fatal("cached edge-scroll rows differ from the canonical full render")
	}

	editor.selActive = true
	editor.selAnchorOffset = editor.li.GetLineOffset(82)
	editor.CursorLine = 82
	editor.CursorPos = 3
	before = editor.semanticStyledRowsRendered
	selected := semanticStyledEditorWindowRows(editor, window, width)
	if got := editor.semanticStyledRowsRendered - before; got != 1 {
		t.Fatalf("same-row selection endpoint repainted %d rows, want 1", got)
	}
	if want := semanticRenderStyledEditorWindowRows(editor, window, width); !reflect.DeepEqual(selected, want) {
		t.Fatal("cached same-row selection differs from the canonical full render")
	}

	editor.CursorLine = 83
	editor.CursorPos = 2
	before = editor.semanticStyledRowsRendered
	selected = semanticStyledEditorWindowRows(editor, window, width)
	if got := editor.semanticStyledRowsRendered - before; got != 2 {
		t.Fatalf("selection crossing a row boundary repainted %d rows, want 2", got)
	}
	if want := semanticRenderStyledEditorWindowRows(editor, window, width); !reflect.DeepEqual(selected, want) {
		t.Fatal("cached cross-row selection differs from the canonical full render")
	}

	oldThemeColor := vtui.ThemePalette[7]
	defer func() { vtui.ThemePalette[7] = oldThemeColor }()
	vtui.ThemePalette[7] ^= 0x00010101
	before = editor.semanticStyledRowsRendered
	themed := semanticStyledEditorWindowRows(editor, window, width)
	if got, want := editor.semanticStyledRowsRendered-before,
		uint64(len(window.rows)); got != want {
		t.Fatalf("theme change repainted %d rows, want complete invalidation of %d",
			got, want)
	}
	if want := semanticRenderStyledEditorWindowRows(editor, window, width); !reflect.DeepEqual(themed, want) {
		t.Fatal("theme-invalidated cache differs from the canonical full render")
	}
}
