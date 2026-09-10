package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Editor tests that press a key and expect an action to run. The editor reaches
// the action layer through its LookupHotkey seam, which the hotkey manager and
// the action table answer — and both live here. In internal/editor the registry
// is empty, so the same tests would pass by finding nothing to do.

// The same constructor internal/editor's multicursor tests use.
func multiCursorEditor(t *testing.T, text string) *editor.EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	ev := editor.NewEditorView(piecetable.New([]byte(text)), nil, "test.txt")
	t.Cleanup(ev.Close)
	ev.SetPosition(0, 0, 80, 12)
	return ev
}

// A frame that reports itself as the desktop, so the cursor-shape assertions
// see the editor as the active window. internal/editor's own tests declare the
// same three lines.
type desktopWindowWrapper struct {
	*editor.EditorView
}

func (d desktopWindowWrapper) GetType() vtui.FrameType {
	return vtui.TypeUser
}

// newWrapMemoryStore installs a file-state provider writing into a temporary
// directory and returns it, so a test can read back what the editor recorded.
func newWrapMemoryStore(t *testing.T) *fileops.F4FileStateProvider {
	t.Helper()
	fs := &fileops.F4FileStateProvider{
		Path:  filepath.Join(t.TempDir(), "file_states.json"),
		Limit: 10,
		Data:  make(map[string]*fileops.FileState),
	}
	old := fileops.GlobalFileState
	fileops.GlobalFileState = fs
	t.Cleanup(func() { fileops.GlobalFileState = old })
	return fs
}

func TestEditor_DeleteLine(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	Pt := piecetable.New([]byte("line1\nline2\nline3"))
	ev := editor.NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.CursorLine = 1
	ev.CursorPos = 2

	pressKey(ev, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_Y,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	expected := "line1\nline3"
	if ev.Pt.String() != expected {
		t.Errorf("Expected %q, got %q", expected, ev.Pt.String())
	}

	if ev.CursorLine != 1 {
		t.Errorf("Expected CursorLine 1, got %d", ev.CursorLine)
	}

	pressKey(ev, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_Y,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	expected2 := "line1"
	if ev.Pt.String() != expected2 {
		t.Errorf("Expected %q, got %q", expected2, ev.Pt.String())
	}

	if ev.CursorLine != 0 || ev.CursorPos != 5 {
		t.Errorf("Cursor misplaced after last line delete: Line %d, Pos %d", ev.CursorLine, ev.CursorPos)
	}

	ev.Undo()
	if ev.Pt.String() != "line1\nline3" {
		t.Errorf("Undo last line delete failed, got %q", ev.Pt.String())
	}
}

// Editor actions that know nothing about the caret set put it down first,
// rather than working through the primary caret and leaving the rest painted
// over text they no longer describe.
func TestEditor_MultiCursor_PlainActionsCollapseTheSet(t *testing.T) {
	ev := multiCursorEditor(t, "one\ntwo\nthree")
	ev.ToggleCursorAt(4)

	vtui.FrameManager.Push(ev)
	if !RunAction("Editor.DeleteLine") {
		t.Fatal("Editor.DeleteLine did not run")
	}

	if ev.MultiCursor() {
		t.Errorf("extra carets survived a single-caret action: %v", ev.ExtraCaretOffsets())
	}
}

// Without an active modal input state, Esc reaches the hotkey
// dispatcher and runs the Editor.Quit action.
func TestEditorView_EscRunsQuitAction(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	Pt := piecetable.New([]byte("foo"))
	ev := editor.NewEditorView(Pt, nil, "test.txt")

	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	})

	if !ev.IsDone() {
		t.Error("Editor should be closed by the Editor.Quit action")
	}
}

func TestEditorView_SaveFile(t *testing.T) {
	// 1. Create a temporary file
	tmpFile := "test_save.txt"
	t.Cleanup(func() {
		if err := os.Remove(tmpFile); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove temporary save file: %v", err)
		}
	})
	err := os.WriteFile(tmpFile, []byte("Original"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Open it in the editor
	Pt := piecetable.New([]byte("Original"))
	v := vfs.NewOSVFS(t.TempDir())
	ev := editor.NewEditorView(Pt, v, tmpFile)
	defer ev.Close()
	// Add mock file object to editor so SaveToFile logic triggers cleanly
	f, err := v.Open(context.Background(), tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	ev.File = f

	// 3. Simulate typing text " + Edit" at the end
	ev.CursorPos = 8
	for _, char := range " + Edit" {
		ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char})
	}

	// 4. Simulate pressing F2 (Save)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) // Needed for PostTask to work
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2})

	// 5. Wait for async save to finish by processing tasks
	timeout := time.After(1 * time.Second)
	for ev.Saving {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for async save to complete")
		}
	}

	// 6. Read file from disk and check that data was written
	savedData, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Original + Edit"
	if string(savedData) != expected {
		t.Errorf("Save failed: expected %q on disk, got %q", expected, string(savedData))
	}
}
func TestEditorView_HexModeToggleAndTyping(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	Pt := piecetable.New([]byte("abc"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 24)

	vtui.FrameManager.Push(ev)

	// Toggle Hex Mode
	RunAction("Editor.HexMode")
	if !ev.HexMode {
		t.Fatal("HexMode should be true")
	}

	// 'a' is 0x61. Let's type '4' '1' to change it to 'A' (0x41)
	ev.CursorPos = 0
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '4'})
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '1'})

	if Pt.String() != "Abc" {
		t.Errorf("Hex typing failed, got %q", Pt.String())
	}

	if ev.CursorPos != 1 || ev.HexNibble != 0 {
		t.Errorf("Cursor did not advance correctly after hex typing, got pos %d, nibble %d", ev.CursorPos, ev.HexNibble)
	}

	// Test navigation (left by nibble)
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if ev.CursorPos != 0 || ev.HexNibble != 1 {
		t.Errorf("Left arrow in hex mode failed, got pos %d, nibble %d", ev.CursorPos, ev.HexNibble)
	}
}

func TestEditorView_F3_ToggleWordWrap(t *testing.T) {
	Pt := piecetable.New([]byte("some text"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()
	ev.WordWrap = true

	// Press F3 (Wait, make sure your code uses VK_F3 now)
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3})
	if ev.WordWrap {
		t.Error("F3 failed to disable WordWrap")
	}

	// Press F3 again
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3})
	if !ev.WordWrap {
		t.Error("F3 failed to re-enable WordWrap")
	}
}

func TestEditorView_DefaultsAndToggles(t *testing.T) {
	Pt := piecetable.New([]byte("test content"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()

	// 1. Check defaults
	if ev.WordWrap {
		t.Error("WordWrap should be OFF by default")
	}
	if ev.ShowWhitespaces {
		t.Error("ShowWhitespaces should be OFF by default")
	}

	// 2. Toggle F3 (Wrap)
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3})
	if !ev.WordWrap {
		t.Error("F3 failed to toggle WordWrap to ON")
	}

	// 3. Toggle F5 (Whitespaces)
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5})
	if !ev.ShowWhitespaces {
		t.Error("F5 failed to toggle ShowWhitespaces to ON")
	}
}

func TestEditorView_SelectAll(t *testing.T) {
	Pt := piecetable.New([]byte("First\nSecond"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()

	// Ctrl+A
	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if !ev.SelActive {
		t.Fatal("Selection should be active after Ctrl+A")
	}
	min, max := ev.GetSelectionRange()
	if min != 0 || max != Pt.Size() {
		t.Errorf("Ctrl+A range failed: [0:%d], got [%d:%d]", Pt.Size(), min, max)
	}
	// Cursor should jump to EOF in Far
	if ev.CursorLine != 1 || ev.CursorPos != 6 {
		t.Errorf("Ctrl+A cursor pos failed, got %d:%d", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditorView_FarX_SmartCut(t *testing.T) {
	Pt := piecetable.New([]byte("Select me\nNext line"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 24)

	// Scenario A: Selection active -> Ctrl+X is CUT
	ev.SelActive = true
	ev.SelAnchorOffset = 0
	ev.CursorPos = 6 // "Select"
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_X, ControlKeyState: vtinput.LeftCtrlPressed})
	if Pt.String() != " me\nNext line" {
		t.Errorf("Ctrl+X Cut failed: %q", Pt.String())
	}

	// Scenario B: No selection -> Ctrl+X is DOWN
	ev.SelActive = false
	ev.CursorLine = 0
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_X, ControlKeyState: vtinput.LeftCtrlPressed})
	if ev.CursorLine != 1 {
		t.Error("Ctrl+X Down failed")
	}
}

func TestEditorView_FarSelectAll_Behavior(t *testing.T) {
	Pt := piecetable.New([]byte("All\nText"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()

	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if !ev.SelActive || ev.SelAnchorOffset != 0 {
		t.Error("Ctrl+A anchor should be 0")
	}
	if ev.CursorLine != 1 || ev.CursorPos != 4 {
		t.Errorf("Ctrl+A cursor should be at EOF, got %d:%d", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditorView_FarSelectAll(t *testing.T) {
	Pt := piecetable.New([]byte("Line 1\nLine 2"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()

	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if !ev.SelActive {
		t.Fatal("Selection should be active after Ctrl+A")
	}
	min, max := ev.GetSelectionRange()
	if min != 0 || max != Pt.Size() {
		t.Errorf("Ctrl+A range failed: [0:%d], got [%d:%d]", Pt.Size(), min, max)
	}
	// В Far курсор прыгает в конец после выделения всего текста
	if ev.CursorLine != 1 || ev.CursorPos != 6 {
		t.Errorf("Ctrl+A cursor pos failed, got %d:%d", ev.CursorLine, ev.CursorPos)
	}
}

func TestEditorView_FarX_CutVsDown(t *testing.T) {
	Pt := piecetable.New([]byte("Some selected text\nNext line"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()

	ev.SetPosition(0, 0, 80, 24)

	// 1. С выделением Ctrl+X должен сработать как Cut
	ev.SelActive = true
	ev.SelAnchorOffset = 0
	ev.CursorPos = 4 // Выделено "Some"

	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_X, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if Pt.String() != " selected text\nNext line" {
		t.Errorf("Ctrl+X (Cut) failed: text is %q", Pt.String())
	}

	// 2. Без выделения Ctrl+X должен сработать как Down (навигация Far)
	ev.SelActive = false
	ev.CursorLine = 0
	ev.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_X, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if ev.CursorLine != 1 {
		t.Error("Ctrl+X without selection should move cursor down")
	}
}

func TestEditorView_Search_ShiftF7_Reverse(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// "one two three"
	//  0123456789012
	//  e: 2, 11, 12
	content := "one two three"
	Pt := piecetable.New([]byte(content))
	ev := editor.NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 24)

	// 1. Initial backward search from end (next=false)
	// Should find the 'e' at index 12.
	ev.SelActive = false
	ev.CursorPos = 13
	ev.Search("e", false, true, false, false, false)

	timeout := time.After(1 * time.Second)
	for !ev.SelActive {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Backward search 1 timed out")
		}
	}
	if ev.SelAnchorOffset != 12 {
		t.Errorf("Expected offset 12, got %d", ev.SelAnchorOffset)
	}

	// Drain leftover tasks AND WAIT for any pending async search logic to finish.
	// This makes the test deterministic under load.
	vtui.DebugLog("TEST_SEARCH: Pumping tasks after first search...")
	pumpDeadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(pumpDeadline) {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// 2. "Find Next" backward search (Shift+F7)
	// Cursor is at 13 (end of match). Reverse Next should skip index 12 and find 11.
	ev.SelActive = false
	editor.LastEditorSearchReverse = true
	vtui.DebugLog("TEST_SEARCH: Triggering Search 2. Current CursorPos: %d", ev.CursorPos)
	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_F7, ControlKeyState: vtinput.ShiftPressed,
	})

	// Use a more robust drain to handle async search completion
	found := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !found {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if ev.SelActive {
				found = true
			}
			// Check if we hit the "Not found" dialog
			if top := vtui.FrameManager.GetTopFrame(); top != nil && top.GetTitle() == " Search " {
				t.Fatal("Search reported 'Not found' unexpectedly")
			}
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !found {
		t.Fatal("Shift+F7 backward search failed to find second match")
	}

	if ev.SelAnchorOffset != 11 {
		t.Errorf("Shift+F7 reverse (Next) failed: expected offset 11, got %d", ev.SelAnchorOffset)
	}
}

func TestEditorView_SaveFailure_NoDataLoss(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpFile := filepath.Join(t.TempDir(), "important.txt")
	if err := os.WriteFile(tmpFile, []byte("Original"), 0600); err != nil {
		t.Fatal(err)
	}

	// Use our failing VFS
	baseVfs := vfs.NewOSVFS(filepath.Dir(tmpFile))
	failingVfs := &mockFailingVFS{VFS: baseVfs, failCreate: true}

	Pt := piecetable.New([]byte("Original"))
	ev := editor.NewEditorView(Pt, failingVfs, tmpFile)
	defer ev.Close()
	f, err := failingVfs.Open(context.Background(), tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	ev.File = f

	// 1. Modify the file
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'X'})
	if !ev.Modified {
		t.Fatal("Editor should be modified")
	}

	// 2. Attempt to save (F2)
	pressKey(ev, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2})

	// Process async tasks
	timeout := time.After(2 * time.Second)
	saveFinished := false
	for !saveFinished {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if !ev.Saving {
				saveFinished = true
			}
		case <-timeout:
			t.Fatal("Timeout waiting for save operation")
		}
	}

	// 3. Assertions
	// The modified flag MUST remain true because the save failed!
	if !ev.Modified {
		t.Error("CRITICAL: Editor 'modified' flag was cleared even though save failed! Data loss risk.")
	}

	// Original file must remain untouched
	data, _ := os.ReadFile(tmpFile)
	if string(data) != "Original" {
		t.Errorf("CRITICAL: Original file was corrupted during failed save. Got %q", string(data))
	}

	// Should have popped an error dialog
	if vtui.FrameManager.GetTopFrameType() != vtui.TypeDialog {
		t.Error("Editor did not show an error dialog upon save failure")
	}
}
func TestEditorView_ModificationStress(t *testing.T) {
	// Tests stability of LineIndex and navigation during randomized edits.
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	Pt := piecetable.New([]byte(content))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 24)
	ev.WordWrap = true

	// A sequence of mixed operations
	ops := []struct {
		char uint16
		vk   uint16
		ctrl bool
	}{
		{vk: vtinput.VK_END},
		{char: 'a'}, {char: 'b'}, {char: 'c'},
		{vk: vtinput.VK_RETURN},
		{char: 'x'}, {char: 'y'}, {char: 'z'},
		{vk: vtinput.VK_UP},
		{vk: vtinput.VK_HOME},
		{vk: vtinput.VK_DELETE}, {vk: vtinput.VK_DELETE},
		{vk: vtinput.VK_BACK},
		{char: ' '},
		{vk: vtinput.VK_A, ctrl: true}, // Select all
		{vk: vtinput.VK_DELETE},        // Wipe document
		{char: 'R'}, {char: 'e'}, {char: 's'}, {char: 't'}, {char: 'a'}, {char: 'r'}, {char: 't'},
	}

	for i, op := range ops {
		ctrlFlag := vtinput.ControlKeyState(0)
		if op.ctrl {
			ctrlFlag = vtinput.LeftCtrlPressed
		}
		pressKey(ev, &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			Char:            rune(op.char),
			VirtualKeyCode:  op.vk,
			ControlKeyState: ctrlFlag,
		})

		// After every op, verify LineIndex integrity
		expectedLi := piecetable.NewLineIndex()
		expectedLi.Rebuild(ev.Pt)
		if ev.Li.LineCount() != expectedLi.LineCount() {
			t.Fatalf("Step %d: LineCount mismatch. Got %d, want %d", i, ev.Li.LineCount(), expectedLi.LineCount())
		}
	}

	if ev.Pt.String() != "Restart" {
		t.Errorf("Stress test result mismatch: %q", ev.Pt.String())
	}
}

func TestEditorView_CrosshairStateAndNoLeak(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	oldCrosshair := config.App.EditorCrosshair
	config.App.EditorCrosshair = true
	defer func() { config.App.EditorCrosshair = oldCrosshair }()

	Pt := piecetable.New([]byte("line1\nline2\nline3"))
	ev := editor.NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 11)

	ev.CursorLine = 1
	ev.CursorPos = 2
	ev.SetFocus(true)

	ev.Show(scr)

	crossAttr := vtui.Palette[theme.ColEditorCrosshair]
	crossBG := vtui.GetRGBBack(crossAttr)

	activeRowCell := scr.GetCell(5, 2)
	if vtui.GetRGBBack(activeRowCell.Attributes) != crossBG {
		t.Errorf("Expected active row Y=2 to have crosshair background %06X, got %06X", crossBG, vtui.GetRGBBack(activeRowCell.Attributes))
	}

	nonActiveRowCell := scr.GetCell(5, 1)
	if vtui.GetRGBBack(nonActiveRowCell.Attributes) == crossBG {
		t.Error("Non-active row erroneously has crosshair background (sticking/leakage)")
	}

	verticalCell := scr.GetCell(2, 1)
	if vtui.GetRGBBack(verticalCell.Attributes) != crossBG {
		t.Errorf("Expected vertical crosshair column X=2 on non-active row Y=1 to have crosshair background, got %06X", vtui.GetRGBBack(verticalCell.Attributes))
	}

	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.Show(scr)

	activeRowCellAfter := scr.GetCell(5, 2)
	if vtui.GetRGBBack(activeRowCellAfter.Attributes) == crossBG {
		t.Error("Crosshair background leaked/stuck on row 2 after cursor moved away")
	}
}
func TestEditor_InsertToggle(t *testing.T) {
	Pt := piecetable.New([]byte("data"))
	ev := editor.NewEditorView(Pt, nil, "")
	defer ev.Close()

	if ev.Overtype {
		t.Error("Editor should start in Insert mode")
	}

	// Нажимаем Insert
	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_INSERT,
	})

	if !ev.Overtype {
		t.Error("Insert key failed to toggle Overtype mode")
	}
}

func TestEditorViewInsertOverwriteCursorShape(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	Pt := piecetable.New([]byte("hello world"))
	ev := editor.NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.ResizeConsole(80, 25)

	vtui.FrameManager.Push(desktopWindowWrapper{ev})

	// Симулируем рендеринг, чтобы обновить состояние ScreenBuf
	ev.Show(scr)
	scr.Flush()

	// По умолчанию overtype = false, курсор должен быть Underline
	if ev.Overtype {
		t.Error("Expected default overtype mode to be false")
	}

	_, cy := scr.GetCursorPos()
	// Проверяем форму курсора на активной строке
	if cy >= 0 {
		_, _, _, shape := scr.GetCursorStateForTesting()
		if shape != vtui.CursorShapeUnderline {
			t.Errorf("Expected cursor shape to be Underline, got %v", shape)
		}
	}

	// Нажимаем Insert
	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_INSERT,
	})

	// Проверяем, что режим сменился на overtype
	if !ev.Overtype {
		t.Error("Expected overtype mode to be true after pressing Insert")
	}

	// Рендерим заново
	ev.Show(scr)
	scr.Flush()

	// Теперь форма курсора должна быть Block
	if cy >= 0 {
		_, _, _, shape := scr.GetCursorStateForTesting()
		if shape != vtui.CursorShapeBlock {
			t.Errorf("Expected cursor shape to be Block after toggling overtype, got %v", shape)
		}
	}
}

// The choice must reach the store when it is made, not only at close: an
// editor still open when f4 exits never reaches Close.
func TestEditorView_WordWrapToggleIsRemembered(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	fs := newWrapMemoryStore(t)
	t.Cleanup(func() { fs.Flush() })

	ev := editor.NewEditorView(piecetable.New([]byte("some text")), nil, "wrapped.txt")
	defer ev.Close()

	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})
	if !ev.WordWrap {
		t.Fatal("F3 did not turn word wrap on")
	}
	fs.Flush()

	state := fs.GetState(fileops.FileStateKey(nil, "wrapped.txt"))
	if state == nil || !state.EditorWrap {
		t.Fatal("word wrap turned on with F3 was not remembered for the file")
	}

	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})
	if ev.WordWrap {
		t.Fatal("F3 did not turn word wrap off again")
	}
	fs.Flush()

	if state = fs.GetState(fileops.FileStateKey(nil, "wrapped.txt")); state == nil || state.EditorWrap {
		t.Fatal("word wrap turned off again was not remembered for the file")
	}
}
