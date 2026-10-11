package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
)

// TestIssue413GroupKeyInHighlightSectionsSortsFiles is the regression test for
// issue #413: highlight.ini says a coloured [Highlight_N] rule joins a sort
// group when it carries a Group key, but the start-up code gave the sort
// groups no highlight rules, so such a Group was ignored and nothing clustered.
func TestIssue413GroupKeyInHighlightSectionsSortsFiles(t *testing.T) {
	prevSort, prevHighlighter := panel.GlobalSortGroups, theme.GlobalFileHighlighter
	panel.GlobalSortGroups, theme.GlobalFileHighlighter = &panel.SortGroupSet{}, &theme.FileHighlighter{}
	t.Cleanup(func() { panel.GlobalSortGroups, theme.GlobalFileHighlighter = prevSort, prevHighlighter })

	loadHighlightIni(ini.Parse(strings.NewReader(`
[Highlight_1]
Name = Executables
Mask = *.exe,*.bat,*.cmd,*.sh,*.bash
Group = 0

[Highlight_2]
Name = Executables Linux
IncludeAttributes = Executable
ExcludeAttributes = directory
Group = 0

[Highlight_3]
Name = Pictures
Mask = *.png,*.jpg
Group = 2

[Highlight_4]
Name = Text, no group
Mask = *.txt
`)))

	if !panel.GlobalSortGroups.Configured() {
		t.Fatal("no sort groups were built from the Group keys of the Highlight sections")
	}
	cases := []struct {
		item vfs.VFSItem
		want int
	}{
		{vfs.VFSItem{Name: "setup.exe"}, 0},
		{vfs.VFSItem{Name: "tool", IsExecutable: true}, 0},
		{vfs.VFSItem{Name: "bin", IsDir: true, IsExecutable: true}, panel.DefaultSortGroupOrder},
		{vfs.VFSItem{Name: "photo.png"}, 2},
		{vfs.VFSItem{Name: "notes.txt"}, panel.DefaultSortGroupOrder},
	}
	for _, tc := range cases {
		item := tc.item
		if got := panel.GlobalSortGroups.GroupOf(&item); got != tc.want {
			t.Errorf("GroupOf(%q) = %d, want %d", item.Name, got, tc.want)
		}
	}
}
