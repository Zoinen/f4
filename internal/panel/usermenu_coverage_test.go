package panel

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestUserMenuStateTracksNestedItemsAndBreadcrumbs(t *testing.T) {
	root := []UserMenuItem{
		{HotKey: "a", Label: "&Top", Commands: []string{"top"}},
		{HotKey: "b", Label: "Sub &&Menu", Submenu: []UserMenuItem{{Label: "&Child", Commands: []string{"child"}}}},
	}
	s := &userMenuState{rootTitle: "User menu", rootItems: root, path: []int{1}}

	if got := s.currentItems(); len(got) != 1 || got[0].Label != "&Child" {
		t.Fatalf("currentItems() = %#v, want child level", got)
	}
	if got := s.currentTitle(); got != "User menu -> Sub &Menu" {
		t.Fatalf("currentTitle() = %q", got)
	}

	replacement := []UserMenuItem{{Label: "&Replacement"}}
	s.replaceCurrentItems(replacement)
	if !reflect.DeepEqual(s.rootItems[1].Submenu, replacement) {
		t.Fatalf("replaceCurrentItems() did not replace nested level: %#v", s.rootItems)
	}

	s.path = []int{99}
	if got := s.currentItems(); got != nil {
		t.Fatalf("invalid currentItems() = %#v, want nil", got)
	}
	if got := s.currentTitle(); got != "User menu" {
		t.Fatalf("invalid currentTitle() = %q, want root title", got)
	}
	s.path = nil
	s.replaceCurrentItems([]UserMenuItem{{Label: "new root"}})
	if len(s.rootItems) != 1 || s.rootItems[0].Label != "new root" {
		t.Fatalf("root replacement failed: %#v", s.rootItems)
	}
}

func TestItemIndexAtUIRejectsInvalidRows(t *testing.T) {
	menu := vtui.NewVMenu("test")
	menu.AddItem(vtui.MenuItem{Text: "item", UserData: 4})
	menu.AddItem(vtui.MenuItem{Text: "separator"})
	if got := itemIndexAtUI(menu, 0); got != 4 {
		t.Errorf("itemIndexAtUI(item) = %d, want 4", got)
	}
	for _, pos := range []int{-1, 1, 2} {
		if got := itemIndexAtUI(menu, pos); got != -1 {
			t.Errorf("itemIndexAtUI(%d) = %d, want -1", pos, got)
		}
	}
}

func TestUserMenuSaveAndLoadRootForBothFormats(t *testing.T) {
	items := []UserMenuItem{
		{HotKey: "a", Label: "Build", Commands: []string{"go build", "echo done"}},
		{HotKey: "b", Label: "Tools", Submenu: []UserMenuItem{{HotKey: "x", Label: "Clean", Commands: []string{"go clean"}}}},
	}
	for _, tc := range []struct {
		name string
		mode MenuMode
	}{
		{name: "far menu", mode: MenuModeLocal},
		{name: "main menu", mode: MenuModeMain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "menu.ini")
			if err := SaveRootForMode(tc.mode, path, items); err != nil {
				t.Fatalf("SaveRootForMode() error = %v", err)
			}
			got := loadRootForMode(tc.mode, path)
			if !reflect.DeepEqual(got, items) {
				t.Fatalf("loadRootForMode() = %#v, want %#v", got, items)
			}
		})
	}

	if got := loadRootForMode(MenuModeLocal, filepath.Join(t.TempDir(), "missing.ini")); got != nil {
		t.Fatalf("missing FarMenu.ini = %#v, want nil", got)
	}
}

func TestLoadMenuForModeAndResolveMenuStartUseLocalMenu(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FarMenuFileName)
	items := []UserMenuItem{{HotKey: "l", Label: "Local", Commands: []string{"echo local"}}}
	if err := SaveRootForMode(MenuModeLocal, path, items); err != nil {
		t.Fatal(err)
	}
	fsp := &FileSystemPanel{Vfs: vfs.NewOSVFS(dir)}
	pf := &PanelsFrame{ActiveIdx: 0, Panels: [2]Panel{fsp, nil}}

	got, _, source, ok := LoadMenuForMode(pf, MenuModeLocal)
	if !ok || !reflect.DeepEqual(got, items) || source != path {
		t.Fatalf("LoadMenuForMode() = %#v, %q, %q, %v", got, source, source, ok)
	}
	resolved, _, mode, resolvedSource := resolveMenuStart(pf, MenuModeLocal)
	if mode != MenuModeLocal || resolvedSource != path || !reflect.DeepEqual(resolved, items) {
		t.Fatalf("resolveMenuStart() = %#v, mode=%d source=%q", resolved, mode, resolvedSource)
	}
}

func TestUserMenuFallbackAndSaveRoot(t *testing.T) {
	dir := t.TempDir()
	fsp := &FileSystemPanel{Vfs: vfs.NewOSVFS(dir)}
	pf := &PanelsFrame{ActiveIdx: 0, Panels: [2]Panel{fsp, nil}}
	items, _, mode, source := resolveMenuStart(pf, MenuModeLocal)
	if items != nil || mode != MenuModeMain || source == "" {
		t.Fatalf("empty resolveMenuStart() = %#v, mode=%d source=%q", items, mode, source)
	}

	state := &userMenuState{mode: MenuModeLocal, SourcePath: filepath.Join(dir, FarMenuFileName), rootItems: []UserMenuItem{{Label: "saved"}}}
	if !state.saveRoot() {
		t.Fatal("saveRoot() failed for a writable menu")
	}
	if got := loadRootForMode(MenuModeLocal, state.SourcePath); len(got) != 1 || got[0].Label != "saved" {
		t.Fatalf("saved root = %#v", got)
	}
	if (&userMenuState{}).saveRoot() {
		t.Fatal("saveRoot() succeeded without a source path")
	}
}

func TestMenuSizeRespectsHintSubmenusAndConsoleBounds(t *testing.T) {
	pf := &PanelsFrame{LastW: 80, LastH: 25}
	items := []UserMenuItem{{Label: "short"}, {HotKey: "--"}, {Label: strings.Repeat("long", 20)}}
	w, h := menuSize(pf, len(items), items, true)
	if w < len(userMenuBottomHint)+2 || h != len(items)+2 {
		t.Fatalf("menuSize() = %d x %d, want hint width and natural height", w, h)
	}

	narrow := &PanelsFrame{LastW: 30, LastH: 8}
	w, h = menuSize(narrow, 20, nil, false)
	if w < 24 || w > narrow.LastW-4 || h != 5 {
		t.Fatalf("narrow menuSize() = %d x %d, want bounded dimensions", w, h)
	}
}
