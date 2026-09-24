package panel

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func groupTestPanel(t *testing.T) *FileSystemPanel {
	t.Helper()
	fp := NewFileSystemPanel(0, 0, 60, 16, vfs.NewNullVFS(0))
	fp.Entries = nil
	fp.SetViewMode(ViewModeDetailed)
	return fp
}

func TestGroupClassifiers(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	limits := [3]int64{5 << 20, 10 << 20, 100 << 20}
	tests := []struct {
		mode GroupMode
		item vfs.VFSItem
		want string
	}{
		{GroupName, vfs.VFSItem{Name: "ёж"}, "name:Ё"},
		{GroupName, vfs.VFSItem{Name: "e\u0301lan"}, "name:É"},
		{GroupName, vfs.VFSItem{Name: "9file"}, "Digits"},
		{GroupName, vfs.VFSItem{Name: "_file"}, "Other"},
		{GroupExtension, vfs.VFSItem{Name: ".gitignore"}, "NoExtension"},
		{GroupExtension, vfs.VFSItem{Name: "a.TAR.GZ"}, "ext:gz"},
		{GroupExtension, vfs.VFSItem{Name: "a.TXT", NoExtension: true}, "NoExtension"},
		{GroupExtension, vfs.VFSItem{Name: "folder.TXT", IsDir: true}, "ext:txt"},
		{GroupSize, vfs.VFSItem{SizeKnown: true}, "Small"},
		{GroupSize, vfs.VFSItem{}, "Unknown"},
		{GroupSize, vfs.VFSItem{Size: 5 << 20}, "Small"},
		{GroupSize, vfs.VFSItem{Size: 5<<20 + 1}, "Medium"},
		{GroupSize, vfs.VFSItem{Size: 10 << 20}, "Medium"},
		{GroupSize, vfs.VFSItem{Size: 10<<20 + 1}, "Large"},
		{GroupSize, vfs.VFSItem{Size: 100 << 20}, "Large"},
		{GroupSize, vfs.VFSItem{Size: 100<<20 + 1}, "ExtraLarge"},
		{GroupPhysicalSize, vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit | vfs.MetadataPhysicalSize}, "Small"},
		{GroupPhysicalSize, vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, PhysicalSize: 123}, "Unknown"},
		{GroupOwner, vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit | vfs.MetadataUID}, "id:0"},
		{GroupOwnerGroup, vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit | vfs.MetadataGID}, "id:0"},
		{GroupOwner, vfs.VFSItem{}, "Unknown"},
		{GroupPermissions, vfs.VFSItem{KnownMetadata: vfs.MetadataPermissions, UnixMode: 04755}, "permissions:4755"},
		{GroupPermissions, vfs.VFSItem{KnownMetadata: vfs.MetadataPermissions}, "permissions:0000"},
		{GroupAttributes, vfs.VFSItem{KnownMetadata: vfs.MetadataWinAttrs, WinAttrs: 3}, "attrs:00000003"},
		{GroupModeText, vfs.VFSItem{Mode: "REG_SZ"}, "mode:REG_SZ"},
		{GroupModeText, vfs.VFSItem{}, "Unknown"},
		{GroupKind, vfs.VFSItem{IsSymlink: true, IsDir: true}, "Links"},
		{GroupKind, vfs.VFSItem{IsSymlink: true, IsDir: true, ReparseTag: 0xa0000003}, "Junctions"},
		{GroupHidden, vfs.VFSItem{KnownMetadata: vfs.MetadataHidden}, "No"},
		{GroupExecutable, vfs.VFSItem{KnownMetadata: vfs.MetadataExecutable, IsExecutable: true}, "Yes"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			fp := &FileSystemPanel{GroupBy: tt.mode}
			got := fp.groupFor(&FileEntry{VFSItem: tt.item}, now, limits)
			if got.id != tt.want {
				t.Fatalf("got %s, want %s", got.id, tt.want)
			}
		})
	}
}

func TestGroupDatesCalendar(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, now := range []time.Time{time.Date(2026, 1, 1, 12, 0, 0, 0, loc), time.Date(2026, 3, 9, 12, 0, 0, 0, loc), time.Date(2026, 11, 2, 12, 0, 0, 0, loc)} {
		for days, want := range map[int]string{1: "Future", 0: "Today", -1: "Yesterday", -2: "TwoDaysAgo"} {
			value := now.AddDate(0, 0, days)
			if got := dateGroup(value, now, true).id; got != want {
				t.Fatalf("%s: got %s want %s", value, got, want)
			}
		}
		old := now.AddDate(0, 0, -3)
		if got := dateGroup(old, now, true).id; got != "month:"+old.Format("2006-01") {
			t.Fatal(got)
		}
		if got := dateGroup(now, now, false).id; got != "Unknown" {
			t.Fatal(got)
		}
	}
}

func TestGroupSortIndependence(t *testing.T) {
	for _, mode := range GroupModes[1:] {
		for _, folders := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%t/%t", mode.ID, folders, reverse), func(t *testing.T) {
					fp := groupTestPanel(t)
					fp.GroupBy, fp.GroupFoldersSeparately, fp.GroupReverse = mode.Mode, folders, reverse
					for i, name := range []string{"..", "beta.TXT", "alpha.go", "apple.TXT", "directory", "9zero", "empty"} {
						item := vfs.VFSItem{Name: name, IsDir: i == 0 || i == 4, Size: int64(i) * 4 << 20, SizeKnown: true, MTime: time.Date(2026, 1, i+1, 12, 0, 0, 0, time.Local), Uid: i % 2, Gid: i % 3, UnixMode: uint32(i%2) * 0111, Mode: fmt.Sprint(i % 2), IsHidden: i%2 == 0, KnownMetadata: vfs.MetadataExplicit | vfs.MetadataUID | vfs.MetadataGID | vfs.MetadataPermissions | vfs.MetadataMTime | vfs.MetadataHidden}
						fp.Entries = append(fp.Entries, &FileEntry{VFSItem: item})
					}
					fp.SortEntries()
					var want []string
					for _, g := range fp.Groups() {
						want = append(want, g.Key)
					}
					for sortMode := SortName; sortMode <= SortUnsorted; sortMode++ {
						for _, sortReverse := range []bool{false, true} {
							fp.SortMode, fp.SortReverse = sortMode, sortReverse
							fp.SortEntries()
							var got []string
							for _, g := range fp.Groups() {
								got = append(got, g.Key)
							}
							if !reflect.DeepEqual(got, want) {
								t.Fatalf("sort=%d reverse=%t groups=%v want %v", sortMode, sortReverse, got, want)
							}
							if fp.Entries[0].Name != ".." {
								t.Fatal("parent moved")
							}
							for _, g := range fp.Groups() {
								group := append([]*FileEntry(nil), fp.Entries[g.StartIndex:g.StartIndex+g.Count]...)
								for j := 1; j < len(group); j++ {
									if group[j].IsDir && !group[j-1].IsDir {
										t.Fatal("folder follows file")
									}
								}
								if sortMode != SortUnsorted {
									expected := &FileSystemPanel{Entries: append([]*FileEntry(nil), group...), SortMode: sortMode, SortReverse: sortReverse}
									expected.SortEntries()
									if !reflect.DeepEqual(group, expected.Entries) {
										t.Fatal("wrong within-group order")
									}
								}
							}
						}
					}
				})
			}
		}
	}
}

func TestGroupRowsNavigationFilter(t *testing.T) {
	before := config.App
	defer func() { config.App = before }()
	config.App.PanelAutoFilter = true
	for _, view := range []ViewMode{ViewModeDetailed, ViewModeMedium, ViewModeBrief, ViewModeWide} {
		fp := groupTestPanel(t)
		fp.SetViewMode(view)
		for _, name := range []string{"..", "a.txt", "alpha.go", "b.txt", "c.txt"} {
			fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: name}})
		}
		fp.SetGrouping(GroupName, false, false)
		if len(fp.Entries) != 5 || fp.displayCount() != 8 {
			t.Fatalf("files=%d rows=%d", len(fp.Entries), fp.displayCount())
		}
		for i := range fp.Entries {
			row := fp.displayOfEntry(i)
			if fp.entryAtDisplay(row) != i {
				t.Fatal("row mapping")
			}
			fp.SetCursorIndex(i)
			if fp.groupNavigationTarget(vtinput.VK_DOWN) != min(4, i+1) {
				t.Fatal("down")
			}
			if fp.groupNavigationTarget(vtinput.VK_UP) != max(0, i-1) {
				t.Fatal("up")
			}
		}
		fp.SetCursorIndex(1)
		for offset := 0; offset < fp.Table.ViewHeight; offset++ {
			row := fp.Table.TopPos + offset
			if row >= len(fp.displayRows) {
				break
			}
			if fp.displayRows[row].entry >= 0 {
				continue
			}
			x, y := fp.Table.X1, fp.Table.Y1+fp.Table.MarginTop+offset
			for _, button := range []uint32{vtinput.FromLeft1stButtonPressed, vtinput.RightmostButtonPressed, vtinput.FromLeft2ndButtonPressed} {
				fp.ProcessMouse(&vtinput.InputEvent{MouseX: groupMouseCoordinate(t, x), MouseY: groupMouseCoordinate(t, y), ButtonState: button, KeyDown: true, MouseEventFlags: vtinput.DoubleClick})
				if fp.GetCursorIndex() != 1 {
					t.Fatal("heading changed cursor")
				}
			}
		}
		fp.FastFindMode, fp.FastFindStr = true, "b"
		fp.updateAutoFilter()
		if len(fp.Groups()) != 1 || fp.Groups()[0].Key != "name:B" {
			t.Fatalf("filter groups %v", fp.Groups())
		}
		fp.ExitFastFind()
		if len(fp.Groups()) != 3 {
			t.Fatal("filter restore")
		}
	}
}

func TestGroupSessionCompatibility(t *testing.T) {
	old := ini.Parse(strings.NewReader("[Workspaces]\nCount=1\n[Workspace/0/Left]\nGroupBy=999\n"))
	states, _ := LoadWorkspaceSessions(old)
	if states[0].Left.GroupBy != GroupNone || !states[0].Left.GroupFoldersSeparately {
		t.Fatal(states)
	}
	states[0].Left.GroupBy = GroupOwner
	states[0].Left.GroupReverse = true
	states[0].Left.GroupFoldersSeparately = false
	var b strings.Builder
	WriteWorkspaceSessions(&b, states, 0)
	got, _ := LoadWorkspaceSessions(ini.Parse(strings.NewReader(b.String())))
	if !reflect.DeepEqual(got, states) {
		t.Fatalf("round trip: %v != %v", got, states)
	}
}

func TestGroupMidnightAndCalculatedFolders(t *testing.T) {
	fp := groupTestPanel(t)
	today := time.Date(2026, 3, 1, 12, 0, 0, 0, time.Local)
	fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "a", MTime: today}, Selected: true}}
	fp.setGroupingAt(GroupModified, false, false, today)
	if fp.Groups()[0].Key != "Today" {
		t.Fatal(fp.Groups())
	}
	fp.RefreshGrouping(today.AddDate(0, 0, 1))
	if fp.Groups()[0].Key != "Yesterday" || !fp.Entries[0].Selected {
		t.Fatal(fp.Groups())
	}
	fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true, Size: 8 << 20}, SizeCalculated: true}, {VFSItem: vfs.VFSItem{Name: "unknown", IsDir: true}}}
	fp.SetGrouping(GroupSize, false, false)
	if fp.Groups()[0].Key != "Medium" || fp.Groups()[1].Key != "Unknown" {
		t.Fatal(fp.Groups())
	}
	fp.SetSortMode(SortSize)
	if !fp.Entries[0].SizeCalculated || fp.Groups()[0].Key != "Medium" {
		t.Fatal("sort lost calculated size")
	}
	fp.SetGrouping(GroupSize, true, false)
	if fp.Groups()[len(fp.Groups())-1].Key != "Unknown" {
		t.Fatal("reversed missing data")
	}
}

func TestGroupFrameHeadingDoesNotActivate(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	before := config.App
	defer func() { config.App = before }()
	config.App.NavigationMode = config.NavigationClassic
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	waitForDirectoryLoads(t)
	fp := pf.Panels[0].(*FileSystemPanel)
	fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}, {VFSItem: vfs.VFSItem{Name: "a.txt"}}}
	fp.SetViewMode(ViewModeDetailed)
	fp.SetGrouping(GroupName, false, false)
	fp.SetCursorIndex(0)
	pf.ActiveIdx = 1
	for _, button := range []uint32{vtinput.FromLeft1stButtonPressed, vtinput.RightmostButtonPressed, vtinput.FromLeft2ndButtonPressed} {
		for _, flags := range []uint32{0, vtinput.DoubleClick} {
			path := fp.Vfs.GetPath()
			pf.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: groupMouseCoordinate(t, fp.Table.X1), MouseY: groupMouseCoordinate(t, fp.Table.Y1+fp.Table.MarginTop+1), ButtonState: button, MouseEventFlags: flags})
			if pf.ActiveIdx != 1 || fp.GetCursorIndex() != 0 || fp.Vfs.GetPath() != path || pf.middleMouseDown {
				t.Fatalf("heading button=%d flags=%d active=%d cursor=%d path=%s old=%s middle=%t visible=%t hit=%t top=%d", button, flags, pf.ActiveIdx, fp.GetCursorIndex(), fp.Vfs.GetPath(), path, pf.middleMouseDown, fp.IsVisible(), fp.groupHeadingAt(fp.Table.X1, fp.Table.Y1+fp.Table.MarginTop+1), fp.Table.TopPos)
			}
			pf.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: groupMouseCoordinate(t, fp.Table.X1), MouseY: groupMouseCoordinate(t, fp.Table.Y1+fp.Table.MarginTop+1)})
		}
	}
}

func TestGroupOneRowNavigationAndSemantics(t *testing.T) {
	fp := groupTestPanel(t)
	for i := 0; i < 12; i++ {
		fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("%c.txt", 'A'+i)}})
	}
	fp.SetGrouping(GroupName, false, false)
	for _, mode := range []ViewMode{ViewModeDetailed, ViewModeMedium, ViewModeBrief} {
		fp.SetViewMode(mode)
		fp.Table.ViewHeight = 1
		fp.Refresh()
		for _, key := range []uint16{vtinput.VK_END, vtinput.VK_HOME, vtinput.VK_NEXT, vtinput.VK_PRIOR, vtinput.VK_DOWN, vtinput.VK_UP, vtinput.VK_RIGHT, vtinput.VK_LEFT} {
			fp.SetCursorIndex(fp.groupNavigationTarget(key))
			fp.Refresh()
			if fp.entryAtDisplay(fp.displayOfEntry(fp.GetCursorIndex())) != fp.GetCursorIndex() {
				t.Fatal("cursor on heading")
			}
		}
	}
	fp.SetViewMode(ViewModeDetailed)
	fp.Table.TopPos = 1
	fp.SetCursorIndex(0)
	model := fp.SemanticPanelModel(&vtui.SemanticContext{}, 0, true)
	if model.TotalCount != 12 || len(model.Groups) != 12 || model.Groups[0].StartIndex != 0 || model.Groups[0].Count != 1 {
		t.Fatalf("%+v", model)
	}
	if model.Top != fp.FileTop() || model.DisplayTop != fp.Table.TopPos {
		t.Fatal("semantic anchor")
	}
}

func TestGroupHeadingPainting(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	fp := groupTestPanel(t)
	fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "a.txt"}}, {VFSItem: vfs.VFSItem{Name: "b.txt"}}}
	for _, mode := range []ViewMode{ViewModeDetailed, ViewModeMedium, ViewModeBrief, ViewModeWide} {
		fp.SetViewMode(mode)
		fp.SetGrouping(GroupName, false, false)
		fp.Table.TopPos = 0
		fp.Refresh()
		fp.Show(scr)
		y := fp.Table.Y1 + fp.Table.MarginTop
		width := fp.Table.X2 - fp.Table.X1 + 1
		if fp.gridColumnCount() > 1 {
			width = fp.Table.Columns[0].Width
		}
		padding := (width - 3) / 2
		row := testutil.ScreenRow(scr, y, fp.Table.X1, fp.Table.X1+width-1)
		want := strings.Repeat("─", padding) + " A " + strings.Repeat("─", width-padding-3)
		if row != want {
			t.Fatalf("mode %d heading %q", mode, row)
		}
		if got := scr.GetCell(fp.Table.X1+padding+1, y).Attributes; got != vtui.Palette[theme.ColPanelColumnTitle] {
			t.Fatalf("heading attr %x", got)
		}
		for _, x := range []int{fp.Table.X1, fp.Table.X1 + width - 1} {
			if got := scr.GetCell(x, y).Attributes; got != vtui.Palette[fp.Table.ColorBoxIdx] {
				t.Fatalf("separator attr %x", got)
			}
		}
	}
}

func TestGroupStickyHeading(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	for _, mode := range []ViewMode{ViewModeDetailed, ViewModeMedium, ViewModeBrief, ViewModeWide} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			fp := groupTestPanel(t)
			fp.SetViewMode(mode)
			for _, prefix := range []string{"a", "b"} {
				for i := range 60 {
					fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("%s%02d", prefix, i)}})
				}
			}
			fp.SetGrouping(GroupName, false, false)
			for _, idx := range []int{10, 59, 60, 70} {
				fp.Table.TopPos = fp.displayOfEntry(idx)
				fp.SetCursorIndex(idx)
				fp.Refresh()
				fp.Show(scr)
				x, y := fp.Table.X1, fp.Table.Y1+fp.Table.MarginTop
				if !fp.groupHeadingAt(x, y) || fp.mouseEntryIndex(x, y) != -1 {
					t.Fatal("pinned heading accepts file input")
				}
				if fp.mouseEntryIndex(x, y+1) != idx {
					t.Fatal("pinned heading obscures the first visible file")
				}
				width := fp.Table.X2 - fp.Table.X1 + 1
				if fp.gridColumnCount() > 1 {
					width = fp.Table.Columns[0].Width
				}
				want := " A "
				if idx >= 60 {
					want = " B "
				}
				if row := testutil.ScreenRow(scr, y, x, x+width-1); !strings.Contains(row, want) {
					t.Fatalf("wrong pinned group at %d: %q", idx, row)
				}
				before := fp.GetCursorIndex()
				fp.ProcessMouse(&vtinput.InputEvent{
					Type: vtinput.MouseEventType, KeyDown: true,
					MouseX: groupMouseCoordinate(t, x), MouseY: groupMouseCoordinate(t, y),
					ButtonState: vtinput.FromLeft1stButtonPressed,
				})
				if fp.GetCursorIndex() != before {
					t.Fatal("click on pinned heading moved cursor")
				}
				// Every file cell follows the same projection across column boundaries.
				next := fp.Table.TopPos
				for col := range fp.gridColumnCount() {
					for offset := range fp.Table.ViewHeight {
						if col == 0 && offset == 0 {
							continue
						}
						if got := fp.viewportDisplayRow(fp.Table.TopPos+offset, col); got != next {
							t.Fatalf("projection skipped row: %d != %d", got, next)
						}
						next++
					}
				}
			}
			for _, direction := range []int{1, -1} {
				for i := range len(fp.Entries) {
					idx := i
					if direction < 0 {
						idx = len(fp.Entries) - 1 - i
					}
					fp.SetCursorIndex(idx)
					fp.Refresh()
					if got := fp.entryIndex(fp.Table.SelectPos, fp.Table.SelectCol); got != idx {
						t.Fatalf("cursor obscured: %d != %d", got, idx)
					}
				}
			}
			_, _, maxTop, _, _ := fp.panelScrollMetrics()
			fp.Table.TopPos = maxTop
			last := fp.viewportDisplayRow(maxTop+fp.Table.ViewHeight-1, fp.gridColumnCount()-1)
			if last != fp.displayCount()-1 {
				t.Fatalf("scrollbar cannot reach last row: %d", last)
			}
			fp.Table.ViewHeight = 1
			if fp.stickyGroupHeading(fp.displayOfEntry(10)) != -1 {
				t.Fatal("heading consumed the only file row")
			}
		})
	}
}

func TestGroupUnsortedArrivalOrder(t *testing.T) {
	fp := groupTestPanel(t)
	for _, name := range []string{"beta", "apple", "bravo", "apricot"} {
		fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: name}})
	}
	fp.SetGrouping(GroupName, true, false)
	fp.SortMode = SortUnsorted
	fp.SortEntries()
	var got []string
	for _, e := range fp.Entries {
		got = append(got, e.Name)
	}
	if !reflect.DeepEqual(got, []string{"beta", "bravo", "apple", "apricot"}) {
		t.Fatal(got)
	}
	fp.SetGrouping(GroupNone, false, false)
	got = nil
	for _, e := range fp.Entries {
		got = append(got, e.Name)
	}
	if !reflect.DeepEqual(got, []string{"beta", "apple", "bravo", "apricot"}) {
		t.Fatal(got)
	}
}

func TestGroupLegacySortPriorities(t *testing.T) {
	before := GlobalSortGroups
	defer func() { GlobalSortGroups = before }()
	GlobalSortGroups = &SortGroupSet{}
	GlobalSortGroups.LoadFromIni(ini.Parse(strings.NewReader("[SortGroup_1]\nName=Go\nGroup=1\nMask=*.go\n")))
	for sortMode := SortName; sortMode <= SortUnsorted; sortMode++ {
		for _, reverse := range []bool{false, true} {
			fp := groupTestPanel(t)
			fp.SortMode = sortMode
			fp.SortReverse = reverse
			fp.UseSortGroups = true
			for _, name := range []string{"a.txt", "a.go", "b.txt", "b.go"} {
				fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: name}})
			}
			fp.SetGrouping(GroupName, false, false)
			if len(fp.Groups()) != 2 || fp.Entries[0].Name != "a.go" || fp.Entries[2].Name != "b.go" {
				t.Fatalf("sort=%d reverse=%t: %v", sortMode, reverse, fp.Groups())
			}
		}
	}
}

func TestGroupShiftAndDragAcrossHeadings(t *testing.T) {
	fp := groupTestPanel(t)
	for _, name := range []string{"..", "alpha", "beta", "gamma"} {
		fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: name}})
	}
	fp.SetGrouping(GroupName, false, false)
	fp.SetCursorIndex(0)
	fp.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END, ControlKeyState: vtinput.ShiftPressed})
	if fp.Entries[0].Selected {
		t.Fatal("parent selected")
	}
	for _, e := range fp.Entries[1:] {
		if !e.Selected {
			t.Fatal("range omitted file")
		}
	}
	fp.SetAllItemsSelected(false)
	x, y := fp.Table.X1, fp.Table.Y1+fp.Table.MarginTop
	for _, row := range []int{fp.displayOfEntry(1), fp.displayOfEntry(2) - 1, fp.displayOfEntry(2)} {
		fp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: groupMouseCoordinate(t, x), MouseY: groupMouseCoordinate(t, y+row-fp.Table.TopPos), ButtonState: vtinput.RightmostButtonPressed})
	}
	fp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: groupMouseCoordinate(t, x), MouseY: groupMouseCoordinate(t, y)})
	if fp.rightDragActive || fp.rowDragButton != 0 || !fp.Entries[1].Selected || !fp.Entries[2].Selected {
		t.Fatal("drag selection or release lost")
	}
}

func TestGroupOpaqueModeCollation(t *testing.T) {
	fp := groupTestPanel(t)
	for i, mode := range []string{"type", "TYPE", "é", "e\u0301", "type", "TYPE", "é", "e\u0301"} {
		fp.Entries = append(fp.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprint(i), Mode: mode}})
	}
	fp.SetGrouping(GroupModeText, false, false)
	for _, reverse := range []bool{false, true} {
		fp.SortReverse = reverse
		fp.SortEntries()
		if len(fp.Groups()) != 4 {
			t.Fatalf("equivalent strings interleaved: %v", fp.Groups())
		}
		for _, g := range fp.Groups() {
			if g.Count != 2 {
				t.Fatal(g)
			}
		}
	}
}

func groupMouseCoordinate(t *testing.T, value int) int16 {
	t.Helper()
	if value < 0 || value > 32767 {
		t.Fatalf("mouse coordinate out of range: %d", value)
		return 0
	}
	return int16(value)
}
