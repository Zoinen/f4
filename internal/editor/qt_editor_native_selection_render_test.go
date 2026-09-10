package editor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNativeEditorSelectionUsesStableRowsAndScalarOverlayState(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	oldSelected := vtui.Palette[vtui.ColDialogEditSelected]
	vtui.Palette[vtui.ColDialogEditSelected] = vtui.SetRGBBack(vtui.SetRGBFore(0, 0xffffff), 0x3b6290)
	defer func() { vtui.Palette[vtui.ColDialogEditSelected] = oldSelected }()
	check := func(t *testing.T, data []byte) {
		editor := NewEditorView(piecetable.New(data), nil, "selection.bin")
		defer editor.Close()
		editor.Highlighter = nil
		editor.SetPosition(0, 0, 79, 24)
		editor.NativeViewportColumns = 302
		editor.NativeViewportRows = 46
		editor.NativeViewportRevision = 1
		editor.SetVisible(true)
		editor.SetFocus(true)
		editor.EnsureEngineWidth()
		window := editor.semanticWindowMetadata()
		if !window.Ready {
			t.Fatal("initial window is not ready")
		}
		plain := semanticStyledEditorWindowRows(editor, window, editor.viewportWidth())
		var plainNode map[string]any
		if os.Getenv("F4_TEST_NATIVE_SURFACE_DUMP") != "" {
			plainNode = editor.SemanticNode(nil)
		}
		editor.SelActive = true
		editor.SelAnchorOffset = editor.Li.GetLineOffset(5) + 8
		editor.CursorLine, editor.CursorPos = 2, 2
		if len(data) > 3054 && string(data[:2]) == "MZ" {
			editor.SelAnchorOffset = 3054
			editor.CursorPos = 15
		}
		window = editor.semanticWindowMetadata()
		selected := semanticStyledEditorWindowRows(editor, window, editor.viewportWidth())
		if !reflect.DeepEqual(selected, plain) {
			t.Fatal("stream selection mutated immutable native base rows")
		}
		for _, row := range selected {
			for _, run := range row.Runs {
				if run.Background == "#3b6290" {
					t.Fatalf("stream selection leaked into base row %d: %+v", row.VisualRow, row.Runs)
				}
			}
		}
		selectedNode := editor.SemanticNode(nil)
		anchorRow, anchorColumn := editor.Engine.LogicalToVisual(editor.SelAnchorOffset)
		focusOffset := editor.Li.GetLineOffset(editor.CursorLine) + editor.CursorPos
		focusRow, focusColumn := editor.Engine.LogicalToVisual(focusOffset)
		if !semantic.Bool(selectedNode["selection"]) ||
			semantic.Int64(selectedNode["selectionAnchorRow"]) != int64(anchorRow) ||
			semantic.Int(selectedNode["selectionAnchorColumn"]) != anchorColumn ||
			semantic.Int64(selectedNode["cursorAbsoluteRow"]) != int64(focusRow) ||
			semantic.Int(selectedNode["cursorAbsoluteColumn"]) != focusColumn ||
			semantic.String(selectedNode["selectionForeground"]) != "#ffffff" ||
			semantic.String(selectedNode["selectionBackground"]) != "#3b6290" {
			t.Fatalf("selection overlay state lost projection geometry/style: %#v", selectedNode)
		}
		beforeKey := semantic.String(selectedNode["windowContentKey"])
		lineLength := editor.GetLineLength(editor.CursorLine)
		if editor.CursorPos < lineLength {
			editor.CursorPos++
		} else if editor.CursorPos > 0 {
			editor.CursorPos--
		}
		shiftedNode := editor.SemanticNode(nil)
		if semantic.String(shiftedNode["windowContentKey"]) != beforeKey ||
			!reflect.DeepEqual(semantic.AppMapSlice(shiftedNode["windowRows"]),
				semantic.AppMapSlice(selectedNode["windowRows"])) {
			t.Fatal("moving a stream-selection endpoint rebuilt base rows")
		}
		if directory := os.Getenv("F4_TEST_NATIVE_SURFACE_DUMP"); directory != "" {
			encoded, err := json.Marshal(map[string]any{"plain": plainNode, "selected": selectedNode})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, strings.ReplaceAll(t.Name(), "/", "-")+".json")
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Run("ordinary", func(t *testing.T) {
		check(t, []byte(strings.Repeat("alpha beta gamma delta\n", 120)))
	})
	t.Run("binary", func(t *testing.T) {
		data, err := os.ReadFile(`C:\Windows\pyw.exe`)
		if err != nil {
			t.Skip("Windows pyw.exe fixture unavailable")
		}
		check(t, data)
	})
}

func TestNativeEditorRectangularSelectionStaysInBaseRows(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	editor := NewEditorView(piecetable.New([]byte("alpha\nbeta\n")), nil, "rect.txt")
	defer editor.Close()
	editor.Highlighter = nil
	editor.SetPosition(0, 0, 39, 8)
	editor.NativeViewportColumns, editor.NativeViewportRows = 40, 8
	editor.SetVisible(true)
	editor.RectSelActive = true
	editor.rectSelStartLine, editor.rectSelStartCol = 0, 1
	editor.CursorLine, editor.CursorPos = 1, 3
	editor.EnsureEngineWidth()
	rows := semanticStyledEditorWindowRows(editor, editor.semanticWindowMetadata(), 40)
	selectedBackground := semantic.SemanticAttrColor(vtui.Palette[vtui.ColDialogEditSelected], false)
	found := false
	for _, row := range rows {
		for _, run := range row.Runs {
			found = found || run.Background == selectedBackground
		}
	}
	if !found {
		t.Fatal("rectangular selection lost its row-painted styling")
	}
	if semantic.Bool(editor.SemanticNode(nil)["selection"]) {
		t.Fatal("rectangular selection also enabled the stream overlay")
	}
}

func TestNativeEditorProjectionPreservesStyledRows(t *testing.T) {
	// Native base rows keep all source identities, projected cells and
	// non-stream-selection attributes deterministic across projection modes.
	snapshots := map[string]string{
		"text":         "346d667e49fe3d40a2506829c3beee8be5a384c1bd7b0cb31996a4db2ff49900",
		"wrap":         "6c1a02f890302345553b150f4b282fc15c382145ff7190db7dbb5e7372b3986e",
		"syntax":       "78f87c99c5c468d499436ae071744bbdbe014d38d170e53db2f24198087f9ed7",
		"crosshair":    "e19293f382d822d99b0f66b6f8dfcc2e8e97747060602fe2e41b1aa017bda523",
		"whitespace":   "f07433456294c2b1e43fae14b8c99a32a2da87b3f170440a40f02c6259ca9f55",
		"autocomplete": "14e53ca7dc1d35c6a2d1a5aa2558fa5bc0f9193c84a5490c00f6e502126f3450",
		"hex":          "ad706df8432c3b2e9bccd2656b49c993771d3f2ef3dd0fcc890521f14ff5da21",
		"decode":       "f9d64db2db82beca81050fca870b1a8569dd72e213299e9543d20dd67edd3e77",
	}
	oldConfig := config.App
	defer func() { config.App = oldConfig }()
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	for _, mode := range []string{"text", "wrap", "syntax", "crosshair", "whitespace", "autocomplete", "hex", "decode"} {
		t.Run(mode, func(t *testing.T) {
			config.App.EditorCrosshair = mode == "crosshair"
			config.App.EditorCrossMode = config.ColorerCrossBoth
			data := []byte(strings.Repeat("func main() {\tprintln(\"wide界 é\") }\n\xff\xfe middle\tend\n\n", 12))
			editor := NewEditorView(piecetable.New(data), nil, "projection.go")
			defer editor.Close()
			editor.Highlighter = nil
			if mode == "syntax" || mode == "crosshair" {
				editor.Highlighter = (&chroma.ChromaProvider{}).Create("projection.go", "")
			}
			editor.SetPosition(0, 0, 79, 12)
			editor.NativeViewportColumns = 34
			editor.NativeViewportRows = 10
			editor.NativeViewportRevision = 1
			editor.SetVisible(true)
			editor.SetFocus(true)
			editor.WordWrap = mode == "wrap"
			editor.ShowWhitespaces = mode == "whitespace"
			editor.HexMode = mode == "hex"
			editor.DecodeMode = mode == "decode"
			editor.CursorLine, editor.CursorPos = 3, 5
			editor.SelActive, editor.SelAnchorOffset = true, 8
			if mode == "autocomplete" {
				editor.acEnabled = true
				editor.acMatches = []string{"println"}
				editor.acPrefix = "pr"
			}
			editor.EnsureEngineWidth()
			window := editor.semanticWindowMetadata()
			rows := semanticRenderStyledEditorWindowRows(editor, window, editor.viewportWidth())
			encoded, err := json.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(encoded))
			if got != snapshots[mode] {
				t.Fatalf("styled row snapshot changed: got %s, want %s", got, snapshots[mode])
			}
			if len(rows) == 0 {
				t.Fatal("no styled rows")
			}
		})
	}
}

func BenchmarkNativeEditorRowProjection(b *testing.B) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	editor := NewEditorView(piecetable.New([]byte(strings.Repeat(
		"alpha beta gamma delta epsilon\twide界\n", 1000))), nil, "projection.txt")
	defer editor.Close()
	editor.Highlighter = nil
	editor.SetPosition(0, 0, 79, 24)
	editor.NativeViewportColumns = 302
	editor.NativeViewportRows = 46
	editor.NativeViewportRevision = 1
	editor.ScrollTopRow = 500
	editor.SetVisible(true)
	editor.EnsureEngineWidth()
	window := editor.semanticWindowMetadata()
	width := editor.viewportWidth()
	b.Run("direct", func(b *testing.B) {
		for index := 0; index < b.N; index++ {
			_ = semanticRenderStyledEditorWindowRows(editor, window, width)
		}
	})
	b.Run("temporary-screen", func(b *testing.B) {
		for index := 0; index < b.N; index++ {
			rendered := semantic.SemanticRenderSurface(editor.X1, editor.Y1+1,
				editor.X1+width-1, editor.Y1+len(window.Rows), func(scr *vtui.ScreenBuf) {
					editor.projectTextRows(int(window.Start), width, len(window.Rows),
						editor.textProjectionStyle(), func(row editorProjectedTextRow) {
							scr.Write(editor.X1, editor.Y1+1+row.visualRow-int(window.Start), row.cells)
						})
				})
			if len(rendered.Rows) != len(window.Rows) {
				b.Fatal("temporary screen lost rows")
			}
		}
	})
}
