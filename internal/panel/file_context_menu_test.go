package panel

import (
	"context"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/filemenu"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func menuTestPanels(t *testing.T) (*PanelsFrame, *FileSystemPanel) {
	t.Helper()
	fp := &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir()), CursorIdx: 1}
	fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}, {VFSItem: vfs.VFSItem{Name: "α & 'one.txt"}}, {VFSItem: vfs.VFSItem{Name: "two.txt"}}}
	return &PanelsFrame{ShowPanels: true, Panels: [2]Panel{fp, fp}}, fp
}

func TestFileContextMenuSelection(t *testing.T) {
	pf, fp := menuTestPanels(t)
	s := captureFileMenuSelection(pf)
	if s == nil || !reflect.DeepEqual(s.names, []string{"α & 'one.txt"}) {
		t.Fatalf("selection=%+v", s)
	}
	fp.Entries[2].Selected = true
	s = captureFileMenuSelection(pf)
	if !reflect.DeepEqual(s.names, []string{"two.txt"}) {
		t.Fatalf("marked selection=%v", s.names)
	}
	fp.CursorIdx = 0
	if s = captureFileMenuSelection(pf); s == nil || len(s.names) != 1 {
		t.Fatal("parent cursor suppressed marked files")
	}
	fp.Entries[2].Selected = false
	if captureFileMenuSelection(pf) != nil {
		t.Fatal("parent entry targeted")
	}
	pf.ShowPanels = false
	if captureFileMenuSelection(pf) != nil {
		t.Fatal("hidden panels targeted")
	}
}

func TestFileContextMenuAnchorTracksVisibleCursor(t *testing.T) {
	_, fp := menuTestPanels(t)
	fp.Table = vtui.NewTable(41, 3, 38, 12, []vtui.TableColumn{{Width: 12}, {Width: 12}, {Width: 12}})
	fp.Table.ViewHeight = 10
	fp.Table.MarginTop = 1
	for _, tc := range []struct {
		mode              ViewMode
		top, cursor, x, y int
	}{
		{ViewModeDetailed, 20, 23, 41, 8},
		{ViewModeMedium, 20, 33, 54, 8},
		{ViewModeBrief, 20, 43, 67, 8},
		{ViewModeDetailed, 20, 29, 41, 14},
	} {
		fp.ViewMode = tc.mode
		fp.Table.TopPos = tc.top
		fp.CursorIdx = tc.cursor
		x, y := fileMenuAnchor(fp)
		if x != tc.x || y != tc.y {
			t.Errorf("%+v: got %d,%d", tc, x, y)
		}
	}
}

func TestFileContextMenuPlacementUsesExplicitAnchorAndClamps(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	m := vtui.NewVMenu("Files")
	m.AddItem(vtui.MenuItem{Text: "Open"})
	placeFileMenu(m, 12, 7)
	if x, y, _, _ := m.GetPosition(); x != 12 || y != 7 {
		t.Fatalf("anchor was ignored: %d,%d", x, y)
	}
	placeFileMenu(m, 79, 24)
	if x, y, x2, y2 := m.GetPosition(); x < 0 || y < 0 || x2 >= 80 || y2 >= 25 {
		t.Fatalf("outside viewport: %d,%d,%d,%d", x, y, x2, y2)
	}
}

func TestFileContextMenuStaleSelectionNeverDispatches(t *testing.T) {
	pf, fp := menuTestPanels(t)
	s := captureFileMenuSelection(pf)
	old := ContextMenuCommand
	ContextMenuCommand = func(*PanelsFrame, string) { t.Fatal("stale command dispatched") }
	t.Cleanup(func() { ContextMenuCommand = old })
	fp.CursorIdx = 2
	s.invoke("trash", nil, false)
	fp.CursorIdx = 1
	fp.Vfs = vfs.NewOSVFS(t.TempDir())
	s.invoke("trash", nil, false)
	pf.Closed = true
	if s.valid() {
		t.Fatal("closed frame accepted")
	}
}

func TestFileContextMenuFallbackCapabilities(t *testing.T) {
	pf, _ := menuTestPanels(t)
	s := captureFileMenuSelection(pf)
	entries := s.entries(false, false, true)
	items := fallbackMenuItems(entries, nil, func(string) {})
	for _, entry := range entries {
		if entry.ID == "open" || entry.ID == "edit" || entry.ID == "trash" || entry.ID == "rename" || entry.ID == "move" {
			if !entry.Disabled {
				t.Errorf("read-only/nonlocal action enabled: %s", entry.ID)
			}
		}
	}
	if len(items) == 0 {
		t.Fatal("virtual-file fallback has no operations")
	}
	called := ""
	items = fallbackMenuItems([]filemenu.Entry{{ID: "open-with", Label: "Open With"}}, []filemenu.Entry{{ID: "app:test.desktop", Label: "Test"}}, func(id string) { called = id })
	items[0].SubItems[0].OnClick()
	if called != "app:test.desktop" {
		t.Fatal("submenu did not dispatch its application")
	}
}

func TestFileContextMenuFrameCancelsHelper(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	for _, viaKey := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		m := &fileMenuFrame{VMenu: vtui.NewVMenu("Files"), cancel: cancel}
		m.AddItem(vtui.MenuItem{Text: "Open"})
		if viaKey {
			m.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
		} else {
			m.Close()
		}
		if ctx.Err() == nil {
			t.Fatal("closing menu did not cancel native helper")
		}
	}
}

func TestFileContextMenuPendingDoesNotDraw(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(50, 20)
	vtui.FrameManager.Init(scr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &fileMenuFrame{VMenu: vtui.NewVMenu("Files"), cancel: cancel, pending: true}
	m.SetPosition(2, 2, 32, 8)
	m.AddItem(vtui.MenuItem{Text: "Open"})
	before := scr.GetCell(2, 2)
	m.Show(scr)
	if got := scr.GetCell(2, 2); !reflect.DeepEqual(got, before) || m.HasShadow() {
		t.Fatal("pending native request drew a menu or shadow")
	}
	m.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if !m.IsDone() || ctx.Err() == nil {
		t.Fatal("invisible pending request did not cancel on Escape")
	}
}

func TestFileContextMenuFallbackDispatchesCapturedPanel(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf, fp := menuTestPanels(t)
	oldRunner, oldCommand := FileMenuRunner, ContextMenuCommand
	oldMenu, oldOwner := activeFileMenu, activeFileMenuOwner
	t.Cleanup(func() {
		FileMenuRunner = oldRunner
		ContextMenuCommand = oldCommand
		activeFileMenu = oldMenu
		activeFileMenuOwner = oldOwner
	})
	FileMenuRunner = func(context.Context, filemenu.Request) filemenu.Result {
		return filemenu.Result{Outcome: filemenu.Unavailable}
	}
	called := false
	ContextMenuCommand = func(owner *PanelsFrame, id string) {
		if owner != pf || owner.GetActivePanel() != fp || id != "properties" {
			t.Fatalf("retargeted context action: %s", id)
		}
		called = true
	}
	ShowFileContextMenu(pf)
	m := activeFileMenu
	runInfoPanelUITasksUntil(t, func() bool { return len(m.Items) > 1 })
	// Change the active panel after capture: dispatch must restore the source.
	pf.ActiveIdx = 1
	pf.Panels[1] = &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir())}
	for _, item := range m.Items {
		if item.Text == fileMenuLabel("properties", "Properties…") {
			item.OnClick()
			break
		}
	}
	runInfoPanelUITasksUntil(t, func() bool { return called })
	if !m.IsDone() {
		t.Fatal("menu remained open after dispatch")
	}
}

func TestFileContextMenuUsesLivePalette(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(50, 20)
	vtui.FrameManager.Init(scr)
	previous := append([]uint64(nil), vtui.Palette...)
	t.Cleanup(func() { copy(vtui.Palette, previous) })
	menu := vtui.NewVMenu("Files")
	menu.SetPosition(2, 2, 32, 8)
	for i := 0; i < 12; i++ {
		menu.AddItem(vtui.MenuItem{Text: "&Open"})
	}
	indices := []int{vtui.ColMenuText, vtui.ColMenuSelectedText, vtui.ColMenuHighlight, vtui.ColMenuSelectedHighlight, vtui.ColMenuBox, vtui.ColMenuTitle, vtui.ColScrollBar}
	for pass := 0; pass < 2; pass++ {
		menu.SetSelectPos(0)
		for i, index := range indices {
			vtui.Palette[index] = uint64(0x20 + pass*0x20 + i)
		}
		menu.Show(scr)
		for _, check := range []struct{ x, y, index int }{{6, 3, vtui.ColMenuSelectedText}, {4, 3, vtui.ColMenuSelectedHighlight}, {6, 4, vtui.ColMenuText}, {4, 4, vtui.ColMenuHighlight}, {2, 2, vtui.ColMenuBox}, {32, 4, vtui.ColScrollBar}} {
			if got := scr.GetCell(check.x, check.y).Attributes; got != vtui.Palette[check.index] {
				t.Errorf("pass %d at %d,%d: got %x want %x", pass, check.x, check.y, got, vtui.Palette[check.index])
			}
		}
		menu.SetSelectPos(1)
		menu.Show(scr)
		if got := scr.GetCell(6, 4).Attributes; got != vtui.Palette[vtui.ColMenuSelectedText] {
			t.Fatal("new selection did not use current palette")
		}
	}
}
