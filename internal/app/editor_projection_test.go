package app

import (
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func projectionTestEditor(t *testing.T, text string) *editor.EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := editor.NewEditorView(piecetable.New([]byte(text)), nil, "")
	t.Cleanup(ev.Close)
	ev.SetPosition(0, 0, 20, 6)
	ev.SetVisible(true)
	return ev
}
func TestEditorWordWrapActionKeepsCursorVisibleAfterLongLines(t *testing.T) {
	longLine := strings.Repeat("1234567890\t界xy ", 800)
	ev := projectionTestEditor(t, longLine+"\nshort\n\n"+longLine+"\nshort\n")
	ev.Highlighter = nil
	ev.NativeViewportColumns, ev.NativeViewportRows = 302, 46
	vtui.FrameManager.AddScreen(ev)
	ev.CursorLine, ev.CursorPos, ev.ScrollLeft = 4, 5, 500
	ev.EnsureEngineWidth()
	anchor := ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	for _, wrapped := range []bool{true, false} {
		if !RunAction("Editor.WordWrap") || ev.WordWrap != wrapped {
			t.Fatalf("wrap action did not set wrap=%v", wrapped)
		}
		node := ev.SemanticNode(&vtui.SemanticContext{})
		if got := ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos; got != anchor {
			t.Fatalf("wrap=%v changed cursor source anchor: got %d want %d", wrapped, got, anchor)
		}
		row := semantic.Int(node["cursorVisualRow"])
		if row < 0 || row >= ev.ViewportHeight() || !semantic.Bool(node["cursorVisible"]) {
			t.Fatalf("wrap=%v left cursor outside native viewport: row=%d top=%d", wrapped, row, ev.ScrollTopRow)
		}
	}
}
