package panel

import (
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"strings"
	"testing"
)

func TestSortGroupSetLoadsSharedAndLegacyRules(t *testing.T) {
	ini := ini.Parse(strings.NewReader(`[Highlight_1]
Name = Images
Group = 0
Mask = *.png

[SortGroup_1]
Name = Legacy
Group = 1
Mask = *.legacy
`))
	rules := theme.ParseHighlightRules(ini)
	set := &SortGroupSet{}
	set.LoadFromIni(ini, rules)
	if len(set.Groups) != 2 {
		t.Fatalf("loaded %d sort groups, want shared and legacy rules", len(set.Groups))
	}

	image := vfs.VFSItem{Name: "photo.png"}
	legacy := vfs.VFSItem{Name: "old.legacy"}
	if got := set.GroupOf(&image); got != 0 {
		t.Errorf("shared rule group = %d, want 0", got)
	}
	if got := set.GroupOf(&legacy); got != 1 {
		t.Errorf("legacy rule group = %d, want 1", got)
	}
}
