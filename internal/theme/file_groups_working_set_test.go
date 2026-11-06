package theme

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/ini"
)

func TestDeletedWorkingSetDoesNotResurrectColourStyleRules(t *testing.T) {
	highlighter := &FileHighlighter{activeColorStyle: "Modern"}
	highlighter.LoadThemeRules(ini.Parse(strings.NewReader(
		"[Highlight_0]\nMask=*.txt\nNormalColor=foreground:#abcdef\n")))
	if len(highlighter.ThemeRules) == 0 {
		t.Fatal("fixture did not load original palette")
	}
	empty, err := filegroups.Compile([]filegroups.Group{})
	if err != nil {
		t.Fatal(err)
	}
	if err := highlighter.LoadFileGroups(empty); err != nil {
		t.Fatal(err)
	}
	if len(highlighter.ThemeRules) != 0 || len(highlighter.UserRules) != 0 {
		t.Fatal("deleted groups were resurrected by the old palette loader")
	}
}
