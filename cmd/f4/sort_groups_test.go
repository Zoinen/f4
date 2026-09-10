package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const testSortGroupsIni = `
[SortGroup_2]
Name = Images
Mask = *.png, *.jpg

[SortGroup_1]
Name = Executables
Group = 0
IncludeAttributes = executable
ExcludeAttributes = directory

[SortGroup_10]
Mask = *.txt
Group = 5
`

func useTestSortGroups(t *testing.T, ini string) {
	t.Helper()
	previous := GlobalSortGroups
	GlobalSortGroups = &SortGroupSet{}
	GlobalSortGroups.LoadFromIni(ParseIni(strings.NewReader(ini)))
	t.Cleanup(func() { GlobalSortGroups = previous })
}

func newSortGroupPanel(t *testing.T) *FileSystemPanel {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewNullVFS(0))
	if fp.cancelLoad != nil {
		fp.cancelLoad()
	}
	return fp
}

func TestParseSortGroupsOrdersSectionsNumericallyAndHonoursGroupKey(t *testing.T) {
	groups := parseSortGroups(ParseIni(strings.NewReader(testSortGroupsIni)))
	if len(groups) != 3 {
		t.Fatalf("parsed %d groups, want 3", len(groups))
	}
	// SortGroup_10 must come after SortGroup_2: the suffix is a number, not a
	// string.
	wantNames := []string{"Executables", "Images", "*.txt"}
	for i, want := range wantNames {
		if groups[i].Name != want {
			t.Errorf("group %d name = %q, want %q", i, groups[i].Name, want)
		}
	}
	wantOrders := []int{0, 1, 5}
	for i, want := range wantOrders {
		if groups[i].Order != want {
			t.Errorf("group %d order = %d, want %d", i, groups[i].Order, want)
		}
	}
}

func TestSortGroupsReuseHighlightRules(t *testing.T) {
	ini := ParseIni(strings.NewReader(`[Highlight_4]
Name = Archives
Group = 1
Mask = *.zip, *.7z

[Highlight_5]
Name = Images
Group = 2
Mask = *.png, *.jpg

[Highlight_6]
Name = Unsorted highlight rule
Mask = *.txt
`))
	groups := sortGroupsFromHighlightRules(parseHighlightRules(ini))
	if len(groups) != 2 {
		t.Fatalf("shared highlight rules produced %d sort groups, want 2", len(groups))
	}
	if groups[0].Name != "Archives" || groups[0].Order != 1 {
		t.Fatalf("first shared group = %#v, want Archives at 1", groups[0])
	}
	if groups[1].Name != "Images" || groups[1].Order != 2 {
		t.Fatalf("second shared group = %#v, want Images at 2", groups[1])
	}

	archive := vfs.VFSItem{Name: "backup.ZIP"}
	if got := groups[0].Filter.Match(&archive, true); !got {
		t.Fatal("shared highlight matcher did not match archive")
	}
}

func TestSortGroupSetLoadsSharedAndLegacyRules(t *testing.T) {
	ini := ParseIni(strings.NewReader(`[Highlight_1]
Name = Images
Group = 0
Mask = *.png

[SortGroup_1]
Name = Legacy
Group = 1
Mask = *.legacy
`))
	rules := parseHighlightRules(ini)
	set := &SortGroupSet{}
	set.LoadFromIni(ini, rules)
	if len(set.Groups) != 2 {
		t.Fatalf("loaded %d sort groups, want shared and legacy rules", len(set.Groups))
	}

	image := vfs.VFSItem{Name: "photo.png"}
	legacy := vfs.VFSItem{Name: "old.legacy"}
	if got := set.GroupOf(&image); got != 0 {
		t.Errorf("shared rule group = %d, want 0", got)
	}
	if got := set.GroupOf(&legacy); got != 1 {
		t.Errorf("legacy rule group = %d, want 1", got)
	}
}

func TestSortGroupSetGroupOfFallsBackToDefaultGroup(t *testing.T) {
	useTestSortGroups(t, testSortGroupsIni)

	cases := []struct {
		item vfs.VFSItem
		want int
	}{
		{vfs.VFSItem{Name: "run.sh", IsExecutable: true}, 0},
		{vfs.VFSItem{Name: "photo.PNG"}, 1},
		{vfs.VFSItem{Name: "notes.txt"}, 5},
		{vfs.VFSItem{Name: "data.bin"}, defaultSortGroupOrder},
	}
	for _, tc := range cases {
		item := tc.item
		if got := GlobalSortGroups.GroupOf(&item); got != tc.want {
			t.Errorf("GroupOf(%q) = %d, want %d", item.Name, got, tc.want)
		}
	}
}

func TestSortEntriesClustersByGroupBeforeSortMode(t *testing.T) {
	useTestSortGroups(t, testSortGroupsIni)
	fp := newSortGroupPanel(t)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "readme.md"}},
		{VFSItem: vfs.VFSItem{Name: "b.png"}},
		{VFSItem: vfs.VFSItem{Name: "notes.txt"}},
		{VFSItem: vfs.VFSItem{Name: "sub", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "run.sh", IsExecutable: true}},
		{VFSItem: vfs.VFSItem{Name: "a.png"}},
	}
	fp.sortMode = SortName
	fp.useSortGroups = true
	fp.sortEntries()

	want := []string{"..", "sub", "run.sh", "a.png", "b.png", "notes.txt", "readme.md"}
	if got := entryNames(fp.entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("grouped sort = %v, want %v", got, want)
	}
}

func TestSortEntriesKeepsGroupOrderWhenReversed(t *testing.T) {
	useTestSortGroups(t, testSortGroupsIni)
	fp := newSortGroupPanel(t)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "a.png"}},
		{VFSItem: vfs.VFSItem{Name: "readme.md"}},
		{VFSItem: vfs.VFSItem{Name: "b.png"}},
		{VFSItem: vfs.VFSItem{Name: "run.sh", IsExecutable: true}},
	}
	fp.sortMode = SortName
	fp.sortReverse = true
	fp.useSortGroups = true
	fp.sortEntries()

	// Reversing flips the order inside a group; the groups themselves stay
	// where their configuration put them.
	want := []string{"run.sh", "b.png", "a.png", "readme.md"}
	if got := entryNames(fp.entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("reversed grouped sort = %v, want %v", got, want)
	}
}

func TestSortEntriesGroupsUnsortedPanelWithoutReordering(t *testing.T) {
	useTestSortGroups(t, testSortGroupsIni)
	fp := newSortGroupPanel(t)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "z.md"}},
		{VFSItem: vfs.VFSItem{Name: "b.png"}},
		{VFSItem: vfs.VFSItem{Name: "a.md"}},
		{VFSItem: vfs.VFSItem{Name: "a.png"}},
	}
	fp.sortMode = SortUnsorted
	fp.useSortGroups = true
	fp.sortEntries()

	// Only the clustering moves rows: inside a group the original order stays.
	want := []string{"b.png", "a.png", "z.md", "a.md"}
	if got := entryNames(fp.entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("grouped unsorted panel = %v, want %v", got, want)
	}
}

func TestSortEntriesIgnoresGroupsWhenPanelHasThemOff(t *testing.T) {
	useTestSortGroups(t, testSortGroupsIni)
	fp := newSortGroupPanel(t)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "b.png"}},
		{VFSItem: vfs.VFSItem{Name: "a.md"}},
	}
	fp.sortMode = SortName
	fp.sortEntries()

	want := []string{"a.md", "b.png"}
	if got := entryNames(fp.entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("ungrouped sort = %v, want %v", got, want)
	}
}

func TestSortEntriesWithoutConfiguredGroupsIsANoOp(t *testing.T) {
	useTestSortGroups(t, "")
	fp := newSortGroupPanel(t)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "b.md"}},
		{VFSItem: vfs.VFSItem{Name: "a.md"}},
	}
	fp.sortMode = SortUnsorted
	fp.useSortGroups = true
	fp.sortEntries()

	if got := entryNames(fp.entries); !reflect.DeepEqual(got, []string{"b.md", "a.md"}) {
		t.Fatalf("unsorted panel was reordered without configured groups: %v", got)
	}
}

func TestWorkspaceSessionRoundTripsSortGroupFlag(t *testing.T) {
	states := []workspaceSessionState{{
		Number: 1, ActivePanel: 0, WidePanel: -1,
		ShowPanels: true, ShowLeft: true, ShowRight: true,
		Left:  panelSessionState{Path: "/left", ViewMode: int(ViewModeMedium), SortMode: int(SortName), UseSortGroups: true},
		Right: panelSessionState{Path: "/right", ViewMode: int(ViewModeMedium), SortMode: int(SortName)},
	}}

	for i := range states {
		states[i].Left.Gallery = panelGallerySessionState{LayoutMode: GalleryLayoutMasonry, ColumnCount: 2, Densities: map[GalleryLayoutMode]int{}}
		states[i].Right.Gallery = states[i].Left.Gallery
	}
	var encoded strings.Builder
	writeWorkspaceSessions(&encoded, states, 0)
	got, _ := loadWorkspaceSessions(ParseIni(strings.NewReader(encoded.String())))
	if !reflect.DeepEqual(got, states) {
		t.Fatalf("sort-group flag did not survive the session round trip:\n got: %#v\nwant: %#v", got, states)
	}
}
