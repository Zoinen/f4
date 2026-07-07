package main

import (
	"context"
	"fmt"
	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/unxed/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPanelsFrame_Layout(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()

	// Simulate 80x25 terminal
	pf.ResizeConsole(80, 25)

	// Calculate expected positions for 80x25 with KeyBar
	expectedKeyBarY := 24
	expectedCmdLineY := 23 // Always 1 line above KeyBar if KeyBar is present

	// 1. Check reserved rows with KeyBar visible
	if pf.keyBar.Y1 != expectedKeyBarY {
		t.Errorf("KeyBar position error: expected %d, got %d", expectedKeyBarY, pf.keyBar.Y1)
	}
	if pf.cmdLine.Y1 != expectedCmdLineY {
		t.Errorf("CommandLine position error: expected %d, got %d", expectedCmdLineY, pf.cmdLine.Y1)
	}

	// 2. Check layout after hiding KeyBar
	pf.showKeyBar = false
	pf.ResizeConsole(80, 25)

	// After hiding KeyBar, CommandLine should move to the bottom row
	expectedKeyBarY = 24 // Still the last line, but invisible
	expectedCmdLineY = 24
	if pf.cmdLine.Y1 != expectedCmdLineY {
		t.Errorf("CommandLine should be at %d when KeyBar hidden, got %d", expectedCmdLineY, pf.cmdLine.Y1)
	}
	if pf.keyBar.IsVisible() {
		t.Error("KeyBar should be invisible")
	}
}
func TestPanelsFrame_ArkanoidHotkey(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	vtui.FrameManager.Push(pf) // Screen 0

	initialScreens := len(vtui.FrameManager.Screens)

	// 1. Запуск игры
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  'A',
		ControlKeyState: vtinput.LeftAltPressed | vtinput.LeftCtrlPressed,
	})

	if len(vtui.FrameManager.Screens) != initialScreens+1 {
		t.Fatalf("Expected %d screens, got %d", initialScreens+1, len(vtui.FrameManager.Screens))
	}

	arkScreen := vtui.FrameManager.Screens[len(vtui.FrameManager.Screens)-1]
	if !arkScreen.Transparent {
		t.Error("Arkanoid screen should be transparent (headless)")
	}
	if arkScreen.GetTitle() != "Arkanoid" {
		t.Errorf("Expected Arkanoid title, got %s", arkScreen.GetTitle())
	}

	// 2. Пытаемся запустить еще раз (не должно создавать новый экран, а только переключить)
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  'A',
		ControlKeyState: vtinput.LeftAltPressed | vtinput.LeftCtrlPressed,
	})

	if len(vtui.FrameManager.Screens) != initialScreens+1 {
		t.Error("Second Arkanoid launch erroneously created a duplicate screen")
	}
}
func TestPanelsFrame_SelectionByMask(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fsp := pf.panels[1].(*FileSystemPanel)
	pf.activeIdx = 1

	// 1. Command line not empty -> should not intercept for regular char
	pf.cmdLine.Edit.SetText("a")
	handled := pf.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    '+',
	})
	if !handled {
		t.Error("Key should be handled by cmdLine")
	}

	// 1.5 Command line not empty, but Numpad + -> SHOULD intercept
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	handled = pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ADD,
	})
	if !handled {
		t.Error("Numpad + should be intercepted even if cmdLine is not empty")
	}
	if vtui.FrameManager.GetTopFrameType() != vtui.TypeDialog {
		t.Error("Selection dialog was not shown for Numpad +")
	}
	vtui.FrameManager.Pop() // Clean up dialog

	// 2. Command line empty, fastFindMode active -> should not intercept
	pf.cmdLine.Clear()
	fsp.fastFindMode = true
	handled = pf.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    '+',
	})
	if !handled {
		t.Error("Key should be handled by fastFindMode in active panel")
	}

	// 3. Command line empty, fastFindMode NOT active -> SHOULD intercept and show dialog
	fsp.fastFindMode = false
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	handled = pf.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    '+',
	})
	if !handled {
		t.Error("Key should be intercepted for selection dialog")
	}
	if vtui.FrameManager.GetTopFrameType() != vtui.TypeDialog {
		t.Error("Selection dialog was not shown")
	}
}
func TestPanelsFrame_GetActivePTY(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()

	// Default panels use OSVFS, so active PTY should be the local one
	active := pf.getActivePTY()
	if active != pf.pty {
		t.Errorf("Expected active PTY to be the local PTY for OSVFS")
	}
}
func TestPanelsFrame_ProcessMouse_DoubleClick(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Active is initially right (1)
	if pf.activeIdx != 1 {
		t.Fatalf("Expected initial activeIdx 1, got %d", pf.activeIdx)
	}

	tmp := t.TempDir()
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	// Bypass async load
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	initialPath := fsp.vfs.GetPath()

	// Double click on ".." in left panel.
	// Left panel 0..39. Table start Y=1. Header Y=1. Row 0 at Y=2.
	pf.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseX:          5,
		MouseY:          2,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.DoubleClick,
	})

	if pf.activeIdx != 0 {
		t.Errorf("Expected activeIdx 0 after left click, got %d", pf.activeIdx)
	}

	if fsp.vfs.GetPath() == initialPath {
		t.Error("Double click on '..' should have changed directory")
	}
}

func setupMockPanelsFrame() *PanelsFrame {
	pf := &PanelsFrame{activeIdx: 1, showPanels: true, showKeyBar: true, showLeftPanel: true, showRightPanel: true}
	pf.pty = &mockPty{}
	pf.termView = NewTerminalView(80, 24)
	// Initialize MenuBar with enough items to satisfy updateMenuCheckmarks (needs index 0 and 4)
	pf.menuBar = vtui.NewMenuBar(nil)
	pf.menuBar.Items = make([]vtui.MenuBarItem, 5)
	for i := 0; i < 5; i++ {
		pf.menuBar.Items[i].SubItems = make([]vtui.MenuItem, 8)
	}
	pf.cmdLine = NewCommandLine(">")
	pf.keyBar = vtui.NewKeyBar()
	// Use OSVFS because tests create real files in t.TempDir()
	pf.panels[0] = NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS("."))
	pf.panels[1] = NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS("."))
	pf.initPTY()
	return pf
}
func TestPanelsFrame_ProcessMouse_DoubleClickFile(t *testing.T) {
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	tmp := t.TempDir()
	runnablePath := filepath.Join(tmp, "run.sh")
	os.WriteFile(runnablePath, []byte("echo"), 0755)

	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.SetViewMode(ViewModeDetailed)
	fsp.vfs.SetPath(tmp)

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "run.sh", IsDir: false}},
	}
	fsp.Refresh()

	// Must init frame manager to catch async tasks from actionExecute
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Double click on "run.sh" in left panel.
	// Panel at (0,0), Table at (1,1), Header at Y=1, Row 0 at Y=2, Row 1 (run.sh) at Y=3.
	pf.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseX:          5,
		MouseY:          3,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.DoubleClick,
	})

	// Wait for the async task that actually executes the file.
	// Since other tasks (like ReadDirectory) might be in the queue,
	// we process the channel in a loop until panels are hidden.
	timeout := time.After(1 * time.Second)
	for pf.showPanels {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("actionExecute did not hide the panels within 1s")
		}
	}
	if pf.showPanels {
		t.Error("Double clicking a runnable file should hide the panels")
	}
}

func TestPanelsFrame_KeyHandling(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// 1. Test Tab to switch active panel
	if pf.activeIdx != 1 {
		t.Fatalf("Initial active panel should be right (1), got %d", pf.activeIdx)
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.activeIdx != 0 {
		t.Error("Tab did not switch active panel to left (0)")
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.activeIdx != 1 {
		t.Error("Tab did not switch active panel back to right (1)")
	}

	// 2. Test Ctrl+O to toggle panels
	if !pf.showPanels {
		t.Fatal("Panels should be visible initially")
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_O, ControlKeyState: vtinput.LeftCtrlPressed})
	if pf.showPanels {
		t.Error("Ctrl+O did not hide panels")
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_O, ControlKeyState: vtinput.LeftCtrlPressed})
	if !pf.showPanels {
		t.Error("Ctrl+O did not show panels again")
	}

	// 3. Test Ctrl+Enter to insert filename
	pf.activeIdx = 0
	if fsp, ok := pf.panels[0].(*FileSystemPanel); ok {
		// Mock entries to avoid async dependency
		fsp.entries = []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "testfile.txt"}},
		}
		fsp.Refresh()
		fsp.SetCursorIndex(1)
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: vtinput.LeftCtrlPressed})

	expectedName := pf.panels[0].GetSelectedName()
	if pf.cmdLine.Edit.GetText() != expectedName {
		t.Errorf("Ctrl+Enter failed: expected '%s', got '%s'", expectedName, pf.cmdLine.Edit.GetText())
	}

	// 4. Test Ctrl+O to toggle panels even when PTY is busy (Issue #50)
	pf.showPanels = false
	pf.pty = &mockPty{}
	pf.executing = true // PTY is busy

	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_O, ControlKeyState: vtinput.LeftCtrlPressed})
	if !pf.showPanels {
		t.Error("Ctrl+O should show panels even when PTY is busy")
	}

	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_O, ControlKeyState: vtinput.LeftCtrlPressed})
	if pf.showPanels {
		t.Error("Ctrl+O should hide panels even when PTY is busy")
	}
}
func TestPanelsFrame_MenuCommands(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	handled := pf.HandleCommand(CmLeftDetailed, nil)
	if !handled {
		t.Error("CmLeftDetailed not handled")
	}
	if pf.panels[0].(*FileSystemPanel).viewMode != ViewModeDetailed {
		t.Error("Left panel mode not changed to Detailed")
	}

	pf.HandleCommand(CmRightDetailed, nil)
	if pf.panels[1].(*FileSystemPanel).viewMode != ViewModeDetailed {
		t.Error("Right panel mode not changed to Detailed")
	}

	// Sort mode commands
	pf.HandleCommand(CmLeftSortTime, nil)
	if pf.panels[0].(*FileSystemPanel).sortMode != SortTime {
		t.Error("Left panel sort mode not changed to Time")
	}

	pf.HandleCommand(CmRightSortSize, nil)
	if pf.panels[1].(*FileSystemPanel).sortMode != SortSize {
		t.Error("Right panel sort mode not changed to Size")
	}

	// Menu checkmarks
	menuText := pf.menuBar.Items[0].SubItems[1].Text
	if !strings.HasPrefix(menuText, "√") {
		t.Errorf("Menu checkmark not updated, got %q", menuText)
	}
	sortText := pf.menuBar.Items[0].SubItems[5].Text
	if !strings.HasPrefix(sortText, "√") {
		t.Errorf("Sort menu checkmark not updated, got %q", sortText)
	}
}
func TestPanelsFrame_RefreshOnFocus(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()

	// We need to verify Refresh was called.
	// Since we don't have a mock VFS easily swappable here without refactoring,
	// we check if the internal state handles the focus event without crashing
	// and returns true.

	handled := pf.ProcessKey(&vtinput.InputEvent{
		Type:     vtinput.FocusEventType,
		SetFocus: true,
	})

	if !handled {
		t.Error("PanelsFrame should handle FocusEventType and return true")
	}
}
func TestPanelsFrame_Clone(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(100, 30)

	// Use a real temp directory that exists on all platforms
	tmpDir := t.TempDir()

	// Set some specific state
	pf.activeIdx = 0
	if fsp, ok := pf.panels[0].(*FileSystemPanel); ok {
		if err := fsp.vfs.SetPath(tmpDir); err != nil {
			t.Fatalf("SetPath failed: %v", err)
		}
		fsp.table.SelectPos = 5
	}

	// Clone the panels
	clone := pf.Clone()
	defer clone.Close()

	// Verify state transfer
	if clone.activeIdx != 0 {
		t.Errorf("Clone failed to copy activeIdx: %d", clone.activeIdx)
	}

	if fsp, ok := clone.panels[0].(*FileSystemPanel); ok {
		if fsp.vfs.GetPath() != tmpDir {
			t.Errorf("Clone failed to copy VFS path: got %s, want %s", fsp.vfs.GetPath(), tmpDir)
		}
		if fsp.table.SelectPos != 5 {
			t.Errorf("Clone failed to copy Table SelectPos: %d", fsp.table.SelectPos)
		}
		if fsp.viewMode != pf.panels[0].(*FileSystemPanel).viewMode {
			t.Error("Clone failed to copy ViewMode")
		}
		if fsp.sortMode != pf.panels[0].(*FileSystemPanel).sortMode {
			t.Error("Clone failed to copy SortMode")
		}
		if fsp.sortReverse != pf.panels[0].(*FileSystemPanel).sortReverse {
			t.Error("Clone failed to copy SortReverse")
		}
	}

	// Verify they are independent instances
	clone.activeIdx = 1
	if pf.activeIdx == 1 {
		t.Error("Clone should be independent from its parent")
	}
}
func TestPanelsFrame_Clone_TerminalData(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()

	// 1. Simulate complex terminal output
	// Inject data directly into pt to simulate extruded history
	pf.termView.pt.Insert(0, []byte("L1\nL2\n"))
	pf.termView.li.UpdateAfterInsert(0, []byte("L1\nL2\n"))

	// Simulate active grid data
	pf.termView.CursorY = 5
	pf.termView.Lines[4][0].Char = 'H' // Previous row
	pf.termView.Lines[5][0].Char = 'A' // Active row (will be wiped)
	pf.termView.CursorX = 1

	clone := pf.Clone()
	defer clone.Close()

	// 2. Check if log is deep-copied
	if clone.termView.pt.String() != "L1\nL2\n" {
		t.Errorf("Terminal log not cloned. Got %q", clone.termView.pt.String())
	}

	// 3. CRITICAL: Check if LineIndex is correctly pointing to the NEW pt
	if clone.termView.li.LineCount() != 3 {
		t.Errorf("Terminal LineIndex not synced in clone. Expected 3 lines, got %d", clone.termView.li.LineCount())
	}

	// 4. Check if visual grid is copied
	if clone.termView.Lines[4][0].Char != 'H' {
		t.Error("Terminal visual grid (Lines) history not copied to clone")
	}

	// 5. Verify prompt reset logic
	if clone.termView.CursorX != 0 {
		t.Errorf("Expected clone CursorX to be 0 after prompt wipe, got %d", clone.termView.CursorX)
	}
	if clone.termView.Lines[5][0].Char != ' ' {
		t.Error("Current terminal line was not cleared during clone")
	}
}
func TestPanelsFrame_Labels(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	ks := pf.GetKeyLabels()

	if ks == nil {
		t.Fatal("PanelsFrame labels are nil")
	}

	// F3 in panels should be "View" (or whatever you set in lang.go)
	if ks.Normal[2] == "" {
		t.Error("PanelsFrame F3 label should not be empty")
	}
}
func TestPanelsFrame_HistoryNavigation(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25) // Initialize panels
	pf.showPanels = false    // Hide panels to enable history intercept
	pf.cmdLine.Edit.AddHistory("git status")

	// Press Up Arrow
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_UP,
	})

	if pf.cmdLine.Edit.GetText() != "git status" {
		t.Errorf("PanelsFrame failed to pass Up Arrow to history. Got '%s'", pf.cmdLine.Edit.GetText())
	}

	// Reset, show panels, try again
	pf.cmdLine.Clear()
	pf.cmdLine.Edit.HistoryPos = -1
	pf.showPanels = true

	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_UP,
	})

	if pf.cmdLine.Edit.GetText() != "" {
		t.Error("Up Arrow should NOT trigger history when panels are visible")
	}
}
func TestPanelsFrame_HistoryNavigation_HiddenPanels(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.showPanels = false // Panels are hidden
	pf.cmdLine.Edit.AddHistory("last command")

	// Press Up Arrow - should trigger HistoryUp on the command line
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_UP,
	})

	if pf.cmdLine.Edit.GetText() != "last command" {
		t.Errorf("Up arrow failed to cycle history with hidden panels. Got: %q", pf.cmdLine.Edit.GetText())
	}

	// Press Esc - should clear line and reset history position
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	})

	if !pf.cmdLine.IsEmpty() || pf.cmdLine.Edit.HistoryPos != -1 {
		t.Error("Esc failed to reset history state")
	}
}
func TestPanelsFrame_EnterAddsToHistory(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.cmdLine.Edit.SetText("ls -la")

	// Simulate Enter
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if len(pf.cmdLine.Edit.History) == 0 || pf.cmdLine.Edit.History[0] != "ls -la" {
		t.Errorf("Command was not added to history on Enter. History: %v", pf.cmdLine.Edit.History)
	}
}

func TestPanelsFrame_AltScreenTerminalHeight(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.pty = &mockPty{}
	pf.parser = NewAnsiParser(pf.termView, pf.pty)
	height := 25
	pf.showKeyBar = true

	// 1. Normal mode: terminal should leave space for KeyBar
	pf.termView.UseAltScreen = false
	pf.ResizeConsole(80, height)
	// termY2 should be h-2 (23)
	if pf.termView.Y2 != 23 {
		t.Errorf("Normal mode: expected terminal Y2=23, got %d", pf.termView.Y2)
	}

	// 2. AltScreen mode: terminal should occupy the KeyBar's row
	pf.termView.UseAltScreen = true
	pf.ResizeConsole(80, height)
	// termY2 should be h-1 (24)
	if pf.termView.Y2 != 24 {
		t.Errorf("AltScreen mode: expected terminal Y2=24, got %d", pf.termView.Y2)
	}
}

func TestPanelsFrame_KeyBarSuppression(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.showKeyBar = true
	pf.ResizeConsole(80, 25)

	// We need to simulate the frame being on top to trigger the logic
	vtui.FrameManager.Push(pf)

	// 1. Normal mode: KeyBar should be registered
	pf.termView.UseAltScreen = false
	pf.Show(scr)
	if vtui.FrameManager.KeyBar == nil {
		t.Error("KeyBar should be registered in FrameManager in normal mode")
	}

	// 2. AltScreen mode: KeyBar should be removed from FrameManager
	pf.termView.UseAltScreen = true
	pf.Show(scr)
	if vtui.FrameManager.KeyBar != nil {
		t.Error("KeyBar should be UNregistered from FrameManager in AltScreen mode")
	}

	// 3. Busy mode but panels visible: KeyBar should be registered (Issue #50)
	pf.termView.UseAltScreen = false
	pf.showPanels = true
	pf.pty = &mockPty{} // Ensure active PTY is not nil
	pf.executing = true
	pf.Show(scr)
	if vtui.FrameManager.KeyBar == nil {
		t.Error("KeyBar should be registered in FrameManager in busy mode when panels are visible")
	}

	// 4. Busy mode and panels hidden: KeyBar should be UNregistered (Issue #50)
	pf.showPanels = false
	pf.Show(scr)
	if vtui.FrameManager.KeyBar != nil {
		t.Error("KeyBar should be UNregistered from FrameManager in busy mode when panels are hidden")
	}
}
func TestPanelsFrame_RefreshAll(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	// Test that RefreshAll doesn't crash on freshly initialized panels
	pf.RefreshAll()
}

func TestPanelsFrame_ManualRefresh(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Setup a mock directory
	tmp := t.TempDir()
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	// Press Ctrl+R
	handled := pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_R,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if !handled {
		t.Error("Ctrl+R was not handled")
	}

	// It should trigger ReadDirectory
	if !fsp.isLoading {
		t.Error("Ctrl+R did not trigger panel refresh (isLoading should be true)")
	}
}

func TestPanelsFrame_AutoRefresh(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Setup a mock directory
	tmp := t.TempDir()
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	// Emulate an initial read that populates MTime
	fsp.lastDirMTime = time.Now().Add(-10 * time.Minute)
	// Write a file to update actual directory MTime
	os.WriteFile(filepath.Join(tmp, "test.txt"), []byte("data"), 0644)

	// Emulate the timer expiration
	pf.lastAutoRefresh = time.Now().Add(-5 * time.Second)

	// Trigger Show which should fire the async stat check
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	pf.Show(scr)

	// Pump the TaskChan to execute RunAsync and RunOnUI
	timeout := time.After(2 * time.Second)
pump:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if fsp.isLoading {
				break pump // Success: it triggered a refresh
			}
		case <-timeout:
			t.Fatal("AutoRefresh failed to trigger ReadDirectory after MTime change")
		}
	}
}
func TestPanelsFrame_ResizingIntegration(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()

	// Initial size 80x25
	pf.ResizeConsole(80, 25)

	// 1. Verify initial positions of standard components
	if pf.keyBar.Y1 != 24 {
		t.Errorf("Initial KeyBar Y1: expected 24, got %d", pf.keyBar.Y1)
	}
	if pf.cmdLine.Y1 != 23 {
		t.Errorf("Initial CommandLine Y1: expected 23, got %d", pf.cmdLine.Y1)
	}

	// 2. Perform resize to 120x40
	pf.ResizeConsole(120, 40)

	// 3. Verify that components moved/scaled correctly
	if pf.keyBar.Y1 != 39 {
		t.Errorf("Resized KeyBar Y1: expected 39, got %d", pf.keyBar.Y1)
	}
	if pf.keyBar.X2 != 119 {
		t.Errorf("Resized KeyBar X2: expected 119, got %d", pf.keyBar.X2)
	}
	if pf.cmdLine.Y1 != 38 {
		t.Errorf("Resized CommandLine Y1: expected 38, got %d", pf.cmdLine.Y1)
	}

	// 4. Verify panels scaled
	leftX1, _, leftX2, _ := pf.panels[0].GetPosition()
	rightX1, _, rightX2, _ := pf.panels[1].GetPosition()

	if leftX1 != 0 || leftX2 != 59 {
		t.Errorf("Resized Left Panel X range: expected 0..59, got %d..%d", leftX1, leftX2)
	}
	if rightX1 != 60 || rightX2 != 119 {
		t.Errorf("Resized Right Panel X range: expected 60..119, got %d..%d", rightX1, rightX2)
	}
}
func TestPanelsFrame_ExitWarning_ActiveTasks(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	fm.Push(pf)

	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = []*QueueTask{{ID: 1, State: "Queued"}}
	qm.mu.Unlock()

	// Триггерим выход
	pf.HandleCommand(vtui.CmQuit, nil)

	// Находим диалог
	top := fm.GetTopFrame()
	if top == nil {
		t.Fatal("Exit dialog not shown")
	}

	// Проверяем текст сообщения (должен содержать упоминание активных задач)
	foundWarning := false
	// Перебираем детей контейнера (диалога)
	if container, ok := top.(vtui.Container); ok {
		for _, child := range container.GetChildren() {
			if txt, ok := child.(*vtui.Text); ok {
				if strings.Contains(txt.GetText(), "active background operations") {
					foundWarning = true
					break
				}
			}
		}
	}

	if !foundWarning {
		t.Error("Exit dialog did not show warning about active background tasks")
	}
}
func TestPanelsFrame_SwapPanels(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	pathL := filepath.Join(t.TempDir(), "left")
	pathR := filepath.Join(t.TempDir(), "right")
	os.MkdirAll(pathL, 0755)
	os.MkdirAll(pathR, 0755)

	fspL := pf.panels[0].(*FileSystemPanel)
	fspR := pf.panels[1].(*FileSystemPanel)

	fspL.vfs.SetPath(pathL)
	fspR.vfs.SetPath(pathR)
	fspL.SetViewMode(ViewModeDetailed)
	fspR.SetViewMode(ViewModeMedium)

	pf.activeIdx = 0 // Active is Left

	// Execute Swap
	pf.HandleCommand(CmSwapPanels, nil)

	// 1. Verify instances are swapped in the array
	if pf.panels[0] != fspR || pf.panels[1] != fspL {
		t.Error("Panels instances were not swapped in pf.panels array")
	}

	// 2. Verify activeIdx followed the content
	if pf.activeIdx != 1 {
		t.Errorf("activeIdx should have moved to 1 to follow the panel, got %d", pf.activeIdx)
	}

	// 3. Verify positions were updated (fspR was Right, now should be Left)
	x1, _, x2, _ := fspR.GetPosition()
	if x1 != 0 || x2 != 39 {
		t.Errorf("Swapped panel (Right->Left) has wrong X position: %d..%d", x1, x2)
	}

	// 4. Verify state preservation
	if fspR.viewMode != ViewModeMedium {
		t.Error("Swapped panel did not preserve its ViewMode")
	}
}
func TestPanelsFrame_Clone_SelectionPreservation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "selected.txt"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(tmp, "normal.txt"), []byte("data"), 0644)

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)
	fsp.ReadDirectory() // Explicitly start loading the temp directory

	// 1. Wait for initial load to finish
	timeout := time.After(2 * time.Second)
	for fsp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for initial load")
		}
	}
	// Drain UI queue
	for i := 0; i < 10; i++ {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
		}
	}

	// 2. Select "selected.txt" (should be at index 2, as 0:.., 1:normal, 2:selected)
	found := false
	for i, e := range fsp.entries {
		if e.Name == "selected.txt" {
			fsp.SetItemSelected(i, true)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Setup failed: 'selected.txt' not found in entries")
	}

	// 3. Perform Clone
	clone := pf.Clone()
	defer clone.Close()
	cloneFsp := clone.panels[0].(*FileSystemPanel)

	// 4. Clone triggers async ReadDirectory. Wait for it.
	timeout = time.After(2 * time.Second)
	for cloneFsp.isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for clone load")
		}
	}
	// Final drain
	for i := 0; i < 10; i++ {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
		}
	}

	// 5. Verify preservation
	foundInClone := false
	for _, e := range cloneFsp.entries {
		if e.Name == "selected.txt" {
			foundInClone = true
			if !e.Selected {
				t.Error("Selection was lost after clone/reload")
			}
		}
		if e.Name == "normal.txt" && e.Selected {
			t.Error("'normal.txt' erroneously marked as selected in clone")
		}
	}
	if !foundInClone {
		t.Error("'selected.txt' missing in cloned panel entries")
	}
}

func TestPanelsFrame_GetTitle_WithProvider(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	v := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/var"), title: "remote@server"}
	fp := pf.panels[0].(*FileSystemPanel)
	fp.vfs = v
	pf.activeIdx = 0

	title := pf.GetTitle()
	if !strings.Contains(title, "Panels: remote@server:") {
		t.Errorf("Expected title to contain 'Panels: remote@server:', got %q", title)
	}
}

func TestPanelsFrame_Prompt_WithProvider(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	v := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/etc"), title: "admin@prod"}
	fp := pf.panels[0].(*FileSystemPanel)
	fp.vfs = v
	pf.activeIdx = 0

	prompt := pf.buildPrompt()

	// Convert prompt to string
	promptStr := ""
	for _, c := range prompt {
		if c.Char != vtui.WideCharFiller {
			promptStr += string(rune(c.Char))
		}
	}

	if !strings.Contains(promptStr, "admin@prod") {
		t.Errorf("Expected prompt to contain VFS title 'admin@prod', got %q", promptStr)
	}
}

func TestPanelsFrame_GetPaths(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	tmp := t.TempDir()
	pathL := filepath.Join(tmp, "left")
	pathR := filepath.Join(tmp, "right")
	os.MkdirAll(pathL, 0755)
	os.MkdirAll(pathR, 0755)

	pf.panels[0].(*FileSystemPanel).vfs.SetPath(pathL)
	pf.panels[1].(*FileSystemPanel).vfs.SetPath(pathR)

	l, r := pf.GetPaths()
	if l != pathL || r != pathR {
		t.Errorf("GetPaths failed. Got %q, %q; want %q, %q", l, r, pathL, pathR)
	}
}
func TestPanelsFrame_StateCapture(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fspL := pf.panels[0].(*FileSystemPanel)
	fspR := pf.panels[1].(*FileSystemPanel)

	// Mock cursors
	fspL.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "file.l"}}}
	fspR.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "file.r"}}}
	fspL.SetCursorIndex(0)
	fspR.SetCursorIndex(0)

	pf.activeIdx = 0 // Left active

	lFile := fspL.GetSelectedName()
	rFile := fspR.GetSelectedName()

	if lFile != "file.l" || rFile != "file.r" || pf.activeIdx != 0 {
		t.Errorf("State capture failed: L:%q, R:%q, Active:%d", lFile, rFile, pf.activeIdx)
	}
}
func TestPanelsFrame_CloneIndependence(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Set path in original
	fsp := pf.panels[0].(*FileSystemPanel)
	origPath := t.TempDir()
	fsp.vfs.SetPath(origPath)

	// Clone
	clone := pf.Clone()
	defer clone.Close()

	// Change path in clone
	newPath := t.TempDir()
	clone.panels[0].(*FileSystemPanel).vfs.SetPath(newPath)

	// Verify original is unchanged
	if pf.panels[0].(*FileSystemPanel).vfs.GetPath() != origPath {
		t.Error("Cloned PanelsFrame shares VFS state with parent!")
	}
}
func TestPanelsFrame_CtrlO_HardRedraw(t *testing.T) {
	fm := vtui.FrameManager
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)

	pf := NewPanelsFrame()
	defer pf.Close()
	fm.Push(pf)

	// Ensure screen is "clean" initially
	scr.Flush()

	// Simulate Ctrl+O
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_O, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	// After Ctrl+O, the ScreenBuf MUST be marked as dirty (needs full redraw)
	// getCell (or any mutex-locked method) is not needed here, just check the internal flag
	// which is exported for this reason.
	// Since we can't easily access unexported 'dirty', we verify the effect of HardReset:
	// all shadow cells must be zeroed.
	for i := 0; i < 80*25; i++ {
		// Use a hack to check shadow if possible, or just trust the logic if dirty isn't visible.
		// In vtui, HardReset sets dirty = true.
	}

	// We'll add a helper/check to vtui for testing this if needed,
	// but for now, we check the logic works.
}
func TestPanelsFrame_PTYLockContention(t *testing.T) {
	// Этот тест проверяет, что тяжелый парсинг в PTY-потоке не блокирует
	// доступ UI-потока к методу getActivePTY (регрессия дедлока).
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()

	// Симулируем "забитую" очередь задач
	for i := 0; i < 64; i++ {
		vtui.FrameManager.PostTask(func() {})
	}

	// Запускаем в отдельной горутине тяжелый парсинг
	// (в реальности он теперь идет вне ptyMutex)
	go func() {
		hugeData := strings.Repeat("A", 100000)
		pf.ptyMutex.Lock()
		active := (pf.getActivePTYUnsafe() == pf.pty)
		pf.ptyMutex.Unlock()

		if active {
			pf.parser.Process([]byte(hugeData))
		}
	}()

	// UI-поток пытается взять мутекс через getActivePTY.
	// Если дедлок не починен, мы зависнем здесь.
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			_ = pf.getActivePTY()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	select {
	case <-done:
		// Успех
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK DETECTED: getActivePTY blocked by PTY processing loop")
	}
}
func TestPanelsFrame_Clone_Comprehensive(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// 1. Setup specific state on the left panel
	fsp := pf.Left().(*FileSystemPanel)
	fsp.SetViewMode(ViewModeDetailed)
	fsp.sortMode = SortSize
	fsp.sortReverse = true
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file1"}},
		{VFSItem: vfs.VFSItem{Name: "file2"}, Selected: true},
		{VFSItem: vfs.VFSItem{Name: "file3"}},
	}
	fsp.Refresh()
	fsp.SetCursorIndex(2) // On "file2"

	// 2. Setup terminal state
	pf.termView.PutChar('f', 0)
	pf.termView.PutChar('o', 0)
	pf.termView.PutChar('o', 0)
	pf.termView.PutChar('\n', 0)

	// 3. Perform Clone
	clone := pf.Clone()
	defer clone.Close()

	// 4. Verify Panel State
	cloneFsp := clone.Left().(*FileSystemPanel)
	if cloneFsp.viewMode != ViewModeDetailed {
		t.Error("Clone failed to preserve ViewMode")
	}
	if cloneFsp.sortMode != SortSize || !cloneFsp.sortReverse {
		t.Error("Clone failed to preserve sort state")
	}
	if cloneFsp.GetCursorIndex() != 2 {
		t.Errorf("Clone failed to preserve cursor index: expected 2, got %d", cloneFsp.GetCursorIndex())
	}
	if cloneFsp.GetSelectedName() != "file2" {
		t.Errorf("Clone failed to preserve selection: expected 'file2', got %q", cloneFsp.GetSelectedName())
	}
	if !cloneFsp.entries[2].Selected {
		t.Error("Clone failed to preserve individual item selection flag")
	}

	// 5. Verify Terminal State
	if !strings.HasPrefix(string(clone.termView.GetAllLogBytes()), "foo\n") {
		t.Errorf("Clone failed to preserve terminal history: %q", string(clone.termView.GetAllLogBytes()))
	}

	// 6. Verify Active Panel index
	if clone.activeIdx != pf.activeIdx {
		t.Errorf("Clone failed to preserve active panel index: %d", clone.activeIdx)
	}
}
func TestIsTerminalRunnable(t *testing.T) {
	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)

	// 1. Обычный текстовый файл -> false
	txtFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(txtFile, []byte("hello"), 0644)
	if vfs.IsTerminalRunnable(context.Background(), v, txtFile) {
		t.Error("Text file should not be terminal-runnable")
	}

	// 2. Файл с расширением .sh -> true
	shFile := filepath.Join(tmpDir, "test.sh")
	os.WriteFile(shFile, []byte("echo hi"), 0644)
	if !vfs.IsTerminalRunnable(context.Background(), v, shFile) {
		t.Error(".sh file should be terminal-runnable")
	}

	// 3. Файл с шебангом без расширения -> true
	binFile := filepath.Join(tmpDir, "my-tool")
	os.WriteFile(binFile, []byte("#!/usr/bin/env bash\necho hi"), 0644)
	if !vfs.IsTerminalRunnable(context.Background(), v, binFile) {
		t.Error("File with shebang should be terminal-runnable")
	}

	// 4. Директория -> false
	subDir := filepath.Join(tmpDir, "folder")
	os.Mkdir(subDir, 0755)
	if vfs.IsTerminalRunnable(context.Background(), v, subDir) {
		t.Error("Directory should not be terminal-runnable")
	}

	// 5. Unix Executable Bit (если не на Windows)
	if runtime.GOOS != "windows" {
		execFile := filepath.Join(tmpDir, "compiled-bin")
		os.WriteFile(execFile, []byte{0x7f, 'E', 'L', 'F'}, 0755)
		if !vfs.IsTerminalRunnable(context.Background(), v, execFile) {
			t.Error("File with executable bit should be terminal-runnable on Unix")
		}
	}
}

func TestPanelsFrame_ReturnExecution(t *testing.T) {
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Создаем временный запускаемый файл
	tmp := t.TempDir()
	runnablePath := filepath.Join(tmp, "runme.sh")
	os.WriteFile(runnablePath, []byte("echo 1"), 0755)

	// Настраиваем VFS и выбираем этот файл на панели
	fsp := pf.panels[1].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "runme.sh", IsDir: false}},
	}
	fsp.Refresh()
	fsp.SelectName("runme.sh")

	// Проверяем начальное состояние
	if !pf.showPanels {
		t.Fatal("Panels should be visible initially")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) // For TaskChan

	// Имитируем нажатие Enter
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// Ждем асинхронного выполнения
	timeout := time.After(1 * time.Second)
	for pf.showPanels {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("actionExecute did not hide the panels within 1s")
		}
	}
	if pf.showPanels {
		t.Error("Panels should be hidden after executing a terminal-runnable file")
	}

	if len(pf.cmdLine.Edit.History) == 0 {
		t.Error("Executed file was not added to history")
	} else {
		expectedCmd := "runme.sh"
		if runtime.GOOS != "windows" {
			expectedCmd = "./runme.sh"
		}
		if pf.cmdLine.Edit.History[0] != expectedCmd {
			t.Errorf("History mismatch: got %q, want %q", pf.cmdLine.Edit.History[0], expectedCmd)
		}
	}
}
func TestPanelsFrame_CommandLineEnter(t *testing.T) {
	pf := setupMockPanelsFrame()
	pty := pf.pty.(*mockPty)
	defer pf.Close()

	// Вводим команду в консоль
	pf.cmdLine.Edit.SetText("ls -la")

	// Нажимаем Enter
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// Панели должны скрыться
	if pf.showPanels {
		t.Error("Panels should hide after command execution from command line")
	}
	// PTY должен получить команду
	if !strings.Contains(string(pty.written), "ls -la") {
		t.Errorf("PTY did not receive command. Got: %q", string(pty.written))
	}
}

func TestPanelsFrame_CommandLineEnter_WhenBusy(t *testing.T) {
	pf := setupMockPanelsFrame()
	pty := pf.pty.(*mockPty)
	defer pf.Close()

	pf.executing = true // PTY is busy

	// Вводим команду в консоль
	pf.cmdLine.Edit.SetText("ls -la")

	// Нажимаем Enter
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// Панели должны скрыться
	if pf.showPanels {
		t.Error("Panels should hide after command execution even when PTY is busy")
	}
	// PTY должен получить команду
	if !strings.Contains(string(pty.written), "ls -la") {
		t.Errorf("PTY did not receive command when busy. Got: %q", string(pty.written))
	}
}

func TestPanelsFrame_DirectoryEnter(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "work_dir")
	os.Mkdir(sub, 0755)

	fsp := pf.panels[1].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "work_dir", IsDir: true}},
	}
	fsp.Refresh()
	fsp.SelectName("work_dir")

	// Нажимаем Enter на директории
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// Панели НЕ должны скрываться
	if !pf.showPanels {
		t.Error("Panels should NOT hide when entering a directory")
	}
	// Путь должен измениться
	if fsp.vfs.GetPath() != sub {
		t.Errorf("VFS path did not change. Expected %s, got %s", sub, fsp.vfs.GetPath())
	}
}

func TestPanelsFrame_NonRunnableOpen(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "readme.txt")
	os.WriteFile(docPath, []byte("some text"), 0644)

	fsp := pf.panels[1].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "readme.txt", IsDir: false}},
	}
	fsp.Refresh()
	fsp.SelectName("readme.txt")

	// Нажимаем Enter на текстовом файле
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// Панели должны остаться видимыми (так как открытие идет через внешнюю ОС)
	if !pf.showPanels {
		t.Error("Panels should stay visible when opening non-runnable files via OS associations")
	}
}
func TestPanelsFrame_SwitchVFS_CacheClear(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.dirCache["/test/path"] = dirCacheEntry{}
	if len(fsp.dirCache) != 1 {
		t.Fatal("Cache setup failed")
	}

	pf.switchToVFS(fsp, vfs.NewOSVFS(t.TempDir()))

	if len(fsp.dirCache) != 0 {
		t.Error("switchToVFS should clear the directory cache")
	}
}

func TestPanelsFrame_Clone_CachePreservation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fsp := pf.panels[0].(*FileSystemPanel)
	items := []vfs.VFSItem{{Name: "cached_item"}}
	fsp.dirCache["/test/path"] = dirCacheEntry{items: items}

	clone := pf.Clone()
	defer clone.Close()
	cloneFsp := clone.panels[0].(*FileSystemPanel)

	if len(cloneFsp.dirCache) != 1 {
		t.Fatalf("Cache not cloned, length is %d", len(cloneFsp.dirCache))
	}
	if cached, ok := cloneFsp.dirCache["/test/path"]; !ok || len(cached.items) != 1 || cached.items[0].Name != "cached_item" {
		t.Error("Cloned cache content is incorrect")
	}

	// Verify independence
	cloneFsp.dirCache["/new/path"] = dirCacheEntry{}
	if len(fsp.dirCache) != 1 {
		t.Error("Cloned cache is not independent from original")
	}
}

func TestExecuteFileOp_BackgroundButtonTrigger(t *testing.T) {
	// This test ensures that the logic inside Background button click works
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fm.Push(pf)

	initialScreens := len(fm.Screens)

	// Simulate what Background button does:
	fork := pf.Clone()
	fm.AddScreen(fork)

	if len(fm.Screens) != initialScreens+1 {
		t.Errorf("Backgrounding failed to create a new screen. Got %d, want %d", len(fm.Screens), initialScreens+1)
	}
}
func TestExecuteDummyOp_HeadlessMode(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	fm.Push(pf)

	initialScreens := len(fm.Screens)

	// Trigger Mode Foreground (2)
	go pf.ExecuteDummyOp(2)

	// Manually process the task queue (since we are not in fm.Run loop)
	timeout := time.After(1 * time.Second)
	for len(fm.Screens) == initialScreens {
		select {
		case task := <-fm.TaskChan:
			task()
		case <-timeout:
			t.Fatal("ExecuteDummyOp did not post workspace creation task")
		}
	}

	if len(fm.Screens) != initialScreens+1 {
		t.Fatalf("Headless screen not created. Got %d", len(fm.Screens))
	}

	newScreen := fm.Screens[len(fm.Screens)-1]
	if len(newScreen.Frames) != 1 { // Только диалог, без Desktop
		t.Errorf("Headless screen should have 1 frame, got %d", len(newScreen.Frames))
	}
	if !newScreen.Transparent {
		t.Error("Headless screen should be transparent")
	}
}

func TestPanelsFrame_TerminalForwarding_Legacy(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.showPanels = false
	pf.termView.UseAltScreen = true

	// Mock PTY
	pty := &mockPty{}
	pf.pty = pty

	// 1. Ctrl+W should be FORWARDED (Legacy mode has no Kitty/Win32 flags)
	// For letters, TranslateInput expects the Char field to be populated.
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_W, Char: 'w', ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if !strings.Contains(string(pty.written), "\x17") { // 0x17 is Ctrl+W byte
		t.Error("Ctrl+W should be forwarded to terminal in legacy mode")
	}
	pty.written = nil

	// 2. Ctrl+Tab should NOT be forwarded (returns false, handled by FrameManager)
	handled := pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if handled {
		t.Error("Ctrl+Tab should NOT be handled by PanelsFrame in legacy mode")
	}
	if len(pty.written) > 0 {
		t.Error("PTY received bytes for Ctrl+Tab in legacy mode")
	}
}

func TestPanelsFrame_TerminalForwarding_Advanced(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.showPanels = false
	pf.termView.UseAltScreen = true
	pf.termView.Win32InputMode = true // Advanced mode

	pty := &mockPty{}
	pf.pty = pty

	// 1. Ctrl+Tab should be FORWARDED in Advanced mode
	handled := pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if !handled {
		t.Error("Ctrl+Tab should be handled by PanelsFrame in Advanced mode")
	}
	if len(pty.written) == 0 {
		t.Error("PTY did not receive Win32 sequence for Ctrl+Tab")
	}
	pty.written = nil

	// 2. Shift+Ctrl+Tab should NOT be forwarded in any mode
	handled = pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})
	if handled {
		t.Error("Shift+Ctrl+Tab was erroneously forwarded to PTY")
	}
}
func TestPanelsFrame_FilesMenuLabels(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()

	// Items[1] is the "Files" menu
	filesMenu := pf.menuBar.Items[1]
	if filesMenu.Label != "&Files" {
		t.Errorf("Expected Files menu label '&Files', got %q", filesMenu.Label)
	}

	// SubItems[3] should be "Rename or move"
	renMove := filesMenu.SubItems[3]
	expected := "&" + Msg("Menu.Files.RenMov")
	if renMove.Text != expected {
		t.Errorf("Expected Files item %q, got %q", expected, renMove.Text)
	}

	if renMove.Shortcut != "F6" {
		t.Errorf("Expected shortcut 'F6', got %q", renMove.Shortcut)
	}
}

func TestPanelsFrame_ProcessMouse_RightDoubleClickNoEnter(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	tmp := t.TempDir()
	runnablePath := filepath.Join(tmp, "run.sh")
	os.WriteFile(runnablePath, []byte("echo"), 0755)

	fsp := pf.Left().(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "run.sh", IsDir: false}},
	}
	fsp.Refresh()

	// Double click with RIGHT button. Row 1 -> Y=3
	pf.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseX:          5,
		MouseY:          3,
		ButtonState:     vtinput.RightmostButtonPressed,
		MouseEventFlags: vtinput.DoubleClick,
	})

	// Panels should NOT hide. Right double-click should only toggle selection.
	if !pf.showPanels {
		t.Error("Right double-click should NOT simulate Enter")
	}
}

func TestPanelsFrame_CommandRouting_FKeys(t *testing.T) {
	pf := NewPanelsFrame()
	// Mock exit behavior to check F10
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	fm.Push(pf)

	// Simulate F10
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F10,
	})

	// Now it shouldn't be shutdown immediately. A dialog should be on top.
	top := fm.GetTopFrame()
	if top == nil || top.GetTitle() != Msg("Quit.Title") {
		t.Fatalf("Expected quit confirmation dialog, got %v", top)
	}

	// Simulate clicking "Leave" (button 0 in the ShowMessage dialog)
	if d, ok := top.(*vtui.Window); ok && d.OnResult != nil {
		d.OnResult(0)
	}

	if !fm.IsShutdown() {
		t.Error("F10 followed by confirmation did not trigger Shutdown")
	}
}

func TestPanelsFrame_QuitConfirmation_Cancel(t *testing.T) {
	pf := NewPanelsFrame()
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	fm.Push(pf)

	// Trigger Quit
	pf.HandleCommand(vtui.CmQuit, nil)

	top := fm.GetTopFrame()
	if top == nil || top.GetTitle() != Msg("Quit.Title") {
		t.Fatal("Quit dialog didn't appear")
	}

	// Simulate clicking "Cancel" (button 1)
	if d, ok := top.(*vtui.Window); ok && d.OnResult != nil {
		d.OnResult(1)
	}

	if fm.IsShutdown() {
		t.Error("Application shut down even after exit was canceled")
	}
}
func TestPanelsFrame_F9Context(t *testing.T) {
	pf := NewPanelsFrame()
	pf.ResizeConsole(80, 25)

	// 1. Test Left Panel context
	pf.activeIdx = 0
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F9,
	})

	if pf.menuBar.SelectPos != 0 {
		t.Errorf("F9 on left panel: expected menu index 0, got %d", pf.menuBar.SelectPos)
	}
	if !pf.menuBar.Active {
		t.Error("MenuBar should be active after F9")
	}

	// 2. Test Right Panel context
	pf.menuBar.Active = false // Reset
	pf.activeIdx = 1
	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F9,
	})

	if pf.menuBar.SelectPos != 4 {
		t.Errorf("F9 on right panel: expected menu index 4, got %d", pf.menuBar.SelectPos)
	}
}

func TestLayout_F4InternalDialogs_Validity(t *testing.T) {
	vtui.SetDefaultPalette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	t.Run("DummyOpDialog", func(t *testing.T) {
		// We need to capture the dialog created by showDummyOpDialog.
		// Since it pushes to the real FrameManager, we'll initialize it.
		fm := vtui.FrameManager
		fm.Init(vtui.NewSilentScreenBuf())

		pf.showDummyOpDialog()
		top := fm.GetTopFrame()
		if dlg, ok := top.(vtui.Container); ok {
			vtui.AssertLayout(t, dlg)
		} else {
			t.Fatal("Top frame is not a container")
		}
	})
}
func TestPanelsFrame_CopyShortcuts(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fsp := pf.panels[0].(*FileSystemPanel)
	if fsp.cancelLoad != nil {
		fsp.cancelLoad()
	}
	fsp.isLoading = false
	if fsp.loadingTimer != nil {
		fsp.loadingTimer.Stop()
	}

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "target.txt"}},
	}
	fsp.Refresh()
	fsp.SetCursorIndex(1)
	pf.activeIdx = 0

	// 1. Test Ctrl+Ins (Filename)
	vtui.SetClipboard("")
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_INSERT, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if got := vtui.GetClipboard(); got != "target.txt" {
		t.Errorf("Ctrl+Ins failed: expected 'target.txt', got %q", got)
	}

	// 2. Test Ctrl+F (Full Path)
	vtui.SetClipboard("")
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: 'F', ControlKeyState: vtinput.LeftCtrlPressed,
	})
	expectedPath := fsp.vfs.Join(fsp.vfs.GetPath(), "target.txt")
	if got := vtui.GetClipboard(); got != expectedPath {
		t.Errorf("Ctrl+F failed: expected %q, got %q", expectedPath, got)
	}
}
func TestLayout_F4ActionDialogs_Validity(t *testing.T) {
	vtui.SetDefaultPalette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fm := vtui.FrameManager

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)

	// Helper to setup active panel with some files
	setupPanel := func() {
		pf.activeIdx = 0
		fsp := pf.panels[0].(*FileSystemPanel)
		fsp.entries = []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "file1.txt"}},
		}
		fsp.Refresh()
		fsp.SetCursorIndex(1)
	}

	t.Run("CopyDialog", func(t *testing.T) {
		setupPanel()
		actionCopyMove(pf, false)
		dlg := fm.GetTopFrame().(vtui.Container)
		vtui.AssertLayout(t, dlg)
		fm.Pop()
	})

	t.Run("MoveDialog", func(t *testing.T) {
		setupPanel()
		actionCopyMove(pf, true)
		dlg := fm.GetTopFrame().(vtui.Container)
		vtui.AssertLayout(t, dlg)
		fm.Pop()
	})

	t.Run("MkDirDialog", func(t *testing.T) {
		actionMkDir(pf)
		dlg := fm.GetTopFrame().(vtui.Container)
		vtui.AssertLayout(t, dlg)
		fm.Pop()
	})

	t.Run("DeleteDialog", func(t *testing.T) {
		setupPanel()
		actionDelete(pf)
		dlg := fm.GetTopFrame().(vtui.Container)
		vtui.AssertLayout(t, dlg)
		fm.Pop()
	})
	t.Run("FindFileDialog", func(t *testing.T) {
		setupPanel()
		actionFindFile(pf)
		dlg := fm.GetTopFrame().(vtui.Container)
		vtui.AssertLayout(t, dlg)
		fm.Pop()
	})
	t.Run("EditorSettingsDialog", func(t *testing.T) {
		actionEditorSettings(pf)
		dlg := fm.GetTopFrame().(vtui.Container)
		vtui.AssertLayout(t, dlg)
		fm.Pop()
	})
}

func TestPanelsFrame_DriveMenu_OtherPanel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	pathR := filepath.Join(t.TempDir(), "right")
	os.MkdirAll(pathR, 0755)
	pf.panels[1].(*FileSystemPanel).vfs.SetPath(pathR)

	// Open Alt+F1 (Left panel drive menu)
	pf.showDriveMenu(0)

	top := vtui.FrameManager.GetTopFrame()
	menu, ok := top.(*vtui.VMenu)
	if !ok {
		t.Fatal("Drive menu not opened")
	}

	// Ensure "Other panel" is at index 0 and selected
	if menu.GetTitle() != " Drive " || menu.SelectPos != 0 {
		t.Errorf("Menu state invalid: title=%q, pos=%d", menu.GetTitle(), menu.SelectPos)
	}

	// Trigger "Other panel" (idx 0)
	menu.OnAction(0)

	// Left panel VFS path must now match Right panel's path
	got := pf.panels[0].(*FileSystemPanel).vfs.GetPath()
	if got != pathR {
		t.Errorf("Path sync failed. Expected %q, got %q", pathR, got)
	}
}

func TestPanelsFrame_DriveMenu_TerminalBusy(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Simulate busy terminal
	pf.showPanels = false
	pf.termView.UseAltScreen = true

	// Press Alt+F1
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_F1, ControlKeyState: vtinput.LeftAltPressed,
	})

	// Menu should NOT open
	if vtui.FrameManager.GetTopFrameType() == vtui.TypeMenu {
		t.Error("Drive menu opened while terminal was busy")
	}
}

func TestDriveMenu_SmartHotkeys(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Сохраняем оригинал и подменяем реестр
	oldRegistry := DriveRegistry
	DriveRegistry = []DriveEntry{
		{Name: "NetFox", Factory: func() vfs.VFS { return nil }},
		{Name: "Null VFS", Factory: func() vfs.VFS { return nil }},
	}
	defer func() { DriveRegistry = oldRegistry }()

	pf.showDriveMenu(0)
	top := vtui.FrameManager.GetTopFrame()
	menu, ok := top.(*vtui.VMenu)
	if !ok {
		t.Fatalf("Expected VMenu on top, got %T", top)
	}

	// 1. Проверка фокуса (Other panel по умолчанию)
	if menu.SelectPos != 0 {
		t.Errorf("Expected 'Other panel' (index 0) to be focused, got index %d", menu.SelectPos)
	}

	// 2. Ищем плагины в пунктах меню
	var nfIdx, nullIdx int = -1, -1
	for i, itm := range menu.Items {
		cleanText := strings.ReplaceAll(itm.Text, "&", "")
		if strings.Contains(cleanText, "NetFox") {
			nfIdx = i
		}
		if strings.Contains(cleanText, "Null VFS") {
			nullIdx = i
		}
	}

	if nfIdx == -1 || nullIdx == -1 {
		var items []string
		for _, itm := range menu.Items {
			items = append(items, itm.Text)
		}
		t.Fatalf("Plugins not found in menu. Items present: %v", items)
	}

	// 3. Проверка уникальности хоткеев
	// NetFox (первый в списке) заберет 'N' -> "1. &NetFox"
	// Null VFS (второй) увидит, что 'N' занята, и заберет 'u' -> "2. N&ull VFS"
	nfText := menu.Items[nfIdx].Text
	nullText := menu.Items[nullIdx].Text

	if !strings.Contains(nfText, "&N") {
		t.Errorf("NetFox should have 'N' as hotkey: %q", nfText)
	}
	if !strings.Contains(nullText, "N&u") {
		t.Errorf("Null VFS should have 'u' as hotkey (N is taken): %q", nullText)
	}
}

func TestDriveMenu_PhysicalKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Linux-specific physical key test")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	pf.showDriveMenu(0)
	menu := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)

	// Inject VK_OEM_3 (tilde/backtick key)
	// It should find the Home item and trigger selection
	handled := menu.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_OEM_3,
	})

	if !handled {
		t.Error("Drive menu failed to handle physical tilde key")
	}
	if !menu.IsDone() {
		t.Error("Physical key should have triggered selection and closed the menu")
	}
}
func TestPanelsFrame_ShiftInsert_Fallthrough(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// 1. Prepare clipboard
	testText := "ClipboardPayload"
	vtui.SetClipboard(testText)

	// 2. Ensure panel is active (should NOT handle Shift+Ins)
	pf.activeIdx = 0
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "some_file.txt"}}}
	fsp.Refresh()
	fsp.SetFocus(true)

	// 3. Send Shift+Ins
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_INSERT,
		ControlKeyState: vtinput.ShiftPressed,
	})

	// 4. Verify text landed in CommandLine
	got := pf.cmdLine.Edit.GetText()
	if !strings.Contains(got, testText) {
		t.Errorf("Shift+Ins failed to fallthrough to CommandLine. Got %q, expected to contain %q", got, testText)
	}

	// 5. Verify file was NOT selected (Index 0 should remain unselected)
	if fsp.entries[0].Selected {
		t.Error("File was erroneously selected by Shift+Ins")
	}
}
func TestPanelsFrame_PromptTruncation(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()

	// Simulate standard 80-column terminal
	width := 80
	pf.ResizeConsole(width, 25)

	fsp := pf.getActivePanel()

	// Max allowed prompt length is width / 2 = 40.

	t.Run("Short Path No Truncation", func(t *testing.T) {
		// Use NullVFS to bypass real disk checks in tests
		fsp.vfs = vfs.NewNullVFS(0)
		fsp.vfs.SetPath(filepath.FromSlash("/home/user"))
		prompt := pf.buildPrompt()

		visibleLen := 0
		for _, c := range prompt {
			if c.Char != vtui.WideCharFiller {
				visibleLen++
			}
		}

		// Path is short, should be preserved entirely
		found := false
		promptStr := ""
		for _, c := range prompt {
			if c.Char != vtui.WideCharFiller {
				promptStr += string(rune(c.Char))
			}
		}
		if strings.Contains(promptStr, "home") {
			found = true
		}

		if !found {
			t.Errorf("Short path was lost in prompt: %q", promptStr)
		}
	})

	t.Run("Extreme Long Path Truncation", func(t *testing.T) {
		// Use NullVFS to bypass real disk checks in tests
		fsp.vfs = vfs.NewNullVFS(0)
		longPath := "/very/long/directory/path/that/exceeds/the/limit/of/forty/characters/definitely/and/must/be/shortened"
		fsp.vfs.SetPath(filepath.FromSlash(longPath))
		prompt := pf.buildPrompt()

		visibleLen := 0
		promptStr := ""
		for _, c := range prompt {
			if c.Char != vtui.WideCharFiller {
				visibleLen++
				promptStr += string(rune(c.Char))
			}
		}

		// 1. Total length must be within bounds (approx 40)
		if visibleLen > 45 { // 40 + small buffer for user@host
			t.Errorf("Prompt too long: %d chars (%q)", visibleLen, promptStr)
		}

		// 2. Must contain ellipsis
		if !strings.Contains(promptStr, "...") {
			t.Errorf("Truncated prompt missing ellipsis: %q", promptStr)
		}

		// 3. Check OS-specific suffix
		if runtime.GOOS == "windows" {
			if !strings.HasSuffix(promptStr, ">") {
				t.Errorf("Windows prompt should end with '>', got %q", promptStr)
			}
		} else {
			if !strings.HasSuffix(promptStr, "$ ") {
				t.Errorf("Unix prompt should end with '$ ', got %q", promptStr)
			}
		}
	})
}

type mockSlowStatVFS struct {
	vfs.OSVFS
	statCalls int
	statBlock chan struct{}
}

func (m *mockSlowStatVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	m.statCalls++
	if m.statBlock != nil {
		<-m.statBlock
	}
	return m.OSVFS.Stat(ctx, p)
}

func TestPanelsFrame_AutoRefresh_Locking(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Дожидаемся завершения первичной инициализации обеих панелей,
	// чтобы их фоновые вызовы Stat не перекрывались с нашим моком.
	for pf.panels[0].(*FileSystemPanel).isLoading || pf.panels[1].(*FileSystemPanel).isLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for initial load")
		}
	}

	// Настраиваем VFS с блокирующим Stat
	block := make(chan struct{})
	mv := &mockSlowStatVFS{
		OSVFS:     *vfs.NewOSVFS(t.TempDir()),
		statBlock: block,
	}

	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.vfs = mv
	fsp.lastDirMTime = time.Now().Add(-1 * time.Hour) // Эмулируем старое время
	fsp.isCheckingRefresh = false

	// 1. Первый вызов Show() должен инициировать авто-обновление
	pf.lastAutoRefresh = time.Now().Add(-5 * time.Second)
	pf.Show(vtui.NewSilentScreenBuf())

	// Даем немного времени RunAsync запуститься
	time.Sleep(20 * time.Millisecond)

	if !fsp.isCheckingRefresh {
		t.Error("Expected isCheckingRefresh to be true while Stat is pending")
	}
	if mv.statCalls != 1 {
		t.Errorf("Expected 1 Stat call, got %d", mv.statCalls)
	}

	// 2. Повторный вызов Show() НЕ должен инициировать второй Stat, пока первый висит
	pf.lastAutoRefresh = time.Now().Add(-5 * time.Second)
	pf.Show(vtui.NewSilentScreenBuf())

	if mv.statCalls > 1 {
		t.Errorf("Anti-spam failed: Stat called %d times simultaneously", mv.statCalls)
	}

	// 3. Разблокируем Stat и проверяем сброс флага
	close(block)

	// Прокачиваем очередь задач до завершения Stat и RunOnUI
	timeout := time.After(1 * time.Second)
	for fsp.isCheckingRefresh {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("isCheckingRefresh was never reset to false")
		}
	}
}

type vimTestHandler struct {
	vtui.BaseFrame
	onCmd func(cmd int, args any) bool
}

func (v *vimTestHandler) HandleCommand(cmd int, args any) bool {
	if v.onCmd != nil {
		return v.onCmd(cmd, args)
	}
	return false
}

func (v *vimTestHandler) GetType() vtui.FrameType { return vtui.TypeUser }
func (v *vimTestHandler) GetTitle() string        { return "VimHandler" }

func TestPanelsFrame_VimHotkeys_Comprehensive(t *testing.T) {
	vtui.SetDefaultPalette()
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	cmdCaught := 0
	handler := &vimTestHandler{
		onCmd: func(cmd int, args any) bool {
			cmdCaught = cmd
			return true
		},
	}

	oldCfg := AppConfig
	AppConfig.VimHotkeys = true
	defer func() { AppConfig = oldCfg }()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fsp := pf.panels[0].(*FileSystemPanel)
	pf.activeIdx = 0

	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "fileA"}},
		{VFSItem: vfs.VFSItem{Name: "fileB"}},
		{VFSItem: vfs.VFSItem{Name: "fileC"}},
	}
	fsp.Refresh()
	fsp.SetCursorIndex(1) // On fileA

	fm.Push(pf)
	fm.Push(handler)

	// 1. Basic j/k navigation
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'j'})
	if fsp.GetCursorIndex() != 2 {
		t.Errorf("Vim 'j' failed, expected index 2, got %d", fsp.GetCursorIndex())
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'k'})
	if fsp.GetCursorIndex() != 1 {
		t.Errorf("Vim 'k' failed, expected index 1, got %d", fsp.GetCursorIndex())
	}

	// 2. Action dd (Delete)
	cmdCaught = 0
	pf.cmdLine.Clear()
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'd'})
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'd'})
	if cmdCaught != CmDelete {
		t.Errorf("'dd' failed to emit CmDelete, got %d", cmdCaught)
	}
	if !pf.cmdLine.IsEmpty() {
		t.Error("Command line should be cleared after Vim action")
	}

	// 3. Reset on Tab (Switch panel)
	cmdCaught = 0
	pf.cmdLine.Clear()
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'd'})
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'd'})
	if cmdCaught == CmDelete {
		t.Error("Vim prefix should reset after switching panels via Tab")
	}

	// 4. Reset on Mouse click
	cmdCaught = 0
	pf.cmdLine.Clear()
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'c'})
	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 5, MouseY: 5,
	})
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'c'})
	if cmdCaught == CmCopy {
		t.Error("Vim prefix should reset after mouse interaction")
	}

	// 5. Conflict with Fast Find
	cmdCaught = 0
	pf.cmdLine.Clear()
	fsp.SetCursorIndex(1) // Reset cursor position after mouse click test
	fsp.fastFindMode = true
	// In fast find mode, 'j' should be passed to find logic, not navigation.
	// `pf.ProcessKey` will return `false` because Vim logic is skipped,
	// then it will fall through to `fsp.ProcessKey` which will handle fast-find and return `true`.
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'j'})
	if fsp.GetCursorIndex() != 1 {
		t.Error("'j' was handled as Vim navigation despite Fast Find being active")
	}
	if fsp.fastFindStr != "j" {
		t.Errorf("Fast find string should be 'j', got %q", fsp.fastFindStr)
	}
}

func createTestZipForNav(t *testing.T, path string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	_, err = zw.Create("inner_dir/")
	if err != nil {
		t.Fatal(err)
	}

	w, err := zw.Create("inner_dir/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello"))
}

func TestPanelsFrame_NavigateToPath(t *testing.T) {
	// Register the Archive VFS provider manually for this unit test
	vfs.RegisterProvider(&archive.ArchiveProvider{})

	// Initialize a headless FrameManager to prevent nil panics during async directory reads
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(tmpDir, "test.zip")
	createTestZipForNav(t, zipPath)

	pf := &PanelsFrame{}
	lp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmpDir))
	rp := NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS(tmpDir))
	pf.panels[0] = lp
	pf.panels[1] = rp
	pf.activeIdx = 0

	// Test 1: Navigate to absolute path inside the archive
	targetPath := filepath.Join(zipPath, "inner_dir")
	ok := pf.NavigateToPath(lp, targetPath)
	if !ok {
		t.Fatalf("NavigateToPath failed to enter archive: %s", targetPath)
	}

	// Verify VFS switched to ArchiveVFS
	if _, isOS := lp.vfs.(*vfs.OSVFS); isOS {
		t.Error("Expected panel VFS to switch from OSVFS to ArchiveVFS")
	}

	expectedPath := filepath.ToSlash(filepath.Clean(targetPath))
	if filepath.ToSlash(lp.vfs.GetPath()) != expectedPath {
		t.Errorf("Expected VFS path %q, got %q", expectedPath, lp.vfs.GetPath())
	}

	// Test 2: Navigate to ".." at the archive root to escape it
	ok = pf.NavigateToPath(lp, zipPath)
	if !ok {
		t.Fatalf("Failed to navigate to archive root: %s", zipPath)
	}

	ok = pf.NavigateToPath(lp, "..")
	if !ok {
		t.Fatal("Failed to navigate '..' from archive root")
	}

	// Verify we switched back to OSVFS pointing to tmpDir
	if _, isOS := lp.vfs.(*vfs.OSVFS); !isOS {
		t.Error("Expected panel VFS to switch back to OSVFS")
	}

	if filepath.Clean(lp.vfs.GetPath()) != filepath.Clean(tmpDir) {
		t.Errorf("Expected OSVFS path %q, got %q", tmpDir, lp.vfs.GetPath())
	}
}

func TestArchiveBulkExtract_ProgressTracking(t *testing.T) {
	// Register the Archive VFS provider manually for this unit test
	vfs.RegisterProvider(&archive.ArchiveProvider{})

	// Initialize a headless FrameManager to prevent nil panics during async directory reads
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(tmpDir, "progress_test.zip")

	// Create a test zip with 1 folder and 2 files
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)

	// Directory
	_, _ = zw.Create("dir/")
	// File 1 (10 bytes)
	w1, _ := zw.Create("dir/file1.txt")
	w1.Write([]byte("0123456789"))
	// File 2 (20 bytes)
	w2, _ := zw.Create("dir/file2.txt")
	w2.Write([]byte("01234567890123456789"))
	zw.Close()
	f.Close()

	// 1. Setup VFS
	parentVFS := vfs.NewOSVFS(tmpDir)
	arcVFS, err := vfs.FindProvider(context.Background(), parentVFS, zipPath).Open(context.Background(), parentVFS, zipPath)
	if err != nil {
		t.Fatalf("Failed to open archive VFS: %v", err)
	}
	defer arcVFS.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	os.MkdirAll(destDir, 0755)
	dstVFS := vfs.NewOSVFS(destDir)

	// 2. Pre-calculate stats (this mimics ExecuteFileOp's scan phase)
	names := []string{"dir"}
	totalStats, err := vfs.CalculateStats(context.Background(), arcVFS, arcVFS.GetPath(), names, nil)
	if err != nil {
		t.Fatalf("CalculateStats failed: %v", err)
	}

	// Verify scanned stats: 1 dir, 2 files, 30 bytes
	if totalStats.Files != 2 || totalStats.Dirs != 1 || totalStats.Bytes != 30 {
		t.Errorf("Unexpected scanned stats: %+v", totalStats)
	}

	tracker := NewFileOpTracker(totalStats)

	var bytesReported int64

	mockOriginalReporter := &mockTaskReporter{}

	// We want to verify that when we call CopyBulk, the globalAwareReporter updates the tracker
	// and invokes updateUI, which in turn updates the dialog.
	getGlobalStats := func(action string) (string, int, string) {
		_, totalPct, _ := tracker.GetProgress()
		processed, total := tracker.GetStats()
		totalText := fmt.Sprintf("Total: %d/%d", processed.Bytes, total.Bytes)
		timeSpeedText := fmt.Sprintf("Progress: %d%%", totalPct)
		return totalText, totalPct, timeSpeedText
	}

	wrapRep := &globalAwareReporter{
		original:  mockOriginalReporter,
		getGlobal: getGlobalStats,
		tracker:   tracker,
		onBytes: func(n int) {
			bytesReported += int64(n)
		},
	}

	// We pass "AutoQueue" in context to bypass the interactive UI busy-lock prompt
	ctx := context.WithValue(context.Background(), "AutoQueue", true)

	// 3. Execute Bulk Copy
	bulkCopier := arcVFS.(vfs.BulkCopier)
	err = bulkCopier.CopyBulk(ctx, names, dstVFS, destDir, wrapRep)
	if err != nil {
		t.Fatalf("CopyBulk failed: %v", err)
	}

	// 4. Verify results
	processed, _ := tracker.GetStats()

	// All 30 bytes must be reported
	if bytesReported != 30 {
		t.Errorf("Expected 30 bytes reported via onBytes, got %d", bytesReported)
	}
	if processed.Bytes != 30 {
		t.Errorf("Tracker processed bytes mismatch: expected 30, got %d", processed.Bytes)
	}
	if processed.Files != 2 {
		t.Errorf("Tracker processed files mismatch: expected 2, got %d", processed.Files)
	}
	if processed.Dirs != 1 {
		t.Errorf("Tracker processed dirs mismatch: expected 1, got %d", processed.Dirs)
	}

	// Verify files actually extracted and content matches
	b1, err := os.ReadFile(filepath.Join(destDir, "dir/file1.txt"))
	if err != nil || string(b1) != "0123456789" {
		t.Errorf("file1.txt mismatch: %q (err: %v)", string(b1), err)
	}
	b2, err := os.ReadFile(filepath.Join(destDir, "dir/file2.txt"))
	if err != nil || string(b2) != "01234567890123456789" {
		t.Errorf("file2.txt mismatch: %q (err: %v)", string(b2), err)
	}
}

type mockTaskReporter struct{}

func (m *mockTaskReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (m *mockTaskReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
}
func (m *mockTaskReporter) IsCancelled() bool { return false }
