package viewer

import (
	"context"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestViewerANSIProjectionCarriesColorsAndSourceOffsets(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	data := []byte("\x1b[31mabcd\nef\x1b[0m\n")
	vv := cachedSemanticViewer(data)
	vv.AnsiMode = true
	var pending *viewerWindowConstruction
	window, ready, err := vv.constructWindow(2, 3, 0, &pending)
	if err != nil || !ready || len(window.rows) != 3 {
		t.Fatalf("window ready=%v err=%v rows=%d", ready, err, len(window.rows))
	}
	for index, want := range []string{"ab", "cd", "ef"} {
		row := window.rows[index]
		var text strings.Builder
		for _, cell := range row.projection.cells {
			text.WriteString(vtui.CellString(cell.Char))
			if vtui.GetIndexFore(cell.Attributes) != 1 {
				t.Fatalf("row %d lost ANSI foreground: %#x", index, cell.Attributes)
			}
		}
		if text.String() != want {
			t.Fatalf("row %d = %q, want %q", index, text.String(), want)
		}
	}
	if window.rows[1].start != 7 || window.rows[2].start != 10 {
		t.Fatalf("row offsets counted escape cells: %+v", window.rows)
	}
	if window.rows[0].projection.cellByteOffsets[0] != 5 {
		t.Fatal("first cell lost its source byte offset")
	}
}

func TestViewerANSIWrapRetainsTabOrigin(t *testing.T) {
	old := config.App.EditorTabSize
	config.App.EditorTabSize = 4
	t.Cleanup(func() { config.App.EditorTabSize = old })
	data := []byte("\x1b[32m12\tX\n")
	first := scanViewerTextMode(data, 2, true, 0, 0, false, true)
	second := scanViewerTextMode(data[first.lineLen:], 2, true, first.nextColumn, 0, true, true)
	if first.nextColumn != 2 || second.nextColumn != 4 || len(second.cells) != 2 {
		t.Fatalf("ANSI changed tab origin: first=%+v second=%+v", first, second)
	}
}

func TestViewerANSISelectionCopiesVisibleBlockColumns(t *testing.T) {
	vtui.SetDefaultPalette()
	action := map[string]any{
		"block": true,
		"rows": []map[string]any{
			{"text": "\x1b[31mabcd\x1b[0m", "width": 4, "start": 1, "end": 3},
			{"text": "\x1b[32mx", "width": 4, "start": 1, "end": 3},
		},
	}
	if got := semanticViewerSelectionText(action, true); got != "bc\n  " {
		t.Fatalf("ANSI selection copied control sequences instead of displayed cells: %q", got)
	}
}

func TestViewerANSIEndIgnoresEscapesAcrossSourceSlices(t *testing.T) {
	data := []byte("\x1b[31mabcdef\x1b[0m\nxy")
	scanner := newViewerEndRowScanner(context.Background(), int64(len(data)), 0, 2, 2, true, 8)
	scanner.ansi = true
	for index := range data {
		if err := scanner.feed(int64(index), data[index:index+1], index+1 == len(data)); err != nil {
			t.Fatal(err)
		}
	}
	result, ok := scanner.result()
	if !ok || result.top.offset != 9 || result.top.column != 4 || scanner.totalRows != 4 {
		t.Fatalf("End counted ANSI sequences as cells: result=%+v rows=%d", result, scanner.totalRows)
	}
}
