package viewer

import (
	context "context"
	fmt "fmt"
	semantic "github.com/unxed/f4/internal/semantic"
	theme "github.com/unxed/f4/internal/theme"
	vfs "github.com/unxed/f4/vfs"
	vtui "github.com/unxed/vtui"
	reflect "reflect"
	strings "strings"
	testing "testing"
)

func cachedSemanticViewer(data []byte) *ViewerView {
	file := &vfs.MemoryReadAtCloser{Data: data}
	backend := &ViewerBackend{
		File:         file,
		size:         int64(len(data)),
		totalLines:   -1,
		totalForSize: -1,
		cacheData:    data,
		ctx:          context.Background(),
	}
	viewer := &ViewerView{
		Backend:  backend,
		WrapMode: true,
	}
	viewer.ScrollBar = vtui.NewScrollBar(0, 0, 0)
	viewer.SetPosition(0, 0, 39, 8)
	viewer.SetVisible(true)
	return viewer
}

func TestSemanticStyledViewerWindowRowsMatchesTextRendererAndRestoresState(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

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

	expected := semantic.SemanticRenderSurface(viewer.X1, viewer.Y1+1,
		viewer.X1+width-1, viewer.Y2, viewer.DisplayObject)
	viewer.TopOffset = offsets[20]
	viewer.LineOffsets = []int64{701, 709, 719}
	viewer.EofVisible = true
	viewer.lastKnownSize = int64(content.Len())
	viewer.ScrollBar.Value = 17
	viewer.ScrollBar.Min = 3
	viewer.ScrollBar.Max = 91
	viewer.ScrollBar.PgStep = 7
	viewer.ScrollBar.SetVisible(false)

	styled := semanticStyledViewerWindowRows(viewer, window, width)
	if len(styled) != len(window.Rows) {
		t.Fatalf("styled rows=%d, want %d", len(styled), len(window.Rows))
	}
	for index, row := range styled {
		if len(row.Runs) == 0 {
			t.Fatalf("semantic text window row %d has no styled runs", index)
		}
	}
	for index := 0; index < window.ViewportRows; index++ {
		rowIndex := window.ViewportRow + index
		if rowIndex >= len(styled) || index >= len(expected.Rows) {
			break
		}
		if !reflect.DeepEqual(styled[rowIndex].Runs, expected.Rows[index]) {
			t.Fatalf("styled viewer row %d differs from DisplayObject\nwindow: %#v\nvisible: %#v",
				rowIndex, styled[rowIndex].Runs, expected.Rows[index])
		}
	}

	if viewer.TopOffset != offsets[20] ||
		!reflect.DeepEqual(viewer.LineOffsets, []int64{701, 709, 719}) ||
		!viewer.EofVisible || viewer.lastKnownSize != int64(content.Len()) {
		t.Fatalf("viewer render leaked state: top=%d offsets=%v eof=%v size=%d",
			viewer.TopOffset, viewer.LineOffsets, viewer.EofVisible, viewer.lastKnownSize)
	}
	if viewer.ScrollBar.Value != 17 || viewer.ScrollBar.Min != 3 ||
		viewer.ScrollBar.Max != 91 || viewer.ScrollBar.PgStep != 7 || viewer.ScrollBar.IsVisible() {
		t.Fatalf("viewer scrollbar was changed: %#v", viewer.ScrollBar)
	}
	if strings.Contains(styled[window.ViewportRow].Runs[0].Text, "\t") {
		t.Fatal("viewer tab was not expanded by the canonical renderer")
	}
}

func TestSemanticStyledViewerWindowRowsPreservesHexRegions(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

	data := make([]byte, 320)
	for index := range data {
		data[index] = byte(index)
	}
	viewer := cachedSemanticViewer(data)
	viewer.HexMode = true
	viewer.TopOffset = 16 * 9
	width := viewer.semanticContentWidth()
	window := viewer.semanticWindow()
	expected := semantic.SemanticRenderSurface(viewer.X1, viewer.Y1+1,
		viewer.X1+width-1, viewer.Y2, viewer.DisplayObject)

	viewer.TopOffset = 16 * 9
	viewer.LineOffsets = []int64{991}
	viewer.EofVisible = false
	styled := semanticStyledViewerWindowRows(viewer, window, width)
	for index, row := range styled {
		if len(row.Runs) == 0 {
			t.Fatalf("semantic hex window row %d has no styled runs", index)
		}
	}
	for index := 0; index < window.ViewportRows; index++ {
		rowIndex := window.ViewportRow + index
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
	if viewer.TopOffset != 16*9 || !reflect.DeepEqual(viewer.LineOffsets, []int64{991}) || viewer.EofVisible {
		t.Fatalf("hex render leaked viewer state: top=%d offsets=%v eof=%v",
			viewer.TopOffset, viewer.LineOffsets, viewer.EofVisible)
	}
}
