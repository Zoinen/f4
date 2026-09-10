package editor

import (
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/vtui"
	"reflect"
	"strings"
	"testing"
)

func TestSemanticStyledEditorWindowRowsMatchesDisplayObjectAndRestoresState(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	oldCrosshair, oldCrossMode := config.App.EditorCrosshair, config.App.EditorCrossMode
	config.App.EditorCrosshair = true
	config.App.EditorCrossMode = config.ColorerCrossBoth
	defer func() {
		config.App.EditorCrosshair = oldCrosshair
		config.App.EditorCrossMode = oldCrossMode
	}()

	var content strings.Builder
	for index := 0; index < 64; index++ {
		fmt.Fprintf(&content, "func fn%02d() {\tprintln(\"row-%02d\") }\n", index, index)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "window.go")
	defer editor.Close()
	editor.Highlighter = (&chroma.ChromaProvider{}).Create("window.go", "")
	editor.SetPosition(0, 0, 59, 8)
	editor.SetVisible(true)
	editor.SetFocus(true)
	editor.ShowWhitespaces = true
	editor.ScrollTopRow = 24
	editor.CursorLine = 27
	editor.CursorPos = 8
	editor.SelActive = true
	editor.SelAnchorOffset = editor.Li.GetLineOffset(26) + 5
	editor.EnsureEngineWidth()
	width := editor.X2 - editor.X1
	window := editor.semanticWindow()

	// Native rows are the immutable base projection. Compare them with the
	// console renderer with regular stream selection disabled; Qt paints that
	// selection from the separately fenced scalar overlay state.
	editor.SelActive = false
	expected := semantic.SemanticRenderSurface(editor.X1, editor.Y1+1,
		editor.X1+width-1, editor.Y2, editor.DisplayObject)
	editor.SelActive = true
	editor.ScrollTopRow = 24
	editor.ScrollBar.Value = 13
	editor.ScrollBar.Min = 2
	editor.ScrollBar.Max = 111
	editor.ScrollBar.PgStep = 5
	editor.ScrollBar.SetVisible(false)
	oldX1, oldY1, oldX2, oldY2 := editor.GetPosition()
	oldCursorLine, oldCursorPos := editor.CursorLine, editor.CursorPos
	oldSelection, oldAnchor := editor.SelActive, editor.SelAnchorOffset

	styled := semanticStyledEditorWindowRows(editor, window, width)
	if len(styled) != len(window.Rows) {
		t.Fatalf("styled rows=%d, want %d", len(styled), len(window.Rows))
	}
	for index, row := range styled {
		if len(row.Runs) == 0 {
			t.Fatalf("semantic editor window row %d has no styled runs", index)
		}
	}
	for index := 0; index < window.ViewportRows; index++ {
		rowIndex := window.ViewportRow + index
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
		editor.CursorPos != oldCursorPos || editor.SelActive != oldSelection ||
		editor.SelAnchorOffset != oldAnchor || !editor.IsVisible() || !editor.IsFocused() {
		t.Fatalf("editor render leaked state: pos=(%d,%d,%d,%d) top=%d cursor=%d:%d selection=%v@%d",
			gotX1, gotY1, gotX2, gotY2, editor.ScrollTopRow, editor.CursorLine,
			editor.CursorPos, editor.SelActive, editor.SelAnchorOffset)
	}
	if editor.ScrollBar.Value != 13 || editor.ScrollBar.Min != 2 ||
		editor.ScrollBar.Max != 111 || editor.ScrollBar.PgStep != 5 || editor.ScrollBar.IsVisible() {
		t.Fatalf("editor scrollbar was changed: %#v", editor.ScrollBar)
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
	oldMarkOccurrences := config.App.EditorMarkOccurrences
	config.App.EditorMarkOccurrences = false
	t.Cleanup(func() { config.App.EditorMarkOccurrences = oldMarkOccurrences })
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

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
	editor.Highlighter = nil
	editor.SetPosition(0, 0, 79, 8)
	editor.SetVisible(true)
	editor.ScrollTopRow = 80
	width := editor.semanticSurfaceWidth()

	window := editor.semanticWindow()
	first := semanticStyledEditorWindowRows(editor, window, width)
	if got, want := editor.semanticStyledRowsRendered, uint64(len(window.Rows)); got != want {
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

	editor.SelActive = true
	editor.SelAnchorOffset = editor.Li.GetLineOffset(82)
	editor.CursorLine = 82
	editor.CursorPos = 3
	before = editor.semanticStyledRowsRendered
	selected := semanticStyledEditorWindowRows(editor, window, width)
	if got := editor.semanticStyledRowsRendered - before; got != 0 {
		t.Fatalf("same-row selection endpoint repainted %d base rows, want 0", got)
	}
	if want := semanticRenderStyledEditorWindowRows(editor, window, width); !reflect.DeepEqual(selected, want) {
		t.Fatal("cached same-row selection differs from the canonical full render")
	}

	editor.CursorLine = 83
	editor.CursorPos = 2
	before = editor.semanticStyledRowsRendered
	selected = semanticStyledEditorWindowRows(editor, window, width)
	if got := editor.semanticStyledRowsRendered - before; got != 0 {
		t.Fatalf("selection crossing a row boundary repainted %d base rows, want 0", got)
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
		uint64(len(window.Rows)); got != want {
		t.Fatalf("theme change repainted %d rows, want complete invalidation of %d",
			got, want)
	}
	if want := semanticRenderStyledEditorWindowRows(editor, window, width); !reflect.DeepEqual(themed, want) {
		t.Fatal("theme-invalidated cache differs from the canonical full render")
	}
}

func TestSemanticEditorOccurrenceSelectionInvalidatesCachedRows(t *testing.T) {
	old := config.App.EditorMarkOccurrences
	config.App.EditorMarkOccurrences = true
	t.Cleanup(func() { config.App.EditorMarkOccurrences = old })
	ev := projectionTestEditor(t, "word word\nword word")
	ev.Highlighter = nil
	window := ev.semanticWindow()
	width := ev.semanticSurfaceWidth()
	semanticStyledEditorWindowRows(ev, window, width)
	ev.SelActive, ev.SelAnchorOffset, ev.CursorPos = true, 0, 4
	got := semanticStyledEditorWindowRows(ev, window, width)
	want := semanticRenderStyledEditorWindowRows(ev, window, width)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("occurrence selection reused stale base rows")
	}
	if ev.editorCursorStateGuard().eligible {
		t.Fatal("occurrence selection allowed scalar-only cursor update")
	}
}
