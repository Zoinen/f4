package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFileGroupAttributeHighlightMigration(t *testing.T) {
	previous := theme.GlobalFileHighlighter
	priority, correction := config.App.HighlightPriority, config.App.EnforceColorCorrection
	t.Cleanup(func() {
		theme.GlobalFileHighlighter = previous
		config.App.HighlightPriority, config.App.EnforceColorCorrection = priority, correction
	})
	config.App.HighlightPriority, config.App.EnforceColorCorrection = 0, false
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	const source = `[Highlight_101]
UseDefaults=1
IncludeAttributes=Directory
NormalFileName=foreground:#111111
SelectedFileName=foreground:#121212
FileNameUnderCursor=foreground:#131313
FileNameSelectedUnderCursor=foreground:#141414
Mark=D
[Highlight_102]
UseDefaults=1
IncludeAttributes=Hidden
NormalFileName=foreground:#222222
SelectedFileName=foreground:#232323
FileNameUnderCursor=foreground:#242424
FileNameSelectedUnderCursor=foreground:#252525
Mark=H
[Highlight_103]
UseDefaults=1
IncludeAttributes=Symlink
NormalFileName=foreground:#333333
SelectedFileName=foreground:#343434
FileNameUnderCursor=foreground:#353535
FileNameSelectedUnderCursor=foreground:#363636
Mark=L
[Highlight_104]
UseDefaults=1
IncludeAttributes=Junction
NormalFileName=foreground:#444444
SelectedFileName=foreground:#454545
FileNameUnderCursor=foreground:#464646
FileNameSelectedUnderCursor=foreground:#474747
Mark=J
`
	old := &theme.FileHighlighter{}
	old.LoadFromIni(ini.Parse(strings.NewReader(source)))
	doc, err := filegroups.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := filegroups.Compile(filegroups.ImportLegacy(doc, nil))
	if err != nil {
		t.Fatal(err)
	}
	current := &theme.FileHighlighter{}
	if err := current.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"generic", "lucide", "zoin", "system", "lucide"} {
		if err := current.LoadFileGroups(snapshot.ForIconTheme(scope)); err != nil {
			t.Fatal(err)
		}
		for _, item := range []vfs.VFSItem{
			{Name: "folder", IsDir: true},
			{Name: ".hidden", IsDir: true, IsHidden: true},
			{Name: "link", IsDir: true, IsSymlink: true},
			{Name: "hidden-link", IsDir: true, IsHidden: true, IsSymlink: true},
			{Name: "junction", IsDir: true, IsSymlink: true, ReparseTag: vfs.ReparseTagMountPoint},
		} {
			t.Run(scope+"/"+item.Name, func(t *testing.T) {
				for _, selected := range []bool{false, true} {
					for _, cursor := range []bool{false, true} {
						want := old.GetColor(&item, 7, selected, cursor)
						if got := current.GetColor(&item, 7, selected, cursor); got != want {
							t.Errorf("selected=%v cursor=%v old=%x new=%x", selected, cursor, want, got)
						}
					}
				}
				if want, got := old.GetMarker(&item), current.GetMarker(&item); got != want {
					t.Errorf("marker old=%q new=%q", want, got)
				}
				row := treeItem{name: item.Name, depth: 1, vfsItem: item}
				for _, base := range []uint64{vtui.Palette[theme.ColPanelText], vtui.Palette[theme.ColPanelCursor]} {
					theme.GlobalFileHighlighter = old
					want := row.GetCellAttr(0, base)
					theme.GlobalFileHighlighter = current
					if got := row.GetCellAttr(0, base); got != want {
						t.Errorf("tree base=%x old=%x new=%x", base, want, got)
					}
				}
			})
		}
	}
}
