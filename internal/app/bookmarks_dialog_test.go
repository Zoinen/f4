package app

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwapSlots_MovesItemAndPreservesOthers(t *testing.T) {
	var set panel.BookmarkSet
	set[3] = panel.Bookmark{Path: "/three"}
	set[4] = panel.Bookmark{Path: "/four"}
	set[7] = panel.Bookmark{Path: "/seven"}

	set.SwapSlots(3, 4)

	if set[3].Path != "/four" || set[4].Path != "/three" {
		t.Fatalf("swap failed: [3]=%q [4]=%q", set[3].Path, set[4].Path)
	}
	if set[7].Path != "/seven" {
		t.Errorf("unrelated slot 7 changed: %q", set[7].Path)
	}
	for _, i := range []int{0, 1, 2, 5, 6, 8, 9} {
		if !set[i].IsEmpty() {
			t.Errorf("slot %d should still be empty: %#v", i, set[i])
		}
	}
}

func TestSwapSlots_ClampsWithinRange(t *testing.T) {
	var set panel.BookmarkSet
	set[0] = panel.Bookmark{Path: "/zero"}
	set[9] = panel.Bookmark{Path: "/nine"}
	before := set

	// Out-of-range indices are ignored, so the caller can pass
	// "cursor ± 1" from either end without checking first.
	set.SwapSlots(0, -1)
	set.SwapSlots(9, 10)
	set.SwapSlots(-5, 100)

	if set != before {
		t.Fatalf("out-of-range swap mutated the table:\ngot  %#v\nwant %#v", set, before)
	}
}

func TestDeleteAtSlot_ClearsCompletely(t *testing.T) {
	var set panel.BookmarkSet
	set[2] = panel.Bookmark{
		Path:       "/some/path",
		Plugin:     "NetRocks",
		PluginData: "sftp://host",
		PluginFile: "file.txt",
	}

	set.DeleteAtSlot(2)

	if !set[2].IsEmpty() {
		t.Fatalf("slot not empty after delete: %#v", set[2])
	}
	if set[2] != (panel.Bookmark{}) {
		t.Errorf("delete left residue behind: %#v", set[2])
	}
}

func TestSetCurrentDir_ReplacesPathAndWipesPluginFields(t *testing.T) {
	var set panel.BookmarkSet
	set[5] = panel.Bookmark{
		Path:       "/old",
		Plugin:     "NetRocks",
		PluginData: "sftp://host",
		PluginFile: "file.txt",
	}

	set.SetCurrentDir(5, "/new/cwd")

	want := panel.Bookmark{Path: "/new/cwd"}
	if set[5] != want {
		t.Fatalf("got %#v, want %#v", set[5], want)
	}
}

func TestNewBookmarksDialog_LoadFailureReturnsError(t *testing.T) {
	// A directory where the INI is expected: opening succeeds, reading
	// does not. The dialog must report that instead of crashing.
	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	d, err := panel.NewBookmarksDialog(nil, path)
	if err == nil {
		t.Fatalf("expected an error for an unreadable file, got dialog %#v", d)
	}
	if d != nil {
		t.Errorf("no dialog should be built on failure, got %#v", d)
	}
}

func TestBookmarksDialog_RowTextShowsPathOrEmptyMarker(t *testing.T) {
	d := &panel.BookmarksDialog{}
	d.Set[6] = panel.Bookmark{Path: "/mnt/d/work & play"}

	filled := d.RowText(6)
	if !strings.Contains(filled, "6") || !strings.Contains(filled, "/mnt/d/work && play") {
		t.Errorf("row 6 = %q, want the slot digit and the escaped path", filled)
	}
	if empty := d.RowText(0); !strings.Contains(empty, i18n.Msg("Bookmarks.EmptySlot")) {
		t.Errorf("row 0 = %q, want the empty marker", empty)
	}
}
