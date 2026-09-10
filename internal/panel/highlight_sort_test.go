package panel

import (
	ini "github.com/unxed/f4/internal/ini"
	theme "github.com/unxed/f4/internal/theme"
	vfs "github.com/unxed/f4/vfs"
	strings "strings"
	testing "testing"
)

func TestSortGroupsReuseHighlightRules(t *testing.T) {
	ini := ini.Parse(strings.NewReader(`[Highlight_4]
Name = Archives
Group = 1
Mask = *.zip, *.7z

[Highlight_5]
Name = Images
Group = 2
Mask = *.png, *.jpg

[Highlight_6]
Name = Unsorted highlight rule
Mask = *.txt
`))
	groups := sortGroupsFromHighlightRules(theme.ParseHighlightRules(ini))
	if len(groups) != 2 {
		t.Fatalf("shared highlight rules produced %d sort groups, want 2", len(groups))
	}
	if groups[0].Name != "Archives" || groups[0].Order != 1 {
		t.Fatalf("first shared group = %#v, want Archives at 1", groups[0])
	}
	if groups[1].Name != "Images" || groups[1].Order != 2 {
		t.Fatalf("second shared group = %#v, want Images at 2", groups[1])
	}

	archive := vfs.VFSItem{Name: "backup.ZIP"}
	if got := groups[0].Filter.Match(&archive, true); !got {
		t.Fatal("shared highlight matcher did not match archive")
	}
}
