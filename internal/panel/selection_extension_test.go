package panel

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestCurrentExtensionSelection(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	items := []vfs.VFSItem{
		{Name: "..", IsDir: true},
		{Name: "one.TXT"}, {Name: "two.txt"},
		{Name: "archive.tar.gz"}, {Name: "other.gz"},
		{Name: "folder.txt", IsDir: true}, {Name: "other", IsDir: true},
		{Name: "README"}, {Name: "virtual.txt", NoExtension: true},
		{Name: "one.Я"}, {Name: "two.я"},
		{Name: ".env"}, {Name: "sample.env"},
	}
	for _, tc := range []struct {
		name   string
		cursor int
		want   []int
	}{
		{name: "case insensitive", cursor: 1, want: []int{1, 2}},
		{name: "last extension", cursor: 3, want: []int{3, 4}},
		{name: "directories", cursor: 5, want: []int{5, 6}},
		{name: "extensionless", cursor: 7, want: []int{7, 8}},
		{name: "virtual extensionless", cursor: 8, want: []int{7, 8}},
		{name: "unicode", cursor: 9, want: []int{9, 10}},
		{name: "dotfile", cursor: 11, want: []int{11, 12}},
		{name: "parent", cursor: 0, want: []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := &FileSystemPanel{Vfs: vfs.NewNullVFS(0), CursorIdx: tc.cursor}
			for _, item := range items {
				fp.Entries = append(fp.Entries, &FileEntry{VFSItem: item})
			}
			// A mark outside the matching set must survive select and deselect.
			fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: "keep.log"}})
			keep := len(fp.Entries) - 1
			fp.SetItemSelected(keep, true)
			fp.ApplyCurrentExtensionSelection(true)
			selected := []int{}
			for i, entry := range fp.Entries[:keep] {
				if entry.Selected {
					selected = append(selected, i)
				}
			}
			if !reflect.DeepEqual(selected, tc.want) || !fp.Entries[keep].Selected {
				t.Fatalf("selection=%v, want %v; unrelated mark=%t", selected, tc.want, fp.Entries[keep].Selected)
			}
			fp.ApplyCurrentExtensionSelection(false)
			for _, entry := range fp.Entries[:keep] {
				if entry.Selected || fp.SelectedItems[entry.Name] {
					t.Fatalf("%s remained selected", entry.Name)
				}
			}
			if tc.cursor != 0 {
				fp.RestoreSelection()
				for _, i := range tc.want {
					if !fp.Entries[i].Selected || !fp.SelectedItems[fp.Entries[i].Name] {
						t.Fatalf("restore missed %s", fp.Entries[i].Name)
					}
				}
			}
			if fp.GetCursorIndex() != tc.cursor || !fp.Entries[keep].Selected {
				t.Fatal("cursor or unrelated mark changed")
			}
		})
	}
	(&FileSystemPanel{}).ApplyCurrentExtensionSelection(true)
}

func TestCurrentExtensionSelectionRespectsAutoFilter(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.PanelAutoFilter = true
	fp := NewFileSystemPanel(0, 0, 60, 20, vfs.NewNullVFS(0))
	fp.setEntries([]*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "visible.txt"}},
		{VFSItem: vfs.VFSItem{Name: "hidden.txt"}},
	})
	fp.FastFindMode = true
	fp.FastFindStr = "visible"
	fp.refilterEntries()
	if len(fp.Entries) != 1 {
		t.Fatalf("filter returned %d files", len(fp.Entries))
	}
	fp.ApplyCurrentExtensionSelection(true)
	fp.ExitFastFind()
	for _, entry := range fp.Entries {
		if entry.Selected != (entry.Name == "visible.txt") {
			t.Fatalf("filter-hidden row was marked: %s", entry.Name)
		}
	}
}
