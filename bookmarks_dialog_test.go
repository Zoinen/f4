package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwapSlots_MovesItemAndPreservesOthers(t *testing.T) {
	var set BookmarkSet
	set[3] = Bookmark{Path: "/three"}
	set[4] = Bookmark{Path: "/four"}
	set[7] = Bookmark{Path: "/seven"}

	set.swapSlots(3, 4)

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
	var set BookmarkSet
	set[0] = Bookmark{Path: "/zero"}
	set[9] = Bookmark{Path: "/nine"}
	before := set

	// Out-of-range indices are ignored, so the caller can pass
	// "cursor ± 1" from either end without checking first.
	set.swapSlots(0, -1)
	set.swapSlots(9, 10)
	set.swapSlots(-5, 100)

	if set != before {
		t.Fatalf("out-of-range swap mutated the table:\ngot  %#v\nwant %#v", set, before)
	}
}

func TestDeleteAtSlot_ClearsCompletely(t *testing.T) {
	var set BookmarkSet
	set[2] = Bookmark{
		Path:       "/some/path",
		Plugin:     "NetRocks",
		PluginData: "sftp://host",
		PluginFile: "file.txt",
	}

	set.deleteAtSlot(2)

	if !set[2].IsEmpty() {
		t.Fatalf("slot not empty after delete: %#v", set[2])
	}
	if set[2] != (Bookmark{}) {
		t.Errorf("delete left residue behind: %#v", set[2])
	}
}

func TestSetCurrentDir_ReplacesPathAndWipesPluginFields(t *testing.T) {
	var set BookmarkSet
	set[5] = Bookmark{
		Path:       "/old",
		Plugin:     "NetRocks",
		PluginData: "sftp://host",
		PluginFile: "file.txt",
	}

	set.setCurrentDir(5, "/new/cwd")

	want := Bookmark{Path: "/new/cwd"}
	if set[5] != want {
		t.Fatalf("got %#v, want %#v", set[5], want)
	}
}

func TestNewBookmarksDialog_LoadFailureReturnsError(t *testing.T) {
	// A directory where the INI is expected: opening succeeds, reading
	// does not. The dialog must report that instead of crashing.
	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := newBookmarksDialog(nil, path)
	if err == nil {
		t.Fatalf("expected an error for an unreadable file, got dialog %#v", d)
	}
	if d != nil {
		t.Errorf("no dialog should be built on failure, got %#v", d)
	}
}

func TestBookmarksDialog_RowTextShowsPathOrEmptyMarker(t *testing.T) {
	d := &bookmarksDialog{}
	d.set[6] = Bookmark{Path: "/mnt/d/work & play"}

	filled := d.rowText(6)
	if !strings.Contains(filled, "6") || !strings.Contains(filled, "/mnt/d/work && play") {
		t.Errorf("row 6 = %q, want the slot digit and the escaped path", filled)
	}
	if empty := d.rowText(0); !strings.Contains(empty, Msg("Bookmarks.EmptySlot")) {
		t.Errorf("row 0 = %q, want the empty marker", empty)
	}
}
