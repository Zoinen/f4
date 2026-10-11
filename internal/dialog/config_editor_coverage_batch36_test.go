package dialog

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestConfigEditorTextWidthClampsNarrowMenusCoverageBatch36(t *testing.T) {
	if got := configEditorTextWidth(0); got != 1 {
		t.Fatalf("configEditorTextWidth(0) = %d, want 1", got)
	}
	if got := configEditorTextWidth(76); got != 72 {
		t.Fatalf("configEditorTextWidth(76) = %d, want 72", got)
	}
}

func TestConfigEditorTextLinesHandlesNegativeNeedCoverageBatch36(t *testing.T) {
	if got := configEditorTextLines(-1, 0, 20); got != 0 {
		t.Fatalf("configEditorTextLines(-1, 0, 20) = %d, want 0", got)
	}
}

func TestConfigEditorTextLinesHandlesEmptyListSpaceCoverageBatch36(t *testing.T) {
	if got := configEditorTextLines(4, 0, 7); got != 4 {
		t.Fatalf("configEditorTextLines(4, 0, 7) = %d, want 4", got)
	}
}

func TestConfigEditorShownValueLeavesOtherValuesVisibleCoverageBatch36(t *testing.T) {
	for _, tc := range []struct {
		section string
		key     string
		value   string
	}{
		{"Proxy", "Host", "proxy.example"},
		{"Editor", "Password", "visible"},
		{"Proxy", "Password", ""},
	} {
		if got := configEditorShownValue(tc.section, tc.key, tc.value); got != tc.value {
			t.Errorf("configEditorShownValue(%q, %q, %q) = %q, want original value", tc.section, tc.key, tc.value, got)
		}
	}
}

func TestConfigEditorRowWithoutDefaultIsAlwaysChangedCoverageBatch36(t *testing.T) {
	row := configEditorRow{option: config.Option{Section: "Extra", Key: "Flag", Value: ""}}
	if !row.changed() {
		t.Fatal("row without a default should be changed")
	}
	if got := row.mark(); got != "?" {
		t.Fatalf("row without a default mark = %q, want ?", got)
	}
}

func TestConfigEditorRowsPreserveCurrentOrderCoverageBatch36(t *testing.T) {
	current := []config.Option{
		{Section: "B", Key: "Second", Value: "2"},
		{Section: "A", Key: "First", Value: "1"},
	}
	rows := configEditorRows(current, []config.Option{{Section: "A", Key: "First", Value: "0"}})
	if len(rows) != 2 || rows[0].name() != "B.Second" || rows[1].name() != "A.First" {
		t.Fatalf("row order = [%s, %s], want current order", rows[0].name(), rows[1].name())
	}
	if rows[0].hasDefault || rows[1].def != "0" || !rows[1].hasDefault {
		t.Fatalf("default pairing = %#v, want unknown then A.First=0", rows)
	}
}

func TestConfigEditorLinesEmptyAndAllHiddenCoverageBatch36(t *testing.T) {
	if lines, index := configEditorLines(nil, configEditorState{}); len(lines) != 0 || len(index) != 0 {
		t.Fatalf("empty rows produced lines=%q index=%v", lines, index)
	}
	rows := configEditorRows([]config.Option{{Section: "A", Key: "One", Value: "1"}}, []config.Option{{Section: "A", Key: "One", Value: "1"}})
	if lines, index := configEditorLines(rows, configEditorState{hideUnchanged: true}); len(lines) != 0 || len(index) != 0 {
		t.Fatalf("all unchanged rows were not hidden: lines=%q index=%v", lines, index)
	}
}

func TestConfigEditorDescriptionIncludesUnlabelledExplanationCoverageBatch36(t *testing.T) {
	row := configEditorRows([]config.Option{{Section: "Editor", Key: "TabSize", Value: "8"}}, []config.Option{{Section: "Editor", Key: "TabSize", Value: "4"}})[0]
	row.doc = ConfigOptionDoc{Description: "Choose the tab width."}
	got := configEditorDescription(row)
	if !strings.Contains(got, "Choose the tab width.") || !strings.Contains(got, "4") || strings.Contains(got, ". Choose") {
		t.Fatalf("unlabelled description = %q", got)
	}
}

func TestConfigEditorDescriptionOmitsEmptyExplanationWithLabelCoverageBatch36(t *testing.T) {
	row := configEditorRows([]config.Option{{Section: "Editor", Key: "TabSize", Value: "8"}}, []config.Option{{Section: "Editor", Key: "TabSize", Value: "4"}})[0]
	row.doc = ConfigOptionDoc{Label: "Tab width"}
	got := configEditorDescription(row)
	if !strings.HasPrefix(got, "Tab width. ") || strings.Contains(got, "\n") {
		t.Fatalf("label-only description = %q", got)
	}
}

func TestConfigEditorLinesAlignSectionAndKeyColumnsCoverageBatch36(t *testing.T) {
	rows := configEditorRows([]config.Option{
		{Section: "Long", Key: "X", Value: "1"},
		{Section: "S", Key: "LongKey", Value: "2"},
	}, nil)
	lines, index := configEditorLines(rows, configEditorState{alignDot: true})
	if len(lines) != 2 || len(index) != 2 || index[0] != 0 || index[1] != 1 {
		t.Fatalf("aligned lines/index = %q/%v", lines, index)
	}
	if !strings.Contains(lines[0], "Long.X      ") || !strings.Contains(lines[1], "   S.LongKey") {
		t.Fatalf("aligned columns = %q", lines)
	}
}
