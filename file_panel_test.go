package main

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestFileEntry_GetCellText(t *testing.T) {
	// Ensure predictable environment for this test
	orig := AppConfig.HighlightDir
	AppConfig.HighlightDir = false
	defer func() { AppConfig.HighlightDir = orig }()
	// Mock entries
	file := &fileEntry{VFSItem: vfs.VFSItem{Name: "test.txt", Size: 1024, IsDir: false}}
	dir := &fileEntry{VFSItem: vfs.VFSItem{Name: "work", IsDir: true}}

	// 1. Column 0 (Name)
	if file.GetCellText(0) != "test.txt" {
		t.Errorf("File name mismatch: %s", file.GetCellText(0))
	}
	if dir.GetCellText(0) != string(os.PathSeparator)+"work" {
		t.Errorf("Dir name mismatch: %s", dir.GetCellText(0))
	}

	// 2. Column 1 (Size)
	if file.GetCellText(1) != "1 024" {
		t.Errorf("File size mismatch: %s", file.GetCellText(1))
	}

	// Regular directories should have an empty size column
	if dir.GetCellText(1) != "" {
		t.Errorf("Regular dir should have empty size column, got: %q", dir.GetCellText(1))
	}

	// Only ".." directory should have the UP-DIR placeholder
	upDir := &fileEntry{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}
	if upDir.GetCellText(1) != "UP-DIR" {
		t.Errorf("Parent dir (..) should have UP-DIR placeholder, got: %q", upDir.GetCellText(1))
	}

	// Test IsCached coloring
	cachedFile := &fileEntry{VFSItem: vfs.VFSItem{Name: "cache.txt"}, IsCached: true}
	baseAttr := uint64(0x00AABBCC) // Mock color
	cachedAttr := cachedFile.GetCellAttr(0, baseAttr)
	if cachedAttr == baseAttr {
		t.Error("Cached file should return a modified (dimmed) attribute, got same")
	}
}
func TestFileEntry_HighlightDir(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	// Protect global config
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()

	dir := &fileEntry{VFSItem: vfs.VFSItem{Name: "work", IsDir: true}}

	// 1. Without highlighting
	AppConfig.HighlightDir = false

	if dir.GetCellText(0) != string(os.PathSeparator)+"work" {
		t.Errorf("Expected separator prefix when HighlightDir is false, got %q", dir.GetCellText(0))
	}
	if dir.GetCellAttr(0, 0) != 0 {
		t.Error("Expected default attribute when HighlightDir is false")
	}

	// 2. With highlighting
	AppConfig.HighlightDir = true
	if dir.GetCellText(0) != "work" {
		t.Errorf("Expected raw name when HighlightDir is true, got %q", dir.GetCellText(0))
	}
	if dir.GetCellAttr(0, 0) != vtui.Palette[ColPanelDir] {
		t.Error("Expected ColPanelDir attribute when HighlightDir is true")
	}

	// Reset global state
	AppConfig.HighlightDir = false
}

func TestFileSystemPanel_FocusLoss_FastFind(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	fp.fastFindMode = true
	fp.fastFindStr = "test"

	// Имитируем потерю фокуса файловой панелью (например, открыто меню)
	fp.SetFocus(false)

	if fp.fastFindMode || fp.fastFindStr != "" {
		t.Error("Focus loss should deactivate FastFind mode")
	}
}

type mockTitleVFS struct {
	vfs.OSVFS
	title string
}

func (m *mockTitleVFS) GetTitle() string { return m.title }

func TestFileSystemPanel_UpdateTitle_WithProvider(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	v := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/tmp"), title: "user@host"}
	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	// Reset loading flag to avoid [Loading...] suffix
	fp.isLoading = false
	fp.updateTitle(nil)

	got := fp.currentTitle
	if !strings.Contains(got, "user@host:") {
		t.Errorf("Expected title to contain 'user@host:', got %q", got)
	}
}

func TestFileSystemPanel_ShowHiddenFiles(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Protect global config from leakage
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()

	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "normal.txt"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmp, ".hidden.txt"), []byte(""), 0644)

	v := vfs.NewOSVFS(tmp)

	// 1. Show hidden files
	AppConfig.ShowHiddenFiles = true
	fp1 := NewFileSystemPanel(0, 0, 80, 24, v)
	waitForLoad(t, fp1)

	foundHidden := false
	for _, e := range fp1.entries {
		if e.Name == ".hidden.txt" {
			foundHidden = true
			break
		}
	}
	if !foundHidden {
		t.Error("Hidden file should be visible when ShowHiddenFiles is true")
	}

	// 2. Hide hidden files
	AppConfig.ShowHiddenFiles = false
	fp2 := NewFileSystemPanel(0, 0, 80, 24, v)
	waitForLoad(t, fp2)

	foundHidden = false
	for _, e := range fp2.entries {
		if e.Name == ".hidden.txt" {
			foundHidden = true
			break
		}
	}
	if foundHidden {
		t.Error("Hidden file should NOT be visible when ShowHiddenFiles is false")
	}
}

func TestFileSystemPanel_NavigateUp_Selection(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmp := t.TempDir()
	sub := filepath.Join(tmp, "target_folder")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(tmp, "other.txt"), []byte(""), 0644)

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(sub))

	// Drain tasks to finish loading the initial directory
	timeout := time.After(1 * time.Second)
	for fp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for initial load")
		}
	}
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			goto done1
		}
	}
done1:

	// Simulate pressing Enter on ".."
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fp.SetCursorIndex(0)
	fp.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	// Wait for the parent directory to finish loading
	timeout = time.After(1 * time.Second)
	for fp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for parent load")
		}
	}

	// Pump any remaining UI rendering/selection tasks
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			goto done2
		}
	}
done2:

	// Ensure that after returning to the parent directory, the cursor is on the folder we just exited
	if fp.GetSelectedName() != "target_folder" {
		t.Errorf("Expected cursor to land on 'target_folder', got %q", fp.GetSelectedName())
	}
}
func TestFileSystemPanel_SelectedInfo(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	if fp.cancelLoad != nil {
		fp.cancelLoad()
	}
	fp.isLoading = false
	if fp.loadingTimer != nil {
		fp.loadingTimer.Stop()
	}

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "file1.txt", Size: 1234567, IsDir: false}, Selected: true},
		{VFSItem: vfs.VFSItem{Name: "folder1", IsDir: true}, Selected: true},
		{VFSItem: vfs.VFSItem{Name: "file2.txt", Size: 50, IsDir: false}, Selected: false},
	}
	fp.Refresh()

	fp.Show(scr)

	var sb strings.Builder
	for x := 0; x < 80; x++ {
		cell := scr.GetCell(x, 23)
		if cell.Char != 0 && cell.Char != ' ' {
			sb.WriteRune(rune(cell.Char))
		}
	}

	result := sb.String()
	expectedBytes := "1234567"
	if !strings.Contains(result, "Bytes:") || !strings.Contains(result, expectedBytes) {
		t.Errorf("Expected bottom bar to contain formatted bytes %q, got: %q", expectedBytes, result)
	}
	if !strings.Contains(result, "files:1") {
		t.Errorf("Expected bottom bar to contain 'files:1', got: %q", result)
	}
	if !strings.Contains(result, "folders:1") {
		t.Errorf("Expected bottom bar to contain 'folders:1', got: %q", result)
	}
}

func TestFileSystemPanel_Initialization(t *testing.T) {
	if ViewModeMedium != 0 || ViewModeDetailed != 1 {
		t.Fatalf("legacy view mode values changed: Medium=%d Detailed=%d", ViewModeMedium, ViewModeDetailed)
	}
	// Verify that NewFileSystemPanel initializes with valid geometry to prevent collapsed panels
	x, y, w, h := 10, 5, 40, 20
	fp := NewFileSystemPanel(x, y, w, h, vfs.NewOSVFS("."))

	if fp.X1 != x || fp.Y1 != y || fp.X2 != x+w-1 || fp.Y2 != y+h-1 {
		t.Errorf("Panel coordinates not initialized correctly: got (%d,%d)-(%d,%d)", fp.X1, fp.Y1, fp.X2, fp.Y2)
	}

	// Internal table must match panel interior (excluding borders)
	tx1, ty1, tx2, ty2 := fp.table.GetPosition()
	expectedTy2 := y + h - 2
	if h > 6 {
		expectedTy2 = y + h - 4
	}
	if tx1 != x+1 || ty1 != y+1 || tx2 != x+w-2 || ty2 != expectedTy2 {
		t.Errorf("Internal table coordinates mismatch: got (%d,%d)-(%d,%d)", tx1, ty1, tx2, ty2)
	}

	if fp.viewMode != ViewModeMedium {
		t.Errorf("Default view mode should be Medium, got %v", fp.viewMode)
	}

	if !fp.table.CellSelection {
		t.Error("Medium mode should have CellSelection enabled on the table")
	}
}
func TestMediumRow_GetCellText(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 12, vfs.NewOSVFS("."))
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "test.txt", IsDir: false}},
		{VFSItem: vfs.VFSItem{Name: "work", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
	}
	fp.SetViewMode(ViewModeMedium)

	mRow := &mediumRow{fp: fp, r: 0}

	if mRow.GetCellText(0) != "test.txt" {
		t.Errorf("Expected 'test.txt', got %q", mRow.GetCellText(0))
	}
	if mRow.GetCellText(1) != "" {
		t.Errorf("Out of bounds should be empty")
	}

	fp.entries = make([]*fileEntry, 10)
	for i := 0; i < 10; i++ {
		fp.entries[i] = &fileEntry{VFSItem: vfs.VFSItem{Name: "f"}}
	}
	fp.entries[0].Name = "Left"
	fp.entries[7].Name = "Right"
	mRow = &mediumRow{fp: fp, r: 0}
	if mRow.GetCellText(0) != "Left" {
		t.Errorf("Expected 'Left', got %q", mRow.GetCellText(0))
	}
	if mRow.GetCellText(1) != "Right" {
		t.Errorf("Expected 'Right', got %q", mRow.GetCellText(1))
	}
}

func TestBriefRowAndColumns(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 90, 12, vfs.NewOSVFS("."))
	fp.entries = make([]*fileEntry, 20)
	for i := range fp.entries {
		fp.entries[i] = &fileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("file-%d", i)}}
	}
	fp.SetViewMode(ViewModeBrief)
	if len(fp.table.Columns) != 3 || !fp.table.CellSelection {
		t.Fatalf("Brief layout has %d columns, CellSelection=%v", len(fp.table.Columns), fp.table.CellSelection)
	}
	row := &mediumRow{fp: fp, r: 0}
	h := fp.table.ViewHeight
	for col := 0; col < 3; col++ {
		want := fmt.Sprintf("file-%d", col*h)
		if got := row.GetCellText(col); got != want {
			t.Errorf("column %d = %q, want %q", col, got, want)
		}
	}
	fp.SetCursorIndex(2*h + 1)
	if fp.table.SelectCol != 2 || fp.table.SelectPos != 1 {
		t.Errorf("Brief cursor mapping: pos=%d col=%d, want pos=1 col=2", fp.table.SelectPos, fp.table.SelectCol)
	}
}

func TestFileEntryModifiedCell(t *testing.T) {
	entry := &fileEntry{VFSItem: vfs.VFSItem{MTime: time.Date(2026, 8, 4, 12, 34, 0, 0, time.Local)}}
	if got := entry.GetCellText(2); got != "04.08.26 12:34" {
		t.Fatalf("modified cell = %q", got)
	}
	entry.MTime = time.Time{}
	if got := entry.GetCellText(2); got != "" {
		t.Fatalf("zero modified cell = %q, want empty", got)
	}
}

func TestFormatPanelFileNameSeparateExtension(t *testing.T) {
	oldConfig := AppConfig
	defer func() { AppConfig = oldConfig }()
	AppConfig.SeparateFileExtensions = true
	AppConfig.HighlightDir = true

	entry := &fileEntry{VFSItem: vfs.VFSItem{Name: "report.txt"}}
	if got, want := formatPanelFileName(entry, 20), "report           txt"; got != want {
		t.Fatalf("separate extension = %q, want %q", got, want)
	}
	entry.Name = "source.go"
	if got, want := formatPanelFileName(entry, 20), "source           go "; got != want {
		t.Fatalf("short extension = %q, want %q", got, want)
	}
	entry.Name = "archive.longext"
	if got, want := formatPanelFileName(entry, 20), "archive      longext"; got != want {
		t.Fatalf("long extension = %q, want %q", got, want)
	}
	if got := runewidth.StringWidth(formatPanelFileName(entry, 20)); got != 20 {
		t.Fatalf("formatted width = %d, want 20", got)
	}

	for _, name := range []string{"README", ".gitignore", "trailing."} {
		entry.Name = name
		if got := formatPanelFileName(entry, 20); got != name {
			t.Errorf("name %q unexpectedly split: %q", name, got)
		}
	}

	entry.Name = "folder.ext"
	entry.IsDir = true
	if got := formatPanelFileName(entry, 20); got != "folder.ext" {
		t.Fatalf("directory extension was separated: %q", got)
	}
}

func TestSeparateExtensionAppliesToEveryViewMode(t *testing.T) {
	oldConfig := AppConfig
	defer func() { AppConfig = oldConfig }()
	AppConfig.SeparateFileExtensions = true

	fp := NewFileSystemPanel(0, 0, 90, 12, vfs.NewOSVFS("."))
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "sample.go"}}}
	for _, mode := range []ViewMode{ViewModeBrief, ViewModeMedium, ViewModeDetailed, ViewModeWide} {
		fp.SetViewMode(mode)
		fp.Refresh()
		text := fp.table.Rows[0].GetCellText(0)
		if !strings.HasPrefix(text, "sample") || !strings.HasSuffix(text, "go ") {
			t.Errorf("mode %v did not separate extension: %q", mode, text)
		}
		if got, want := runewidth.StringWidth(text), fp.table.Columns[0].Width; got != want {
			t.Errorf("mode %v formatted width=%d, want %d", mode, got, want)
		}
	}
}

func TestFileSystemPanel_InfoLineRendering(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	// Force sync items for deterministic state
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "test.txt", Size: 1024}},
	}
	fp.Refresh()
	fp.SetCursorIndex(1)

	// Simply calling Show() validates that the string truncations and layouts
	// don't panic on normal, short or extreme configurations.
	fp.Show(scr)
}
func TestFileSystemPanel_CursorMapping(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 12, vfs.NewOSVFS("."))

	// Simulate 20 items manually so Refresh() doesn't wipe them
	fp.entries = make([]*fileEntry, 20)
	for i := range fp.entries {
		fp.entries[i] = &fileEntry{VFSItem: vfs.VFSItem{Name: "file"}}
	}

	// 1. Medium Mode (Column-Major)
	fp.SetViewMode(ViewModeMedium)
	fp.Refresh()
	fp.SetCursorIndex(3) // Index 3: Row 3, Col 0
	if fp.table.SelectPos != 3 || fp.table.SelectCol != 0 {
		t.Errorf("Medium mapping index 3: expected pos 3 col 0, got pos %d col %d", fp.table.SelectPos, fp.table.SelectCol)
	}

	fp.SetCursorIndex(10) // Index 10 with H=7 -> Col 1, Row 3
	if fp.table.SelectPos != 3 || fp.table.SelectCol != 1 {
		t.Errorf("Medium mapping index 10: expected pos 3 col 1, got pos %d col %d", fp.table.SelectPos, fp.table.SelectCol)
	}

	// 2. Detailed Mode
	fp.SetViewMode(ViewModeDetailed)
	fp.Refresh()
	fp.SetCursorIndex(5)
	if fp.table.SelectPos != 5 || fp.table.SelectCol != 0 {
		t.Errorf("Detailed mapping failed: expected pos 5, got %d", fp.table.SelectPos)
	}
}

func TestFileSystemPanel_SelectName(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.SetViewMode(ViewModeDetailed)

	// Mock entries
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "a_folder", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "z_folder", IsDir: true}},
	}

	fp.SelectName("z_folder")

	if fp.table.SelectPos != 2 {
		t.Errorf("SelectName failed: expected index 2, got %d", fp.table.SelectPos)
	}

	// Should not change position if name not found
	fp.SelectName("non_existent")
	if fp.table.SelectPos != 2 {
		t.Errorf("SelectName should not change position on failure, got %d", fp.table.SelectPos)
	}
}

func TestFileSystemPanel_MultiSelect(t *testing.T) {
	// 1. Setup real TempDir with files
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "file1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tmp, "file2.txt"), []byte("2"), 0644)
	os.WriteFile(filepath.Join(tmp, "file3.txt"), []byte("3"), 0644)

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(tmp))
	fp.viewMode = ViewModeDetailed

	// Bypass async ReadDirectory for precise testing
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file1.txt"}},
		{VFSItem: vfs.VFSItem{Name: "file2.txt"}},
		{VFSItem: vfs.VFSItem{Name: "file3.txt"}},
	}
	fp.Refresh()

	// 2. Select file1.txt (Index 1)
	fp.SetCursorIndex(1)
	fp.Refresh()

	// Press Insert
	fp.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT})

	if !fp.entries[1].Selected {
		t.Error("file1.txt (index 1) should be selected after Insert")
	}

	// Cursor should move to file2.txt (Index 2)
	if fp.GetCursorIndex() != 2 {
		t.Errorf("Cursor should move to 2, got %d", fp.GetCursorIndex())
	}

	// 3. Shift+Down at cursor=2 (file2.txt).
	//    Single-step Up/Down keep FAR's per-tap semantics: only the
	//    starting row is toggled; the cursor moves to the next row
	//    but that row is left alone (the next tap decides what to
	//    do with it). Range-paint is reserved for multi-step jumps
	//    (Home/End/PgUp/PgDn/Left/Right in grid mode).
	fp.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: vtinput.ShiftPressed,
	})

	if !fp.entries[2].Selected {
		t.Error("file2.txt (index 2) should be selected after Shift+Down")
	}
	if fp.entries[3].Selected {
		t.Error("file3.txt (index 3) must NOT be selected — Shift+Down is single-step, next row is left alone")
	}
	if fp.GetCursorIndex() != 3 {
		t.Errorf("Cursor should move to 3, got %d", fp.GetCursorIndex())
	}

	// 4. Verify results — file1 (from Ins) + file2 (from Shift+Down).
	names := fp.GetSelectedNames()
	if len(names) != 2 || names[0] != "file1.txt" || names[1] != "file2.txt" {
		t.Errorf("GetSelectedNames returned wrong result: %v", names)
	}
}

// TestFileSystemPanel_ShiftMultiStepSwipe covers the long-jump
// shift keys: they select every row the cursor sweeps over. It's
// additive — starting on ".." works (nothing to toggle, we just
// paint from there onward), and a second swipe over already-
// selected rows grows the selection instead of flipping it off.
func TestFileSystemPanel_ShiftMultiStepSwipe(t *testing.T) {
	tmp := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		os.WriteFile(filepath.Join(tmp, n), []byte(n), 0644)
	}
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(tmp))
	fp.viewMode = ViewModeDetailed
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "a"}},
		{VFSItem: vfs.VFSItem{Name: "b"}},
		{VFSItem: vfs.VFSItem{Name: "c"}},
		{VFSItem: vfs.VFSItem{Name: "d"}},
		{VFSItem: vfs.VFSItem{Name: "e"}},
	}
	fp.Refresh()

	// Cursor on ".." (idx 0). Shift+End should still paint a..e —
	// starting on an unselectable row is not a reason to skip the
	// sweep. This exact scenario used to return zero selections.
	fp.SetCursorIndex(0)
	fp.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_END,
		ControlKeyState: vtinput.ShiftPressed,
	})
	for _, want := range []string{"a", "b", "c", "d", "e"} {
		for _, e := range fp.entries {
			if e.Name == want && !e.Selected {
				t.Errorf(`Shift+End starting on "..": expected %q selected`, want)
			}
		}
	}

	// Repeating the sweep from "e" back with Shift+Home must NOT
	// deselect anything — swipes are additive. Everything from ".."
	// (skipped) through "a" is already selected, and the return
	// trip leaves the selection intact.
	fp.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_HOME,
		ControlKeyState: vtinput.ShiftPressed,
	})
	for _, want := range []string{"a", "b", "c", "d", "e"} {
		for _, e := range fp.entries {
			if e.Name == want && !e.Selected {
				t.Errorf("Shift+Home return sweep must not deselect %q", want)
			}
		}
	}
}

// TestFileSystemPanel_ShiftSessionModeDecidedOnFirstKey covers
// FAR-style session semantic: the first Shift+nav decides "select"
// or "deselect" based on the starting row, every subsequent Shift+
// nav in the same session applies that same mode, and any non-
// Shift-nav event closes the session so the next Shift+nav re-
// decides.
func TestFileSystemPanel_ShiftSessionModeDecidedOnFirstKey(t *testing.T) {
	tmp := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		os.WriteFile(filepath.Join(tmp, n), []byte(n), 0644)
	}
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(tmp))
	fp.viewMode = ViewModeDetailed
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "a"}},
		{VFSItem: vfs.VFSItem{Name: "b"}},
		{VFSItem: vfs.VFSItem{Name: "c"}},
		{VFSItem: vfs.VFSItem{Name: "d"}},
		{VFSItem: vfs.VFSItem{Name: "e"}},
	}
	fp.Refresh()

	shiftDown := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.ShiftPressed,
	}
	plainDown := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_DOWN,
	}

	// Session 1: start on unselected "a" (idx 1) → session picks
	// "select" mode. Two Shift+Down keystrokes should select "a"
	// and "b" one after another.
	fp.SetCursorIndex(1)
	fp.ProcessKey(shiftDown)
	fp.ProcessKey(shiftDown)
	if !fp.entries[1].Selected || !fp.entries[2].Selected {
		t.Errorf("session-1 select: a=%v b=%v, want both true",
			fp.entries[1].Selected, fp.entries[2].Selected)
	}
	if fp.entries[3].Selected {
		t.Error("session-1 select: 'c' must not be selected yet — cursor stopped at 'c'")
	}

	// Plain Down closes the session. Cursor moves from c to d
	// without selecting anything.
	fp.ProcessKey(plainDown)
	if fp.shiftSessionActive {
		t.Error("plain Down must close the shift-selection session")
	}

	// Session 2: cursor now on "d". Move it back to "b" (still
	// selected from session 1) to prove the mode re-decides.
	fp.SetCursorIndex(2)
	fp.ProcessKey(shiftDown)
	// b was selected → session mode = deselect. b should be
	// unselected now, cursor moved to c.
	if fp.entries[2].Selected {
		t.Error("session-2 first Shift+Down on selected 'b' should deselect it")
	}
	// Another Shift+Down: c was selected? no, c wasn't selected
	// after session 1. deselect on an already-unselected row is
	// a no-op — still unselected.
	fp.ProcessKey(shiftDown)
	if fp.entries[3].Selected {
		t.Error("session-2 mode is deselect; 'c' must stay unselected")
	}
}

// TestFileSystemPanel_ShiftSessionDeselectFromParentDir covers the
// asymmetric case: cursor on ".." (unselectable) and everything
// below already selected. Session mode has to look past ".." at
// the first real row in the direction of movement; otherwise
// Shift+End would always start in "select" mode and there'd be
// no way to clear the panel selection from that position.
func TestFileSystemPanel_ShiftSessionDeselectFromParentDir(t *testing.T) {
	tmp := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(tmp, n), []byte(n), 0644)
	}
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(tmp))
	fp.viewMode = ViewModeDetailed
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "a"}},
		{VFSItem: vfs.VFSItem{Name: "b"}},
		{VFSItem: vfs.VFSItem{Name: "c"}},
	}
	// Pre-select everything by hand — this is the state the user
	// would land in after a first Shift+End sweep.
	fp.entries[1].Selected = true
	fp.entries[2].Selected = true
	fp.entries[3].Selected = true
	fp.selectedItems = map[string]bool{"a": true, "b": true, "c": true}
	fp.Refresh()

	// Cursor on ".." (idx 0). Shift+End should look past ".." to
	// "a" (selected) and pick "deselect" mode, then clear a..c.
	fp.SetCursorIndex(0)
	fp.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_END,
		ControlKeyState: vtinput.ShiftPressed,
	})
	for _, want := range []string{"a", "b", "c"} {
		for _, e := range fp.entries {
			if e.Name == want && e.Selected {
				t.Errorf("Shift+End from '..': expected %q deselected", want)
			}
		}
	}
}

// TestFileSystemPanel_ShiftRangeSkipsParentDir makes sure the ".."
// row is never selected by a shift-sweep, matching the same rule
// SetItemSelected / ToggleSelection already enforce for Ins.
func TestFileSystemPanel_ShiftRangeSkipsParentDir(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a"), []byte("a"), 0644)
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(tmp))
	fp.viewMode = ViewModeDetailed
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "a"}},
	}
	fp.Refresh()

	// Cursor on "a"; Shift+Home should not select ".."
	fp.SetCursorIndex(1)
	fp.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_HOME,
		ControlKeyState: vtinput.ShiftPressed,
	})
	if fp.entries[0].Selected {
		t.Error(`Shift-sweep must not select the ".." parent-dir row`)
	}
}

// TestFileSystemPanel_SelectionClearedOnDirChange guards against
// the map[filename]→selected persisting between directories: a
// row with the same name in a sibling dir used to inherit the
// old selection. readDirectoryEx now drops the map when the path
// changes.
func TestFileSystemPanel_SelectionClearedOnDirChange(t *testing.T) {
	// Two sibling directories, each with a file named "same".
	parent := t.TempDir()
	a := filepath.Join(parent, "a")
	b := filepath.Join(parent, "b")
	os.MkdirAll(a, 0755)
	os.MkdirAll(b, 0755)
	os.WriteFile(filepath.Join(a, "same"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(b, "same"), []byte("b"), 0644)

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(a))
	fp.viewMode = ViewModeDetailed
	// Simulate a completed load of directory `a` with "same" selected.
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "same"}},
	}
	fp.selectedItems = map[string]bool{"same": true}
	fp.entries[1].Selected = true
	fp.lastLoadedPath = a

	// Now navigate to sibling `b`. readDirectoryEx should notice
	// the path changed and drop the persistent selection so the
	// "same" file in `b` starts unselected.
	fp.vfs.SetPath(b)
	fp.ReadDirectory()

	// Drain any async loader tasks the ReadDirectory scheduled.
	timeout := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			break drain
		}
	}

	for _, e := range fp.entries {
		if e.Name == "same" && e.Selected {
			t.Error(`"same" in sibling directory should NOT inherit selection from previous dir`)
		}
	}
	if fp.selectedItems["same"] {
		t.Error("selectedItems should have been cleared on directory change")
	}
}

func TestFileSystemPanel_ProcessMouse(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.SetViewMode(ViewModeDetailed)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "f1"}},
		{VFSItem: vfs.VFSItem{Name: "f2"}},
	}
	fp.Refresh()

	// Left Click on f1 (Index 1). Table at Y=1, header is 1, so row 0 is Y=2, row 1 is Y=3.
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 5, MouseY: 3, ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	if fp.GetCursorIndex() != 1 {
		t.Errorf("Expected cursorIdx 1, got %d", fp.GetCursorIndex())
	}

	// Right click on f2 (Index 2). Data row 2 is Y=4.
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 5, MouseY: 4, ButtonState: vtinput.RightmostButtonPressed,
	})

	if fp.GetCursorIndex() != 2 {
		t.Errorf("Expected cursorIdx 2, got %d", fp.GetCursorIndex())
	}
	if !fp.entries[2].Selected {
		t.Error("Right click selection failed")
	}

	// Right click again without button release (dragging simulation) - should NOT unselect
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 6, MouseY: 4, ButtonState: vtinput.RightmostButtonPressed,
	})

	if !fp.entries[2].Selected {
		t.Error("Right click drag shouldn't unselect the same item")
	}

	// Release button
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		MouseX: 6, MouseY: 4, ButtonState: 0,
	})

	// Click again - SHOULD unselect
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 6, MouseY: 4, ButtonState: vtinput.RightmostButtonPressed,
	})

	if fp.entries[2].Selected {
		t.Error("New right click should toggle selection")
	}
}

func newPanelScrollTestFixture(mode ViewMode, entryCount int) *FileSystemPanel {
	table := vtui.NewTable(1, 1, 38, 8, nil)
	fp := &FileSystemPanel{
		table:    table,
		viewMode: mode,
		entries:  make([]*fileEntry, entryCount),
	}
	if mode == ViewModeWide {
		fp.wide = true
		fp.viewMode = ViewModeMedium
	}
	for idx := range fp.entries {
		fp.entries[idx] = &fileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("f%d", idx)}}
	}
	fp.ScreenObject.SetPosition(0, 0, 39, 11)
	fp.initScrollBar()
	return fp
}

func TestFileSystemPanel_DragAutoScrollContinuesAndStops(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	for _, mode := range []ViewMode{ViewModeBrief, ViewModeMedium, ViewModeDetailed, ViewModeWide} {
		fp := newPanelScrollTestFixture(mode, 50)
		lastVisible := fp.table.ViewHeight*fp.gridColumnCount() - 1
		fp.SetCursorIndex(lastVisible)
		fp.rowDragButton = vtinput.FromLeft1stButtonPressed

		if !fp.updateDragAutoScroll(fp.Y2 + 1) {
			t.Fatalf("mode %v: movement below the panel did not start drag auto-scroll", mode)
		}
		if fp.table.TopPos != 1 || fp.GetCursorIndex() != lastVisible+1 {
			t.Fatalf("mode %v: first auto-scroll step = top %d, cursor %d; want 1, %d",
				mode, fp.table.TopPos, fp.GetCursorIndex(), lastVisible+1)
		}
		if fp.dragScrollTimer == nil || fp.dragScrollDirection != 1 {
			t.Fatalf("mode %v: drag auto-scroll timer was not scheduled downward", mode)
		}

		contentY := fp.table.Y1 + fp.table.MarginTop
		if fp.updateDragAutoScroll(contentY) {
			t.Fatalf("mode %v: movement back inside the panel still reported auto-scroll", mode)
		}
		if fp.dragScrollTimer != nil || fp.dragScrollDirection != 0 {
			t.Fatalf("mode %v: drag auto-scroll did not stop after returning inside", mode)
		}
	}
}

func TestFileSystemPanel_RightDragAutoScrollSelectsIntermediateRows(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := newPanelScrollTestFixture(ViewModeDetailed, 30)
	start := fp.table.ViewHeight - 1
	fp.SetCursorIndex(start)
	fp.rightDragActive = true
	fp.rightDragSelect = true
	fp.lastRightClickedIdx = start
	fp.rowDragButton = vtinput.RightmostButtonPressed

	if !fp.dragAutoScrollStep(1) {
		t.Fatal("downward drag auto-scroll did not move")
	}
	want := start + 1
	if fp.GetCursorIndex() != want || !fp.entries[want].Selected {
		t.Fatalf("right drag auto-scroll cursor=%d selected=%v; want cursor %d selected",
			fp.GetCursorIndex(), fp.entries[want].Selected, want)
	}

	if !fp.dragAutoScrollStep(-1) {
		t.Fatal("upward drag auto-scroll did not move")
	}
	if fp.GetCursorIndex() != start || !fp.entries[start].Selected {
		t.Fatalf("upward right drag cursor=%d selected=%v; want cursor %d selected",
			fp.GetCursorIndex(), fp.entries[start].Selected, start)
	}
}

func TestFileSystemPanel_DragAutoScrollStopsOnRelease(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := newPanelScrollTestFixture(ViewModeDetailed, 30)
	fp.SetCursorIndex(fp.table.ViewHeight - 1)
	fp.rowDragButton = vtinput.FromLeft1stButtonPressed
	fp.updateDragAutoScroll(fp.Y2 + 1)

	fp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if fp.rowDragButton != 0 || fp.dragScrollTimer != nil || fp.dragScrollDirection != 0 {
		t.Fatal("mouse release did not cancel drag auto-scroll")
	}
}

func TestFileSystemPanel_ScrollBarMetricsAllViewModes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    ViewMode
		columns int
	}{
		{name: "brief", mode: ViewModeBrief, columns: 3},
		{name: "medium", mode: ViewModeMedium, columns: 2},
		{name: "detailed", mode: ViewModeDetailed, columns: 1},
		{name: "wide", mode: ViewModeWide, columns: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := newPanelScrollTestFixture(tc.mode, 0)
			height := fp.table.ViewHeight
			capacity := height * tc.columns

			fp.entries = make([]*fileEntry, capacity)
			_, visible, maxTop, virtualMax, _ := fp.panelScrollMetrics()
			if visible != capacity || maxTop != 0 || virtualMax != 0 {
				t.Fatalf("fitting panel metrics = visible %d, maxTop %d, virtualMax %d; want %d, 0, 0",
					visible, maxTop, virtualMax, capacity)
			}
			if fp.syncScrollBar() {
				t.Fatal("scrollbar visible while all entries fit")
			}

			fp.entries = make([]*fileEntry, capacity+5)
			_, visible, maxTop, virtualMax, _ = fp.panelScrollMetrics()
			wantVirtualMax := (capacity+5+tc.columns-1)/tc.columns - height
			if visible != capacity || maxTop != 5 || virtualMax != wantVirtualMax {
				t.Fatalf("overflow metrics = visible %d, maxTop %d, virtualMax %d; want %d, 5, %d",
					visible, maxTop, virtualMax, capacity, wantVirtualMax)
			}
			if !fp.syncScrollBar() {
				t.Fatal("scrollbar hidden while entries overflow")
			}
			fp.table.TopPos = maxTop
			_, _, _, virtualMax, virtualValue := fp.panelScrollMetrics()
			if virtualValue != virtualMax {
				t.Fatalf("bottom TopPos maps to virtual value %d, want %d", virtualValue, virtualMax)
			}
		})
	}
}

func TestFileSystemPanel_ScrollBarDrawAndMouse(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	fp := newPanelScrollTestFixture(ViewModeMedium, 0)
	capacity := fp.table.ViewHeight * fp.gridColumnCount()
	fp.entries = make([]*fileEntry, capacity+5)
	for idx := range fp.entries {
		fp.entries[idx] = &fileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("f%d", idx)}}
	}

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 12)
	fp.drawScrollBar(scr)
	if got := scr.GetCell(fp.X2, fp.scrollBar.Y1); got.Char != vtui.ScrollUpArrow {
		t.Fatalf("scrollbar top cell = %q, want %q", rune(got.Char), rune(vtui.ScrollUpArrow))
	} else if got.Attributes != vtui.Palette[ColPanelScrollbar] {
		t.Fatalf("scrollbar attr = %#x, want Panel.Scrollbar %#x", got.Attributes, vtui.Palette[ColPanelScrollbar])
	}

	// The down arrow scrolls one item while keeping the cursor on the same
	// visual slot.
	if !fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: int16(fp.X2), MouseY: int16(fp.scrollBar.Y2),
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}) {
		t.Fatal("scrollbar down arrow was not handled")
	}
	fp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if fp.table.TopPos != 1 || fp.GetCursorIndex() != 1 {
		t.Fatalf("after scrollbar step: top=%d cursor=%d, want 1,1", fp.table.TopPos, fp.GetCursorIndex())
	}

	// Crossing the scrollbar during a row drag must not start a new
	// scrollbar gesture.
	fp.setPanelScrollTop(0)
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: int16(fp.X2), MouseY: int16(fp.scrollBar.Y2),
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
	})
	if fp.scrollMouseActive || fp.table.TopPos != 0 {
		t.Fatalf("row drag started scrollbar: active=%v top=%d", fp.scrollMouseActive, fp.table.TopPos)
	}

	// Dragging the thumb to the bottom reaches the exact item-based maximum,
	// even though the scrollbar itself operates in virtual rows.
	fp.setPanelScrollTop(0)
	fp.syncScrollBar()
	thumbY := fp.scrollBar.Y1 + 1
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: int16(fp.X2), MouseY: int16(thumbY),
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 0, MouseY: int16(fp.scrollBar.Y2 - 1),
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	fp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if fp.scrollMouseActive {
		t.Fatal("scrollbar kept mouse capture after button release")
	}
	_, _, maxTop, _, _ := fp.panelScrollMetrics()
	if fp.table.TopPos != maxTop {
		t.Fatalf("after thumb drag: top=%d, want maxTop=%d", fp.table.TopPos, maxTop)
	}
}

func TestFileSystemPanel_ScrollBarHiddenWhenGridFits(t *testing.T) {
	fp := newPanelScrollTestFixture(ViewModeBrief, 0)
	fp.entries = make([]*fileEntry, fp.table.ViewHeight*fp.gridColumnCount())
	fp.table.TopPos = 4
	fp.cursorIdx = 4
	fp.Refresh()
	if fp.table.TopPos != 0 {
		t.Fatalf("fitting Brief grid kept stale TopPos %d after refresh", fp.table.TopPos)
	}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 12)

	fp.drawScrollBar(scr)
	dataY := fp.table.Y1 + fp.table.MarginTop
	if got := scr.GetCell(fp.X2, dataY).Char; got != 0 {
		t.Fatalf("scrollbar drawn for fitting Brief grid: %q", rune(got))
	}
}

func TestFileSystemPanel_SingleRowColumnsFillPanelWidth(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode ViewMode
	}{
		{name: "detailed", mode: ViewModeDetailed},
		{name: "wide", mode: ViewModeWide},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := newPanelScrollTestFixture(tc.mode, 1)
			fp.frame = vtui.NewBorderedFrame(0, 0, 39, 11, vtui.SingleBox, "")
			fp.Resize(40, 12)

			if got := fp.table.Columns[1].Width; got != 11 {
				t.Fatalf("Size column width = %d, want 11", got)
			}
			usedWidth := len(fp.table.Columns) - 1 // separators
			for _, column := range fp.table.Columns {
				usedWidth += column.Width
			}
			tableWidth := fp.table.X2 - fp.table.X1 + 1
			if usedWidth != tableWidth {
				t.Fatalf("columns use %d cells of %d; trailing gap=%d", usedWidth, tableWidth, tableWidth-usedWidth)
			}
		})
	}
}

func TestFileSystemPanel_CursorColorsColumnSeparators(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	for _, tc := range []struct {
		name    string
		mode    ViewMode
		columns []vtui.TableColumn
	}{
		{
			name: "detailed", mode: ViewModeDetailed,
			columns: []vtui.TableColumn{{Width: 20}, {Width: 12}},
		},
		{
			name: "wide", mode: ViewModeWide,
			columns: []vtui.TableColumn{{Width: 10}, {Width: 8}, {Width: 12}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := newPanelScrollTestFixture(tc.mode, 1)
			fp.entries[0].Selected = true
			fp.table.Columns = tc.columns
			fp.table.ColorTextIdx = ColPanelText
			fp.table.ColorSelectedTextIdx = ColPanelCursor
			fp.table.ColorItemSelectTextIdx = ColPanelSelectedText
			fp.table.ColorItemSelectCursorIdx = ColPanelSelectedCursor
			fp.table.ColorTitleIdx = ColPanelColumnTitle
			fp.table.ColorBoxIdx = ColPanelBox
			fp.Refresh()
			fp.table.SetFocus(true)

			scr := vtui.NewSilentScreenBuf()
			scr.AllocBuf(40, 12)
			fp.table.Show(scr)
			fp.drawCursorSeparators(scr)

			y := fp.table.Y1 + fp.table.MarginTop
			x := fp.table.X1
			for column := 0; column < len(fp.table.Columns)-1; column++ {
				x += fp.table.Columns[column].Width
				cell := scr.GetCell(x, y)
				if cell.Char != '│' {
					t.Fatalf("separator %d char = %q, want │", column, rune(cell.Char))
				}
				boxAttr := vtui.Palette[ColPanelBox]
				cursorAttr := vtui.Palette[ColPanelSelectedCursor]
				if cell.Attributes&vtui.IsFgRGB != boxAttr&vtui.IsFgRGB ||
					vtui.GetRGBFore(cell.Attributes) != vtui.GetRGBFore(boxAttr) {
					t.Fatalf("separator %d foreground = %#x, want Panel.Box foreground %#x",
						column, cell.Attributes, boxAttr)
				}
				if cell.Attributes&vtui.IsBgRGB != cursorAttr&vtui.IsBgRGB ||
					vtui.GetRGBBack(cell.Attributes) != vtui.GetRGBBack(cursorAttr) {
					t.Fatalf("separator %d background = %#x, want selected cursor background %#x",
						column, cell.Attributes, cursorAttr)
				}
				if vtui.GetRGBFore(cell.Attributes) == vtui.GetRGBFore(cursorAttr) {
					t.Fatalf("separator %d inherited selected row foreground", column)
				}
				x++
			}
		})
	}
}

func TestFileSystemPanel_MouseClick_Edges(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: ".."}}}
	fp.SetCursorIndex(0)

	// 1. Click on panel border (Y=0)
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 5, MouseY: 0, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if fp.GetCursorIndex() != 0 {
		t.Errorf("Clicking on border should not change selection. Got %d", fp.GetCursorIndex())
	}

	// 2. Click on table header (Y=1)
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 5, MouseY: 1, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if fp.GetCursorIndex() != 0 {
		t.Errorf("Clicking on header should not change selection. Got %d", fp.GetCursorIndex())
	}
}

func TestFileSystemPanel_RightClick_ResetOnRelease(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.viewMode = ViewModeDetailed
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "f1"}}}

	// 1. Right click once -> Selects
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 5, MouseY: 2, ButtonState: vtinput.RightmostButtonPressed,
	})
	if !fp.entries[0].Selected {
		t.Fatal("Should be selected")
	}

	// 2. Release button -> Resets tracker
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false, MouseX: 5, MouseY: 2, ButtonState: 0,
	})

	// 3. Right click again -> Should toggle (Unselect) even though it's the same index
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 5, MouseY: 2, ButtonState: vtinput.RightmostButtonPressed,
	})
	if fp.entries[0].Selected {
		t.Error("Item should have been unselected after button release and re-click")
	}
}

func TestFileSystemPanel_RightDragAppliesToSkippedRows(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.SetViewMode(ViewModeDetailed)
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "f1"}},
		{VFSItem: vfs.VFSItem{Name: "f2"}},
		{VFSItem: vfs.VFSItem{Name: "f3"}},
		{VFSItem: vfs.VFSItem{Name: "f4"}},
	}
	fp.Refresh()

	dataY := fp.table.Y1 + fp.table.MarginTop
	rightDown := func(idx int) {
		fp.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			MouseX: int16(fp.table.X1), MouseY: int16(dataY + idx),
			ButtonState: vtinput.RightmostButtonPressed,
		})
	}
	release := func(idx int) {
		fp.ProcessMouse(&vtinput.InputEvent{
			Type:   vtinput.MouseEventType,
			MouseX: int16(fp.table.X1), MouseY: int16(dataY + idx),
		})
	}

	// Only the endpoints emit events; f2 and f3 must still be selected.
	rightDown(1)
	rightDown(4)
	for idx := 1; idx <= 4; idx++ {
		if !fp.entries[idx].Selected {
			t.Errorf("entry %d was skipped while selecting", idx)
		}
	}

	release(4)

	// Starting a new drag on a selected item fixes the operation to deselect,
	// including skipped rows while moving in the opposite direction.
	rightDown(4)
	rightDown(1)
	for idx := 1; idx <= 4; idx++ {
		if fp.entries[idx].Selected {
			t.Errorf("entry %d was skipped while deselecting", idx)
		}
	}
}

func TestFileSystemPanel_RightDragTracksGridColumn(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.SetViewMode(ViewModeMedium)

	height := fp.table.ViewHeight
	fp.entries = make([]*fileEntry, height*2)
	for idx := range fp.entries {
		fp.entries[idx] = &fileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("f%d", idx)}}
	}
	fp.Refresh()

	startIdx := height - 2
	endIdx := height + 1
	dataY := fp.table.Y1 + fp.table.MarginTop
	leftX := fp.table.X1
	rightX := fp.table.X1 + fp.table.Columns[0].Width + 1

	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: int16(leftX), MouseY: int16(dataY + height - 2),
		ButtonState: vtinput.RightmostButtonPressed,
	})
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: int16(rightX), MouseY: int16(dataY + 1),
		ButtonState: vtinput.RightmostButtonPressed,
	})

	if fp.GetCursorIndex() != endIdx {
		t.Fatalf("cursor index = %d, want %d", fp.GetCursorIndex(), endIdx)
	}
	for idx := startIdx; idx <= endIdx; idx++ {
		if !fp.entries[idx].Selected {
			t.Errorf("grid entry %d was skipped", idx)
		}
	}
}

func TestFileSystemPanel_RightDoubleClickAppliesToWholePanel(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))
	fp.SetViewMode(ViewModeDetailed)
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "f1"}},
		{VFSItem: vfs.VFSItem{Name: "f2"}},
		{VFSItem: vfs.VFSItem{Name: "f3"}},
	}
	fp.Refresh()

	dataY := fp.table.Y1 + fp.table.MarginTop
	rightClick := func(idx int, flags uint32) {
		fp.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			MouseX: int16(fp.table.X1), MouseY: int16(dataY + idx),
			ButtonState: vtinput.RightmostButtonPressed, MouseEventFlags: flags,
		})
	}
	release := func(idx int) {
		fp.ProcessMouse(&vtinput.InputEvent{
			Type:   vtinput.MouseEventType,
			MouseX: int16(fp.table.X1), MouseY: int16(dataY + idx),
		})
	}

	// The first press selects f1; the second press spreads that state to all.
	rightClick(1, 0)
	release(1)
	rightClick(1, vtinput.DoubleClick)
	for idx := 1; idx < len(fp.entries); idx++ {
		if !fp.entries[idx].Selected {
			t.Errorf("entry %d was not selected by right double-click", idx)
		}
	}
	if fp.entries[0].Selected {
		t.Error("parent directory entry must not be selected")
	}

	release(1)

	// Starting on selected f2 makes the first press deselect it; the second
	// press spreads deselection to the whole panel.
	rightClick(2, 0)
	release(2)
	rightClick(2, vtinput.DoubleClick)
	for idx := 1; idx < len(fp.entries); idx++ {
		if fp.entries[idx].Selected {
			t.Errorf("entry %d was not deselected by right double-click", idx)
		}
	}
}

func TestFileSystemPanel_IncrementalInteraction(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))

	// Ensure we have '..' as initial state
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}

	// Симулируем прилет первого чанка
	chunk1 := []vfs.VFSItem{
		{Name: "file_A", IsDir: false},
		{Name: "file_Z", IsDir: false},
	}

	// Вручную вызываем логику обработки чанка (имитируя прилет из горутины)
	fp.entries = append(fp.entries, &fileEntry{VFSItem: chunk1[0]}, &fileEntry{VFSItem: chunk1[1]})
	fp.Refresh()

	// Пользователь выбирает file_Z (это индекс 2, так как 0 это "..")
	fp.SelectName("file_Z")
	if fp.GetSelectedName() != "file_Z" {
		t.Fatalf("Failed to select file_Z, got %s", fp.GetSelectedName())
	}

	// Симулируем прилет второго чанка с файлом, который встанет В НАЧАЛО списка после сортировки
	chunk2 := []vfs.VFSItem{
		{Name: "file_0_first", IsDir: false},
	}

	// Эмуляция PostTask для второго чанка:
	currentSelected := fp.GetSelectedName() // "file_Z"
	fp.entries = append(fp.entries, &fileEntry{VFSItem: chunk2[0]})
	sort.Slice(fp.entries, func(i, j int) bool {
		if fp.entries[i].Name == ".." {
			return true
		}
		if fp.entries[j].Name == ".." {
			return false
		}
		return fp.entries[i].Name < fp.entries[j].Name
	})
	fp.Refresh()
	fp.SelectName(currentSelected) // Удерживаем курсор

	// Проверяем: file_Z теперь должен быть на индексе 3, но курсор должен быть все еще на нем
	if fp.GetSelectedName() != "file_Z" {
		t.Errorf("Cursor jumped! Expected 'file_Z', got '%s'", fp.GetSelectedName())
	}

	// Проверяем, что индекс реально изменился (был 2, стал 3)
	if fp.GetCursorIndex() != 3 {
		t.Errorf("Index should have shifted to 3, got %d", fp.GetCursorIndex())
	}
}
func TestFileSystemPanel_GetSuccessorName(t *testing.T) {
	fp := &FileSystemPanel{}

	setupEntries := func(names ...string) {
		fp.cursorIdx = 0 // Reset state between cases
		fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
		for _, n := range names {
			fp.entries = append(fp.entries, &fileEntry{VFSItem: vfs.VFSItem{Name: n}})
		}
	}

	// Case 1: Single item in the middle. Focus on B. Successor should be C.
	setupEntries("A", "B", "C")
	fp.cursorIdx = 2 // B (Index 0 is .., 1 is A, 2 is B)
	if res := fp.GetSuccessorName(); res != "C" {
		t.Errorf("Case 1 failed: expected 'C', got %q", res)
	}

	// Case 2: Single item at the end. Focus on C. Successor should be B.
	fp.cursorIdx = 3 // C
	if res := fp.GetSuccessorName(); res != "B" {
		t.Errorf("Case 2 failed: expected 'B', got %q", res)
	}

	// Case 3: Multiple selected in the middle. Select A, B. Successor should be C.
	setupEntries("A", "B", "C", "D")
	fp.entries[1].Selected = true // A
	fp.entries[2].Selected = true // B
	if res := fp.GetSuccessorName(); res != "C" {
		t.Errorf("Case 3 failed: expected 'C', got %q", res)
	}

	// Case 4: Multiple selected at the end. Select C, D. Successor should be B.
	setupEntries("A", "B", "C", "D")
	fp.entries[3].Selected = true // C
	fp.entries[4].Selected = true // D
	if res := fp.GetSuccessorName(); res != "B" {
		t.Errorf("Case 4 failed: expected 'B', got %q", res)
	}

	// Case 5: Empty list (only .. exists)
	setupEntries()
	if res := fp.GetSuccessorName(); res != ".." {
		t.Errorf("Case 5 failed: expected '..', got %q", res)
	}
}
func TestGetSelectedNames_ParentSafety(t *testing.T) {
	fp := &FileSystemPanel{}
	// Setup entries: 0: "..", 1: "file.txt"
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file.txt", IsDir: false}},
	}

	// Case 1: Cursor on ".."
	fp.cursorIdx = 0
	names := fp.GetSelectedNames()
	if len(names) != 0 {
		t.Errorf("Security violation: GetSelectedNames returned items when cursor was on '..': %v", names)
	}

	// Case 2: Cursor on "file.txt"
	fp.cursorIdx = 1
	names = fp.GetSelectedNames()
	if len(names) != 1 || names[0] != "file.txt" {
		t.Errorf("Failed to get item under cursor: %v", names)
	}

	// Case 3: "file.txt" selected, but cursor is on ".."
	fp.entries[1].Selected = true
	fp.cursorIdx = 0
	names = fp.GetSelectedNames()
	if len(names) != 1 || names[0] != "file.txt" {
		t.Errorf("Failed to get selected items while cursor is on '..': %v", names)
	}
}
func TestFileSystemPanel_AsyncPendingSelection(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS("."))

	// Target: we want to select "target.txt" which will arrive in the second chunk
	fp.pendingSelection = "target.txt"
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fp.cursorIdx = 0

	// 1. Simulate First Chunk (doesn't contain our target)
	chunk1 := []vfs.VFSItem{{Name: "aaa.txt"}, {Name: "bbb.txt"}}

	// Replicating the logic from ReadDirectory's onChunk callback
	newEntries := make([]*fileEntry, len(chunk1))
	for i, item := range chunk1 {
		newEntries[i] = &fileEntry{VFSItem: item}
	}

	fp.entries = append(fp.entries, newEntries...)
	sort.Slice(fp.entries, func(i, j int) bool { return fp.entries[i].Name < fp.entries[j].Name })

	// Run snapping logic (simplified from file_panel.go)
	for i, entry := range fp.entries {
		if entry.Name == fp.pendingSelection {
			fp.SetCursorIndex(i)
			fp.pendingSelection = ""
			break
		}
	}

	if fp.pendingSelection == "" || fp.GetSelectedName() == "target.txt" {
		t.Error("Snapped prematurely to non-existent item")
	}

	// 2. Simulate Second Chunk (contains our target)
	chunk2 := []vfs.VFSItem{{Name: "target.txt"}, {Name: "zzz.txt"}}
	newEntries2 := make([]*fileEntry, len(chunk2))
	for i, item := range chunk2 {
		newEntries2[i] = &fileEntry{VFSItem: item}
	}

	fp.entries = append(fp.entries, newEntries2...)
	sort.Slice(fp.entries, func(i, j int) bool { return fp.entries[i].Name < fp.entries[j].Name })

	// Run snapping logic again
	for i, entry := range fp.entries {
		if entry.Name == fp.pendingSelection {
			fp.SetCursorIndex(i)
			fp.pendingSelection = ""
			break
		}
	}

	if fp.pendingSelection != "" {
		t.Error("Failed to clear pendingSelection after item arrived")
	}
	if fp.GetSelectedName() != "target.txt" {
		t.Errorf("Cursor failed to snap to 'target.txt'. Currently on: %q", fp.GetSelectedName())
	}
}
func TestFileSystemPanel_NavigateDown_CursorReset(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "subdir")
	os.Mkdir(sub, 0755)

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(tmp))

	// Mock that we are standing on "subdir" (index 1, as index 0 is "..")
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "subdir", IsDir: true}},
	}
	fp.cursorIdx = 1
	fp.Refresh()

	// 1. Press Enter on "subdir"
	fp.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if fp.pendingSelection != ".." {
		t.Errorf("Expected pendingSelection to be '..', got %q", fp.pendingSelection)
	}

	// 2. Simulate data arrival for the new directory
	// Even if the new directory has a file with the same name as the old one,
	// we must stay on ".."
	chunk := []vfs.VFSItem{{Name: "subdir"}} // coincidentally same name
	newEntries := []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}, {VFSItem: chunk[0]}}
	fp.entries = newEntries

	// Run snapping logic from ReadDirectory
	for i, entry := range fp.entries {
		if entry.Name == fp.pendingSelection {
			fp.SetCursorIndex(i)
			fp.pendingSelection = ""
			break
		}
	}

	if fp.GetCursorIndex() != 0 {
		t.Errorf("Cursor did not reset to '..'. Index is %d", fp.GetCursorIndex())
	}
}
func TestFileSystemPanel_FastFind(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "apple"}},
		{VFSItem: vfs.VFSItem{Name: "banana"}},
		{VFSItem: vfs.VFSItem{Name: "cherry"}},
		{VFSItem: vfs.VFSItem{Name: "cat"}},
	}
	fp.Refresh()

	// 1. Trigger FastFind with Alt+C
	fp.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'c',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !fp.fastFindMode {
		t.Fatal("FastFind mode should be active")
	}
	if fp.fastFindStr != "c" {
		t.Errorf("Expected search string 'c', got %q", fp.fastFindStr)
	}
	if fp.GetSelectedName() != "cherry" {
		t.Errorf("Cursor should jump to 'cherry', got %q", fp.GetSelectedName())
	}

	// 2. Append 'a'
	fp.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    'a',
	})

	if fp.fastFindStr != "ca" {
		t.Errorf("Expected search string 'ca', got %q", fp.fastFindStr)
	}
	if fp.GetSelectedName() != "cat" {
		t.Errorf("Cursor should jump to 'cat', got %q", fp.GetSelectedName())
	}

	// 3. Backspace
	fp.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_BACK,
	})
	if fp.fastFindStr != "c" {
		t.Errorf("Expected search string 'c' after backspace, got %q", fp.fastFindStr)
	}
	if fp.GetSelectedName() != "cherry" {
		t.Errorf("Cursor should jump back to 'cherry', got %q", fp.GetSelectedName())
	}

	// 4. Down arrow (next match)
	fp.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_DOWN,
	})
	if fp.GetSelectedName() != "cat" {
		t.Errorf("Down arrow should jump to 'cat', got %q", fp.GetSelectedName())
	}

	// 5. Up arrow (prev match)
	fp.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_UP,
	})
	if fp.GetSelectedName() != "cherry" {
		t.Errorf("Up arrow should jump back to 'cherry', got %q", fp.GetSelectedName())
	}

	// 6. Escape to cancel
	fp.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	})
	if fp.fastFindMode {
		t.Error("Escape should exit FastFind mode")
	}

	// 7. Navigation keys should deactivate FastFind
	fp.fastFindMode = true
	fp.fastFindStr = "c"
	fp.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_LEFT,
	})
	if fp.fastFindMode {
		t.Error("Navigation key (Left) should deactivate FastFind mode")
	}
}
func TestFileSystemPanel_FastFind_Rendering(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	fp.fastFindMode = true
	fp.fastFindStr = "test"

	// Отрисовываем
	fp.Show(scr)

	// Проверяем наличие рамки и текста в буфере.
	// Окно поиска рисуется снизу панели (Y2-2 = 17, 18, 19).
	// Проверяем заголовок поиска (по умолчанию " Search " из Viewer.SearchTitle)
	foundTitle := false
	for x := 0; x < 80; x++ {
		if scr.GetCell(x, 17).Char == 'S' && scr.GetCell(x+1, 17).Char == 'e' {
			foundTitle = true
			break
		}
	}
	if !foundTitle {
		t.Error("FastFind window title not found in ScreenBuf")
	}

	// Проверяем набранный текст "test" на строке ввода (Y=18)
	foundText := false
	for x := 0; x < 80; x++ {
		if scr.GetCell(x, 18).Char == 't' && scr.GetCell(x+1, 18).Char == 'e' && scr.GetCell(x+2, 18).Char == 's' {
			foundText = true
			break
		}
	}
	if !foundText {
		t.Error("FastFind search string 'test' not found in ScreenBuf")
	}
}

func TestFileSystemPanel_FastFind_LongString(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	fp.fastFindMode = true
	// Строка длиной 26 символов. Окно вмещает 20.
	// Ожидаемый результат после обрезки слева: "D_chars_to_scroll_TAIL"
	fp.fastFindStr = "HEAD_chars_to_scroll_TAIL"

	fp.Show(scr)

	// Окно FastFind рисуется с X=9, текст начинается с X=11.
	fieldX1, fieldX2 := 11, 31

	// Проверяем наличие хвоста "TAIL"
	foundTail := false
	for x := fieldX1; x < fieldX2-3; x++ {
		if scr.GetCell(x, 18).Char == 'T' && scr.GetCell(x+1, 18).Char == 'A' &&
			scr.GetCell(x+2, 18).Char == 'I' && scr.GetCell(x+3, 18).Char == 'L' {
			foundTail = true
			break
		}
	}
	if !foundTail {
		t.Error("Long search string tail 'TAIL' not rendered correctly")
	}

	// Проверяем отсутствие головы "HEAD" (она должна была уйти за левый край)
	foundHead := false
	for x := fieldX1; x < fieldX2-3; x++ {
		if scr.GetCell(x, 18).Char == 'H' && scr.GetCell(x+1, 18).Char == 'E' &&
			scr.GetCell(x+2, 18).Char == 'A' && scr.GetCell(x+3, 18).Char == 'D' {
			foundHead = true
			break
		}
	}
	if foundHead {
		t.Error("Long search string head 'HEAD' should be scrolled out of view")
	}
}
func TestFileSystemPanel_FastFind_XLat(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "readme.txt"}},
		{VFSItem: vfs.VFSItem{Name: "заметка.txt"}},
	}
	fp.Refresh()

	// 1. Поиск "readme" через ввод "кефдьу" (в русской раскладке)
	vtui.GlobalXlator.Track('ф') // Включаем русский контекст
	fp.fastFindMode = true
	fp.fastFindStr = "кефд" // "read"
	fp.doFastFind(0)

	if fp.GetSelectedName() != "readme.txt" {
		t.Errorf("XLat FastFind failed: expected 'readme.txt', got %q", fp.GetSelectedName())
	}

	// 2. Поиск "заметка" через ввод "pfvt" (в английской раскладке)
	vtui.GlobalXlator.Track('a') // Включаем английский контекст
	fp.fastFindStr = "pfvt"      // 'p'->'з', 'f'->'а', 'v'->'м', 't'->'е'
	// Сбросим индекс, чтобы гарантированно найти файл с начала списка
	fp.SetCursorIndex(0)
	fp.doFastFind(0)

	if fp.GetSelectedName() != "заметка.txt" {
		t.Errorf("XLat FastFind (reverse) failed: expected 'заметка.txt', got %q", fp.GetSelectedName())
	}
}

func TestFileSystemPanel_ForkDuplication(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "file1.txt"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(tmp, "file2.txt"), []byte("data"), 0644)

	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))

	// Wait for initial load
	timeout := time.After(1 * time.Second)
	for fp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("timeout")
		}
	}
	// Drain remaining tasks
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			goto done1
		}
	}
done1:

	initialCount := len(fp.entries)
	if initialCount != 3 { // "..", "file1.txt", "file2.txt"
		t.Fatalf("Expected 3 entries initially, got %d", initialCount)
	}

	// Simulate what Clone() does
	cloneFsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	// Copy entries like Clone()
	cloneFsp.entries = make([]*fileEntry, len(fp.entries))
	for j, e := range fp.entries {
		cloneFsp.entries[j] = &fileEntry{VFSItem: e.VFSItem, Selected: e.Selected}
	}
	cloneFsp.Refresh()

	// Call readDirectoryEx(true) like Clone()
	cloneFsp.readDirectoryEx(true)

	// Wait for clone load
	timeout = time.After(1 * time.Second)
	for cloneFsp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("timeout")
		}
	}
	// Drain remaining tasks
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			goto done2
		}
	}
done2:

	if len(cloneFsp.entries) != initialCount {
		t.Errorf("Duplication bug! Expected %d entries, got %d", initialCount, len(cloneFsp.entries))
	}
}
func TestFileSystemPanel_FastFind_MouseDeactivation(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	fp.fastFindMode = true

	// Клик мышкой (любой кнопкой) должен выключать поиск
	fp.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      5, MouseY: 5,
	})

	if fp.fastFindMode {
		t.Error("Mouse click should deactivate FastFind mode")
	}
}
func TestFileSystemPanel_DirectoryCache(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	v := vfs.NewOSVFS(t.TempDir())
	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	// 1. Manually populate cache
	items := []vfs.VFSItem{
		{Name: "cached_file.txt", IsDir: false},
	}
	fp.saveToCache(v.GetPath(), items)

	// 2. Call readDirectoryEx and intercept before goroutine returns
	fp.readDirectoryEx(false)

	// At this exact moment, UI should have the cached entries!
	if len(fp.entries) < 2 { // ".." + "cached_file.txt"
		t.Fatalf("Cache not applied immediately, entries len: %d", len(fp.entries))
	}

	found := false
	for _, e := range fp.entries {
		if e.Name == "cached_file.txt" {
			found = true
			if !e.IsCached {
				t.Error("Cached entry IsCached flag not set")
			}
		}
	}
	if !found {
		t.Error("Cached file not found in panel entries")
	}
}

func TestFileSystemPanel_CacheEviction(t *testing.T) {
	fp := &FileSystemPanel{}
	for i := 0; i < 60; i++ {
		fp.saveToCache(fmt.Sprintf("/path/%d", i), nil)
		time.Sleep(1 * time.Millisecond) // Ensure time diff
	}

	if len(fp.dirCache) > maxDirCache {
		t.Errorf("Cache exceeded max size: %d", len(fp.dirCache))
	}

	// The first inserted path "/path/0" should be evicted
	if _, ok := fp.dirCache["/path/0"]; ok {
		t.Error("Oldest entry was not evicted")
	}
}

func TestFileSystemPanel_MaskSelection(t *testing.T) {
	// Initialize with a dummy table to avoid Refresh() nil pointer panic
	fp := &FileSystemPanel{
		table: vtui.NewTable(0, 0, 10, 10, nil),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: ".."}},
			{VFSItem: vfs.VFSItem{Name: "readme.txt"}},
			{VFSItem: vfs.VFSItem{Name: "source.go"}},
			{VFSItem: vfs.VFSItem{Name: "config.json"}},
			{VFSItem: vfs.VFSItem{Name: "main.go"}},
		},
	}

	// 1. Select by mask *.go
	fp.ApplyMaskSelection("*.go", true)
	if !fp.entries[2].Selected || !fp.entries[4].Selected {
		t.Error("Mask selection failed for *.go")
	}
	if fp.entries[1].Selected || fp.entries[3].Selected {
		t.Error("Mask selection selected wrong files")
	}
	if fp.entries[0].Selected {
		t.Error("Mask selection should never select '..'")
	}

	// 2. Invert selection
	fp.InvertSelection()
	if fp.entries[2].Selected || fp.entries[4].Selected {
		t.Error("Inversion failed: .go files should be unselected")
	}
	if !fp.entries[1].Selected || !fp.entries[3].Selected {
		t.Error("Inversion failed: other files should be selected")
	}

	// 3. Deselect by mask
	fp.ApplyMaskSelection("*.json", false)
	if fp.entries[3].Selected {
		t.Error("Deselection failed for *.json")
	}
	if !fp.entries[1].Selected {
		t.Error("Deselection removed wrong files")
	}
}

func TestFileSystemPanel_TitleDoesNotContainSortIndicator(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	v := vfs.NewOSVFS(t.TempDir())
	fp := NewFileSystemPanel(0, 0, 40, 24, v)
	fp.currentTitle = "C:\\work"

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.SetDefaultPalette()

	fp.sortMode = SortExt
	fp.sortReverse = true
	fp.Show(scr)

	if got := rune(scr.GetCell(2, 0).Char); got != ' ' {
		t.Fatalf("title decoration starts with %q; want a space", got)
	}
	if got := rune(scr.GetCell(3, 0).Char); got != 'C' {
		t.Fatalf("first title character = %q; sort indicator still occupies the title", got)
	}
	if got := rune(scr.GetCell(10, 0).Char); got != ' ' {
		t.Fatalf("title decoration ends with %q; want a space", got)
	}

	fp.sortMode = SortSize
	fp.sortReverse = false
	fp.Show(scr)
	if got := rune(scr.GetCell(3, 0).Char); got != 'C' {
		t.Fatalf("changing sort mode changed the path title prefix to %q", got)
	}
}
func TestFileSystemPanel_FastFind_Visibility(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	// Создаем много файлов, чтобы список мог скроллиться
	v := vfs.NewOSVFS(t.TempDir())
	fp := NewFileSystemPanel(0, 0, 40, 10, v)
	fp.viewMode = ViewModeDetailed

	for i := 0; i < 20; i++ {
		fp.entries = append(fp.entries, &fileEntry{
			VFSItem: vfs.VFSItem{Name: fmt.Sprintf("file_%02d", i)},
		})
	}
	fp.Refresh()

	// Включаем поиск
	fp.fastFindMode = true
	// Ищем файл, который находится в самом низу текущего экрана (Row 8-9)
	// Окно поиска перекрывает нижние 2-3 строки.
	fp.fastFindStr = "file_08"
	fp.doFastFind(0)

	// Проверяем, что панель отскроллилась вверх, чтобы "file_08" не был
	// за окном поиска (в последних двух строках ViewHeight)
	H := fp.table.ViewHeight // Должно быть 8 (10 минус рамки)
	relRow := fp.cursorIdx - fp.table.TopPos

	if relRow >= H-2 {
		t.Errorf("Matched item is too low and obscured by search box. RelRow: %d, H: %d", relRow, H)
	}
}

func TestFileSystemPanel_Sorting(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	v := vfs.NewOSVFS(t.TempDir())
	fp := NewFileSystemPanel(0, 0, 80, 24, v)

	t1 := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "beta.txt", Size: 100, MTime: t1}},
		{VFSItem: vfs.VFSItem{Name: "alpha.exe", Size: 50, MTime: t2}},
		{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true}},
	}

	// 1. Sort by Name
	fp.sortMode = SortName
	fp.sortReverse = false
	fp.sortEntries()
	// Expected: .., folder, alpha.exe, beta.txt
	if fp.entries[1].Name != "folder" || fp.entries[2].Name != "alpha.exe" {
		t.Errorf("SortName failed: index 1=%s, index 2=%s", fp.entries[1].Name, fp.entries[2].Name)
	}

	// 2. Sort by Size
	fp.sortMode = SortSize
	fp.sortReverse = false // Descending (large first)
	fp.sortEntries()
	// Expected: .., folder, beta.txt (100), alpha.exe (50)
	if fp.entries[2].Name != "beta.txt" {
		t.Errorf("SortSize failed: index 2=%s", fp.entries[2].Name)
	}

	// 3. Sort by Time
	fp.sortMode = SortTime
	fp.sortReverse = false // Descending (newest first)
	fp.sortEntries()
	// Expected: .., folder, alpha.exe (2024), beta.txt (2023)
	if fp.entries[2].Name != "alpha.exe" {
		t.Errorf("SortTime failed: index 2=%s", fp.entries[2].Name)
	}

	// 4. Test logic in SetSortMode
	fp.SetSortMode(SortName) // Should set reverse = false
	if fp.sortReverse {
		t.Error("SetSortMode(Name) should reset reverse to false")
	}

	fp.SetSortMode(SortName) // Toggle reverse
	if !fp.sortReverse {
		t.Error("SetSortMode(Name) second call should toggle reverse to true")
	}
}

func TestFileSystemPanel_SortColumnIndicators(t *testing.T) {
	for _, tc := range []struct {
		name         string
		viewMode     ViewMode
		sortMode     SortMode
		reverse      bool
		column       int
		wantSuffix   string
		rightAligned bool
	}{
		{name: "brief name ascending", viewMode: ViewModeBrief, sortMode: SortName, column: 0, wantSuffix: " ↑"},
		{name: "medium name descending", viewMode: ViewModeMedium, sortMode: SortName, reverse: true, column: 1, wantSuffix: " ↓"},
		{name: "detailed size descending", viewMode: ViewModeDetailed, sortMode: SortSize, column: 1, wantSuffix: " ↓"},
		{name: "detailed size ascending", viewMode: ViewModeDetailed, sortMode: SortSize, reverse: true, column: 1, wantSuffix: " ↑"},
		{name: "wide time descending", viewMode: ViewModeWide, sortMode: SortTime, column: 2, wantSuffix: " ↓"},
		{name: "brief hidden size", viewMode: ViewModeBrief, sortMode: SortSize, column: 0, wantSuffix: "[" + Msg("Menu.SortSize") + "]↓", rightAligned: true},
		{name: "medium hidden time", viewMode: ViewModeMedium, sortMode: SortTime, reverse: true, column: 0, wantSuffix: "[" + Msg("Menu.SortTime") + "]↑", rightAligned: true},
		{name: "detailed hidden extension", viewMode: ViewModeDetailed, sortMode: SortExt, column: 0, wantSuffix: "[" + Msg("Menu.SortExt") + "]↑", rightAligned: true},
		{name: "wide hidden extension reversed", viewMode: ViewModeWide, sortMode: SortExt, reverse: true, column: 0, wantSuffix: "[" + Msg("Menu.SortExt") + "]↓", rightAligned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := newPanelScrollTestFixture(tc.viewMode, 1)
			fp.frame = vtui.NewBorderedFrame(0, 0, 79, 11, vtui.SingleBox, "")
			fp.sortMode = tc.sortMode
			fp.sortReverse = tc.reverse
			fp.Resize(80, 12)
			if got := fp.table.Columns[tc.column].Title; !strings.HasSuffix(got, tc.wantSuffix) {
				t.Fatalf("column title %q does not end in %q", got, tc.wantSuffix)
			} else if tc.rightAligned && runewidth.StringWidth(got) != fp.table.Columns[tc.column].Width {
				t.Fatalf("hidden sort title width = %d, want column width %d: %q",
					runewidth.StringWidth(got), fp.table.Columns[tc.column].Width, got)
			}
		})
	}

	narrow := hiddenSortColumnTitle(SortExt, true, 12)
	if runewidth.StringWidth(narrow) > 12 || !strings.HasSuffix(narrow, "]↑") {
		t.Fatalf("narrow hidden sort title lost brackets/arrow: %q", narrow)
	}
}

func TestFileSystemPanel_HeaderClickSortsAndToggles(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	fp.SetViewMode(ViewModeDetailed)
	fp.sortMode = SortUnsorted
	fp.sortReverse = false
	fp.updateSortColumnTitles()

	clickName := func(flags uint32) bool {
		return fp.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			MouseX: int16(fp.table.X1), MouseY: int16(fp.table.Y1),
			ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: flags,
		})
	}
	release := func() {
		fp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	}

	if !clickName(0) || fp.sortMode != SortName || fp.sortReverse {
		t.Fatalf("first Name header click: mode=%v reverse=%v", fp.sortMode, fp.sortReverse)
	}
	if !fp.headerMouseActive {
		t.Fatal("header click was not marked as a header gesture")
	}
	release()
	if !clickName(vtinput.DoubleClick) || fp.sortMode != SortName || !fp.sortReverse {
		t.Fatalf("second Name header click: mode=%v reverse=%v", fp.sortMode, fp.sortReverse)
	}
	if !strings.HasSuffix(fp.table.Columns[0].Title, " ↓") {
		t.Fatalf("reversed Name title has no down arrow: %q", fp.table.Columns[0].Title)
	}

	// Separator clicks do not select either adjacent sort column.
	release()
	separatorX := fp.table.X1 + fp.table.Columns[0].Width
	if _, ok := fp.headerSortModeAt(separatorX, fp.table.Y1); ok {
		t.Fatal("column separator was treated as a sortable header")
	}
}

func TestFileSystemPanel_HeaderSortMappingAllViewModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode ViewMode
		want []SortMode
	}{
		{name: "brief", mode: ViewModeBrief, want: []SortMode{SortName, SortName, SortName}},
		{name: "medium", mode: ViewModeMedium, want: []SortMode{SortName, SortName}},
		{name: "detailed", mode: ViewModeDetailed, want: []SortMode{SortName, SortSize}},
		{name: "wide", mode: ViewModeWide, want: []SortMode{SortName, SortSize, SortTime}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := newPanelScrollTestFixture(tc.mode, 1)
			fp.frame = vtui.NewBorderedFrame(0, 0, 79, 11, vtui.SingleBox, "")
			fp.Resize(80, 12)
			x := fp.table.X1
			for column, want := range tc.want {
				mode, ok := fp.headerSortModeAt(x, fp.table.Y1)
				if !ok || mode != want {
					t.Fatalf("column %d maps to %v,%v; want %v,true", column, mode, ok, want)
				}
				x += fp.table.Columns[column].Width + 1
			}
		})
	}
}

/*
func TestDummyFailure(t *testing.T) {
    vtui.DebugLog("This is a trace log before failure.")
    t.Fatal("Intentional failure for log dump test")
}
*/
// waitForLoad is a test helper to wait for a panel's async loading to complete.
func waitForLoad(t *testing.T, fp *FileSystemPanel) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for fp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for panel to load")
		}
	}
	// Drain any final UI tasks after isLoading becomes false
	for i := 0; i < 5; i++ {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			return
		}
	}
}
func TestFileSystemPanel_StructLiteralLazySelection(t *testing.T) {
	// Create panel as a struct literal (nil selectedItems map)
	fp := &FileSystemPanel{}
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "test.txt"}}}

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Panel panicked on selection toggle with nil map: %v", r)
		}
	}()

	fp.SetItemSelected(0, true)
	if fp.selectedItems == nil || !fp.selectedItems["test.txt"] {
		t.Error("Lazy initialization failed to record selection")
	}
}

func TestFileSystemPanel_CacheLoadPreservesSelection(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	v := vfs.NewOSVFS(t.TempDir())
	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	// Simulate items loaded and selected
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "keep.txt"}},
	}
	fp.SetItemSelected(1, true)

	// Populate cache
	fp.saveToCache(v.GetPath(), []vfs.VFSItem{{Name: "keep.txt"}})

	// Trigger a reload (which will hit the cache synchronously)
	fp.readDirectoryEx(false)

	// The synchronous cache load should have preserved the selection
	found := false
	for _, e := range fp.entries {
		if e.Name == "keep.txt" {
			found = true
			if !e.Selected {
				t.Error("Synchronous cache load wiped out the selection state")
			}
		}
	}
	if !found {
		t.Error("keep.txt not found in panel after cache reload")
	}
}

func TestFileSystemPanel_SyncPanelLoad(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Save original config and restore after test
	oldSync := AppConfig.SyncPanelLoad
	AppConfig.SyncPanelLoad = true
	defer func() { AppConfig.SyncPanelLoad = oldSync }()

	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)
	os.WriteFile(filepath.Join(tmpDir, "file_sync.txt"), []byte("data"), 0644)

	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	// 1. Manually populate cache with a different item to see if it gets ignored
	items := []vfs.VFSItem{
		{Name: "cached_file.txt", IsDir: false},
	}
	fp.saveToCache(v.GetPath(), items)

	// 2. Call readDirectoryEx. Since SyncPanelLoad is true, it MUST NOT load "cached_file.txt" from cache.
	fp.readDirectoryEx(false)

	// Verify that cache was ignored (fp.entries should not contain "cached_file.txt" immediately)
	for _, e := range fp.entries {
		if e.Name == "cached_file.txt" {
			t.Error("VFS loaded cached entry immediately, but SyncPanelLoad is enabled")
		}
	}

	// 3. Wait for the async load to complete (should load "file_sync.txt" from disk)
	waitForLoad(t, fp)

	foundReal := false
	for _, e := range fp.entries {
		if e.Name == "file_sync.txt" {
			foundReal = true
		}
		if e.Name == "cached_file.txt" {
			t.Error("Cached file appeared after loading, but it should have been overwritten by real disk contents")
		}
	}

	if !foundReal {
		t.Error("Failed to load real file from disk under SyncPanelLoad")
	}
}

func TestFileSystemPanel_SelectionCleanupAfterReload(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	v := vfs.NewOSVFS(t.TempDir())
	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "ghost.txt"}},
	}
	fp.SetItemSelected(1, true)

	// Verify it is in the map initially
	if !fp.selectedItems["ghost.txt"] {
		t.Fatal("Initial selection missing from map")
	}

	// Now simulate a reload where "ghost.txt" is NOT in the VFS results
	// (we load a cache that has a different file)
	fp.saveToCache(v.GetPath(), []vfs.VFSItem{{Name: "other.txt"}})
	fp.readDirectoryEx(false)

	// Process the tasks
	waitForLoad(t, fp)

	// Verify "ghost.txt" was removed from selectedItems map because it no longer exists on disk
	if fp.selectedItems["ghost.txt"] {
		t.Error("Ghost selection was not cleaned up from selectedItems map after reload")
	}
}

func TestFileSystemPanel_Cache_FullCycle(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)

	// 1. Initial setup
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("b"), 0644)

	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	// 2. Initial fresh read
	waitForLoad(t, fp)

	// 3. Verify cache was populated
	if _, ok := fp.dirCache[tmpDir]; !ok {
		t.Fatal("Cache not populated after initial read")
	}
	if len(fp.entries) != 3 { // .., a.txt, b.txt
		t.Fatalf("Expected 3 entries, got %d", len(fp.entries))
	}
	for _, e := range fp.entries {
		if e.IsCached {
			t.Fatal("Initial read should not produce cached entries")
		}
	}

	// 4. Set cursor and modify backend
	fp.SelectName("b.txt")
	os.WriteFile(filepath.Join(tmpDir, "c.txt"), []byte("c"), 0644)
	os.Remove(filepath.Join(tmpDir, "a.txt"))

	// 5. Trigger cached read
	fp.readDirectoryEx(false)

	// 6. IMMEDIATE CHECKS (before async finishes)
	if len(fp.entries) != 3 {
		t.Fatalf("Immediately after reload, should show 3 cached entries, got %d", len(fp.entries))
	}
	if fp.GetSelectedName() != "b.txt" {
		t.Errorf("Cursor position lost, expected 'b.txt', got %q", fp.GetSelectedName())
	}
	foundA := false
	for _, e := range fp.entries {
		if e.Name == "c.txt" {
			t.Error("'c.txt' should not be visible in cached view")
		}
		if e.Name == "a.txt" {
			foundA = true
		}
		if e.Name != ".." && !e.IsCached {
			t.Errorf("Entry %q should be marked as cached", e.Name)
		}
	}
	if !foundA {
		t.Error("'a.txt' should still be visible in cached view")
	}

	// 7. ASYNC CHECKS (let the real read complete)
	waitForLoad(t, fp)

	if len(fp.entries) != 3 { // .., b.txt, c.txt
		t.Fatalf("After async update, expected 3 entries, got %d", len(fp.entries))
	}
	if fp.GetSelectedName() != "b.txt" {
		t.Errorf("Cursor position lost after async update, expected 'b.txt', got %q", fp.GetSelectedName())
	}
	foundC := false
	for _, e := range fp.entries {
		if e.Name == "a.txt" {
			t.Error("'a.txt' should have been removed after async update")
		}
		if e.Name == "c.txt" {
			foundC = true
		}
		if e.IsCached {
			t.Errorf("Entry %q should NOT be marked as cached after async update", e.Name)
		}
	}
	if !foundC {
		t.Error("'c.txt' not found after async update")
	}
}
func TestFileSystemPanel_LiveSelectionPreservation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)

	// 1. Setup: Panel with cached data
	fp := NewFileSystemPanel(0, 0, 40, 20, v)
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "item1"}, IsCached: true, Selected: true}, // Selected initially
		{VFSItem: vfs.VFSItem{Name: "item2"}, IsCached: true},
	}
	fp.Refresh()
	fp.SetCursorIndex(1) // Stand on item1

	// 2. Simulate User Action: Deselect item1, Select item2 and move cursor to it
	// (while the "real" scan is technically running in background)
	fp.entries[1].Selected = false
	fp.entries[2].Selected = true
	fp.SetCursorIndex(2)

	// 3. Simulate first "real" chunk arrival
	chunk := []vfs.VFSItem{
		{Name: "item1"},
		{Name: "item2"},
		{Name: "item3"},
	}

	// We'll manually call a reconstruction task similar to ReadDirectory
	selectedNames := map[string]bool{"item1": true}
	fp.pendingSelection = "item2"

	newEntries := make([]*fileEntry, len(chunk))
	for i, item := range chunk {
		newEntries[i] = &fileEntry{VFSItem: item}
	}

	// This block mimics the PostTask in ReadDirectory
	for _, e := range fp.entries {
		if e.Name != ".." {
			if e.Selected {
				selectedNames[e.Name] = true
			} else {
				delete(selectedNames, e.Name)
			}
		}
	}

	fp.entries = nil
	fp.entries = append(fp.entries, &fileEntry{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}})
	fp.entries = append(fp.entries, newEntries...)
	for _, e := range fp.entries {
		if e.Name != ".." && selectedNames[e.Name] {
			e.Selected = true
		}
	}

	fp.sortEntries()
	fp.SelectName(fp.pendingSelection)
	fp.Refresh()

	// 4. Verification
	if fp.GetSelectedName() != "item2" {
		t.Errorf("Cursor jump detected! Expected 'item2', got %q", fp.GetSelectedName())
	}

	foundSelected1 := false
	foundSelected2 := false
	for _, e := range fp.entries {
		if e.Name == "item1" && e.Selected {
			foundSelected1 = true
		}
		if e.Name == "item2" && e.Selected {
			foundSelected2 = true
		}
	}
	if foundSelected1 {
		t.Error("Deselection was lost during cache-to-real transition")
	}
	if !foundSelected2 {
		t.Error("Selection was lost during cache-to-real transition")
	}
}
func TestFileSystemPanel_ReadDir_ContextCancel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Используем NullVFS, так как он поддерживает пагинацию и задержки
	v := vfs.NewNullVFS(0)
	fp := NewFileSystemPanel(0, 0, 40, 20, v)

	// Запускаем чтение сценария IOPS (10 000 файлов)
	v.SetPath("/scenarios/iops")
	fp.ReadDirectory()

	if !fp.isLoading {
		t.Fatal("Panel should be loading")
	}

	// Имитируем отмену (например, переход в другую папку)
	fp.cancelLoad()

	// Прокачиваем задачи. Чанки не должны добавляться в список после отмены.
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			break loop
		}
	}

	if len(fp.entries) >= 10001 {
		t.Errorf("ReadDir was not cancelled: got %d entries", len(fp.entries))
	}
}

func TestFileSystemPanel_PendingSelectionPriority(t *testing.T) {
	// Проверяем, что pendingSelection (установленный, например, переименованием)
	// имеет приоритет над текущим положением курсора при прилете чанков данных.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewNullVFS(0))

	// Устанавливаем цель (новое имя файла после ренейма)
	fp.pendingSelection = "new_name.txt"
	// Текущий курсор "случайно" стоит на другом файле (например, индекс 0 - "..")
	fp.cursorIdx = 0

	// Логика из onChunk:
	// Если pendingSelection пустой, мы берем имя из текущего курсора.
	// В нашем случае он НЕ пустой, значит uName не должен переписать его.
	if fp.pendingSelection == "" {
		uName := fp.getRawSelectedName()
		if uName != "" && uName != ".." {
			fp.pendingSelection = uName
		}
	}

	if fp.pendingSelection != "new_name.txt" {
		t.Errorf("Pending selection was overwritten! Got %q, want 'new_name.txt'", fp.pendingSelection)
	}

	// 3. Симулируем прилет чанка, который СОДЕРЖИТ цель
	fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: ".."}}, {VFSItem: vfs.VFSItem{Name: "new_name.txt"}}}

	// Отрабатываем снаппинг
	target := fp.pendingSelection
	for i, entry := range fp.entries {
		if entry.Name == target {
			fp.SetCursorIndex(i)
			fp.pendingSelection = ""
			break
		}
	}

	if fp.GetSelectedName() != "new_name.txt" {
		t.Errorf("Cursor failed to snap to the renamed file. On: %q", fp.GetSelectedName())
	}
}
