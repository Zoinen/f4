package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestPanelSelectionByName(t *testing.T) {
	fp := &panel.FileSystemPanel{
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "a.png"}},
		},
		SelectedItems: map[string]bool{},
	}

	if !fp.SetSelectedByName("a.png", true) {
		t.Fatal("the panel does show that entry")
	}
	if !fp.IsNameSelected("a.png") {
		t.Error("the entry did not get picked")
	}
	if fp.SetSelectedByName("gone.png", true) {
		t.Error("a name the panel does not show cannot be picked")
	}
	fp.SetSelectedByName("a.png", false)
	if fp.IsNameSelected("a.png") {
		t.Error("the entry did not get unpicked")
	}
}
