package dialog

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestConfigEditorRowsMarkChangedAndUnknownDefaults(t *testing.T) {
	current := []config.Option{
		{Section: "Editor", Key: "TabSize", Value: "8"},
		{Section: "Editor", Key: "AutoIndent", Value: "1"},
		{Section: "Appearance", Key: "GuiPosX", Value: "10"},
	}
	defaults := []config.Option{
		{Section: "Editor", Key: "TabSize", Value: "4"},
		{Section: "Editor", Key: "AutoIndent", Value: "1"},
	}
	rows := configEditorRows(current, defaults)
	for i, want := range []string{"*", " ", "?"} {
		if got := rows[i].mark(); got != want {
			t.Errorf("row %d (%s) mark = %q, want %q", i, rows[i].name(), got, want)
		}
	}

	lines, _ := configEditorLines(rows, configEditorState{})
	if len(lines) != 3 || lines[0] != "* Editor.TabSize     │ 8" {
		t.Fatalf("lines = %q", lines)
	}

	lines, index := configEditorLines(rows, configEditorState{hideUnchanged: true})
	if len(lines) != 2 || index[0] != 0 || index[1] != 2 {
		t.Fatalf("with unchanged rows hidden: lines = %q, index = %v", lines, index)
	}

	lines, _ = configEditorLines(rows, configEditorState{alignDot: true})
	if !strings.HasPrefix(lines[0], "*     Editor.TabSize    │ 8") {
		t.Fatalf("aligned line = %q", lines[0])
	}
}

func TestConfigEditorMasksTheProxyPassword(t *testing.T) {
	rows := configEditorRows([]config.Option{{Section: "Proxy", Key: "Password", Value: "c2VjcmV0"}}, nil)
	lines, _ := configEditorLines(rows, configEditorState{})
	if len(lines) != 1 || strings.Contains(lines[0], "c2VjcmV0") || !strings.Contains(lines[0], "********") {
		t.Fatalf("password line = %q", lines)
	}
}

func TestConfigEditorTitleMarksHiddenRows(t *testing.T) {
	if plain, marked := configEditorTitle(false), configEditorTitle(true); marked != plain+" *" {
		t.Fatalf("configEditorTitle(true) = %q, want %q", marked, plain+" *")
	}
}
