package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestFileEntriesFromItemsRevealsOnlyRequestedHiddenFile(t *testing.T) {
	t.Parallel()
	items := []vfs.VFSItem{
		{Name: "visible.jpg"},
		{Name: ".clipboard.png", IsHidden: true},
		{Name: ".other.png", IsHidden: true},
	}
	for _, tc := range []struct {
		name   string
		reveal string
		want   int
	}{
		{name: "ordinary listing", want: 1},
		{name: "clipboard reveal", reveal: ".clipboard.png", want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := fileEntriesFromItems(items, false, tc.reveal)
			if len(entries) != tc.want {
				t.Fatalf("visible entries = %d, want %d", len(entries), tc.want)
			}
			for _, entry := range entries {
				if entry.Name == ".other.png" {
					t.Fatal("unrelated hidden file was revealed")
				}
			}
		})
	}
}

func TestPanelNameOverflowUsesVirtualDisplayName(t *testing.T) {
	t.Parallel()
	entry := &FileEntry{VFSItem: vfs.VFSItem{
		Name: "opaque-id", DisplayName: "A longer directory label", IsDir: true,
	}}
	if got := panelNameOverflowWithOptions(entry, 10, false); got != 14 {
		t.Fatalf("display-name overflow = %d, want 14", got)
	}
	if got := formatPanelFileNameAtWithOptions(entry, 10, 14, false); got != "tory label" {
		t.Fatalf("scrolled display name = %q, want tory label", got)
	}
}
