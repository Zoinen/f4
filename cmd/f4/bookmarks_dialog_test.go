package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
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
	if err := os.Mkdir(path, 0o700); err != nil {
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

func TestBookmarksDialog_SlotAtValidatesMenuData(t *testing.T) {
	menu := vtui.NewVMenu("Bookmarks")
	menu.Items = []vtui.MenuItem{
		{UserData: 3},
		{UserData: "not a slot"},
		{UserData: 10},
	}
	d := &bookmarksDialog{menu: menu}

	if got := d.slotAt(0); got != 3 {
		t.Fatalf("valid menu slot = %d, want 3", got)
	}
	for _, pos := range []int{-1, 1, 2, 3} {
		if got := d.slotAt(pos); got != -1 {
			t.Errorf("slotAt(%d) = %d, want -1", pos, got)
		}
	}

	d.menu = nil
	if got := d.slotAt(0); got != -1 {
		t.Fatalf("slotAt without menu = %d, want -1", got)
	}
}

func TestBookmarksDialog_SizeHonorsConsoleBounds(t *testing.T) {
	d := &bookmarksDialog{pf: &PanelsFrame{lastW: 10, lastH: 8}}
	width, height := d.size()
	if width < 24 {
		t.Fatalf("minimum dialog width = %d, want at least 24", width)
	}
	if height != 5 {
		t.Fatalf("short-console dialog height = %d, want 5", height)
	}

	d.pf = &PanelsFrame{lastW: 200, lastH: 40}
	_, height = d.size()
	if height != len(d.set)+2 {
		t.Fatalf("normal dialog height = %d, want %d", height, len(d.set)+2)
	}
}

func TestBookmarksDialog_RenderPersistAndMutate(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	d := &bookmarksDialog{
		file: path,
		set: BookmarkSet{
			1: {Path: "/one"},
			2: {Path: "/two", Plugin: "old"},
		},
		menu: vtui.NewVMenu("Bookmarks"),
	}
	d.render()
	if len(d.menu.Items) != len(d.set) {
		t.Fatalf("rendered %d rows, want %d", len(d.menu.Items), len(d.set))
	}
	if got := d.slotAt(2); got != 2 {
		t.Fatalf("rendered row user data = %d, want 2", got)
	}

	d.moveSlot(1, 1)
	if d.set[2].Path != "/one" || d.menu.SelectPos != 2 {
		t.Fatalf("move result = %#v, cursor %d; want /one in slot 2 and cursor 2", d.set, d.menu.SelectPos)
	}
	d.moveSlot(0, -1)
	if d.set[0].Path != "" {
		t.Fatal("out-of-range move changed slot 0")
	}

	d.clearSlot(2)
	if !d.set[2].IsEmpty() {
		t.Fatal("clearSlot did not empty slot 2")
	}
	d.clearSlot(-1)

	got, err := LoadBookmarks(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got[2].IsEmpty() || got[1].Path != "/two" {
		t.Fatalf("persisted bookmarks = %#v, want slot 1 /two and empty slot 2", got)
	}
}

func TestBookmarksDialog_SaveCurrentDirReplacesPluginBookmark(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := &PanelsFrame{activeIdx: 0}
	pf.panels[0] = &FileSystemPanel{vfs: vfs.NewNullVFS(0)}
	wantPath := pf.panels[0].(*FileSystemPanel).vfs.GetPath()
	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	d := &bookmarksDialog{
		pf:   pf,
		file: path,
		set:  BookmarkSet{4: {Path: "/old", Plugin: "NetRocks", PluginData: "sftp://host"}},
		menu: vtui.NewVMenu("Bookmarks"),
	}

	d.saveCurrentDir(4)
	if got := d.set[4]; got != (Bookmark{Path: wantPath}) {
		t.Fatalf("saved active directory = %#v, want local root %q bookmark", got, wantPath)
	}
	d.saveCurrentDir(-1)
}

func TestBookmarksDialog_OpenBuildsMenu(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	d := &bookmarksDialog{
		pf:   &PanelsFrame{lastW: 80, lastH: 25},
		file: path,
		set:  BookmarkSet{4: {Path: "/work"}},
	}
	d.open(4, nil)
	if d.menu == nil || len(d.menu.Items) != len(d.set) {
		t.Fatalf("open menu = %#v, want %d rows", d.menu, len(d.set))
	}
	if d.menu.SelectPos != 4 {
		t.Fatalf("open cursor = %d, want 4", d.menu.SelectPos)
	}
	vtui.FrameManager.Pop()
}
