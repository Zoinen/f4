package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFileSystemPanel_SortingLeadingUnderscoreUsesStringCollation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewNullVFS(0))
	if fp.cancelLoad != nil {
		fp.cancelLoad()
	}

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "unxed", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "_test_folder", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "9test", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "archive", IsDir: true}},
	}
	fp.sortMode = SortName
	fp.sortEntries()

	want := []string{"_test_folder", "9test", "archive", "unxed"}
	for i, name := range want {
		if got := fp.entries[i].Name; got != name {
			t.Fatalf("sorted entry %d = %q, want %q; all entries = %v", i, got, name, entryNames(fp.entries))
		}
	}
}

func TestFileSystemPanel_SortingTiesRemainDeterministicWhenReversed(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewNullVFS(0))
	if fp.cancelLoad != nil {
		fp.cancelLoad()
	}

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "same-b", Size: 10}},
		{VFSItem: vfs.VFSItem{Name: "same-a", Size: 10}},
	}
	fp.sortMode = SortSize
	fp.sortReverse = true
	fp.sortEntries()

	if got := entryNames(fp.entries); got[0] != "same-b" || got[1] != "same-a" {
		t.Fatalf("reversed equal-size sort = %v, want [same-b same-a]", got)
	}
}

func entryNames(entries []*fileEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}
