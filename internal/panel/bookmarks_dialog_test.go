package panel

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestBookmarksDialog_SlotAtValidatesMenuData(t *testing.T) {
	menu := vtui.NewVMenu("Bookmarks")
	menu.Items = []vtui.MenuItem{
		{UserData: 3},
		{UserData: "not a slot"},
		{UserData: 10},
	}
	d := &BookmarksDialog{menu: menu}

	if got := d.SlotAt(0); got != 3 {
		t.Fatalf("valid menu slot = %d, want 3", got)
	}
	for _, pos := range []int{-1, 1, 2, 3} {
		if got := d.SlotAt(pos); got != -1 {
			t.Errorf("slotAt(%d) = %d, want -1", pos, got)
		}
	}

	d.menu = nil
	if got := d.SlotAt(0); got != -1 {
		t.Fatalf("slotAt without menu = %d, want -1", got)
	}
}

func TestBookmarksDialog_SizeHonorsConsoleBounds(t *testing.T) {
	d := &BookmarksDialog{Pf: &PanelsFrame{LastW: 10, LastH: 8}}
	width, height := d.size()
	if width < 24 {
		t.Fatalf("minimum dialog width = %d, want at least 24", width)
	}
	if height != 5 {
		t.Fatalf("short-console dialog height = %d, want 5", height)
	}

	d.Pf = &PanelsFrame{LastW: 200, LastH: 40}
	_, height = d.size()
	if height != len(d.Set)+2 {
		t.Fatalf("normal dialog height = %d, want %d", height, len(d.Set)+2)
	}
}

func TestBookmarksDialog_RenderPersistAndMutate(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	d := &BookmarksDialog{
		File: path,
		Set: BookmarkSet{
			1: {Path: "/one"},
			2: {Path: "/two", Plugin: "old"},
		},
		menu: vtui.NewVMenu("Bookmarks"),
	}
	d.render()
	if len(d.menu.Items) != len(d.Set) {
		t.Fatalf("rendered %d rows, want %d", len(d.menu.Items), len(d.Set))
	}
	if got := d.SlotAt(2); got != 2 {
		t.Fatalf("rendered row user data = %d, want 2", got)
	}

	d.moveSlot(1, 1)
	if d.Set[2].Path != "/one" || d.menu.SelectPos != 2 {
		t.Fatalf("move result = %#v, cursor %d; want /one in slot 2 and cursor 2", d.Set, d.menu.SelectPos)
	}
	d.moveSlot(0, -1)
	if d.Set[0].Path != "" {
		t.Fatal("out-of-range move changed slot 0")
	}

	d.clearSlot(2)
	if !d.Set[2].IsEmpty() {
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
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := &PanelsFrame{ActiveIdx: 0}
	pf.Panels[0] = &FileSystemPanel{Vfs: vfs.NewNullVFS(0)}
	wantPath := pf.Panels[0].(*FileSystemPanel).Vfs.GetPath()
	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	d := &BookmarksDialog{
		Pf:   pf,
		File: path,
		Set:  BookmarkSet{4: {Path: "/old", Plugin: "NetRocks", PluginData: "sftp://host"}},
		menu: vtui.NewVMenu("Bookmarks"),
	}

	d.saveCurrentDir(4)
	if got := d.Set[4]; got != (Bookmark{Path: wantPath}) {
		t.Fatalf("saved active directory = %#v, want local root %q bookmark", got, wantPath)
	}
	d.saveCurrentDir(-1)
}

func TestBookmarksDialog_OpenBuildsMenu(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	path := filepath.Join(t.TempDir(), "bookmarks.ini")
	d := &BookmarksDialog{
		Pf:   &PanelsFrame{LastW: 80, LastH: 25},
		File: path,
		Set:  BookmarkSet{4: {Path: "/work"}},
	}
	d.open(4, nil)
	if d.menu == nil || len(d.menu.Items) != len(d.Set) {
		t.Fatalf("open menu = %#v, want %d rows", d.menu, len(d.Set))
	}
	if d.menu.SelectPos != 4 {
		t.Fatalf("open cursor = %d, want 4", d.menu.SelectPos)
	}
	vtui.FrameManager.Pop()
}
