package editor

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestEditorSharedProjectionDrawsUpstreamControlsAndWrapMarks(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	ev := projectionTestEditor(t, "ab\x01cdefgh")
	ev.Highlighter = nil
	ev.ShowControlChars = true
	ev.WordWrap = true
	ev.NativeViewportColumns, ev.NativeViewportRows = 4, 3
	ev.EnsureEngineWidth()
	var rows [][]vtui.CharInfo
	ev.projectTextRows(0, 4, 3, ev.textProjectionStyle(), func(row editorProjectedTextRow) {
		rows = append(rows, append([]vtui.CharInfo(nil), row.cells...))
	})
	if len(rows) < 2 || rows[0][2].Char != 0x2401 {
		t.Fatalf("shared projection lost control pictures: %+v", rows)
	}
	if rows[0][3].Char != 'c' || rows[0][3].Attributes != ev.wrapMarkAttr() {
		t.Fatalf("full-width wrap must tint its last glyph: %+v", rows[0][3])
	}
}
