package editor

import (
	config "github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	reflect "reflect"
	testing "testing"
)

func TestNativeSecondaryCaretsKeepBaseRowsAndSurviveModelConversion(t *testing.T) {
	previous := config.App.EditorMarkOccurrences
	config.App.EditorMarkOccurrences = false
	t.Cleanup(func() { config.App.EditorMarkOccurrences = previous })
	editor := projectionTestEditor(t, "abcdefghij\nsecond row\n")
	editor.Highlighter = nil
	first := editor.SemanticNode(nil)
	beforeRows := editor.semanticStyledRowsRendered
	editor.extraCursors = []extraCaret{{off: 7, anchor: 3, hasSel: true}}
	second := editor.SemanticNode(nil)
	if !reflect.DeepEqual(first["windowRows"], second["windowRows"]) || editor.semanticStyledRowsRendered != beforeRows {
		t.Fatal("secondary selection repainted native base rows")
	}
	carets := semantic.AppMapSlice(second["secondaryCarets"])
	if len(carets) != 1 || semantic.Int(carets[0]["cursorAbsoluteColumn"]) != 7 || !semantic.Bool(carets[0]["selection"]) {
		t.Fatalf("secondary caret lost in typed model: %+v", carets)
	}
	guard := editor.editorCursorStateGuard()
	editor.extraCursors[0].off++
	if !guard.canPublish(editor, true) {
		t.Fatal("secondary movement cannot publish compact cursor state")
	}
	third := editor.SemanticNode(nil)
	if first["windowContentKey"] != third["windowContentKey"] {
		t.Fatal("caret movement changed row content key")
	}
	editor.extraCursors = nil
	if len(semantic.AppMapSlice(editor.SemanticNode(nil)["secondaryCarets"])) != 0 {
		t.Fatal("removed caret retained")
	}
}
