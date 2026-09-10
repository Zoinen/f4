package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestPanelImageSiblings(t *testing.T) {
	fp := &panel.FileSystemPanel{
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "sub", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "a.png"}},
			{VFSItem: vfs.VFSItem{Name: "notes.txt"}},
			{VFSItem: vfs.VFSItem{Name: "b.jpg"}},
		},
		CursorIdx: 4,
	}

	names, index := fp.ImageSiblings()
	if len(names) != 2 || names[0] != "a.png" || names[1] != "b.jpg" {
		t.Fatalf("the pictures of the panel: %v", names)
	}
	if index != 1 {
		t.Errorf("the cursor is on the second picture, got %d", index)
	}

	// A cursor on something that is not a picture has no position.
	fp.CursorIdx = 3
	if _, index := fp.ImageSiblings(); index != -1 {
		t.Errorf("expected no position, got %d", index)
	}
}
