package main

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestViewerEditorHistoryPersistsAndDeduplicatesByFile(t *testing.T) {
	previous := vtui.GlobalHistoryProvider
	provider := stubHistoryProvider{}
	vtui.GlobalHistoryProvider = &provider
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	rememberViewerEditorHistory(fs, first, historyModeView)
	rememberViewerEditorHistory(fs, second, historyModeEdit)
	rememberViewerEditorHistory(fs, first, historyModeEdit)

	entries := loadViewerEditorHistory()
	if len(entries) != 2 {
		t.Fatalf("history length = %d, want 2: %#v", len(entries), entries)
	}
	if entries[0].Path != first || entries[0].Mode != historyModeEdit {
		t.Fatalf("most recent entry = %#v, want edited %q", entries[0], first)
	}
	if entries[1].Path != second || entries[1].Mode != historyModeEdit {
		t.Fatalf("second entry = %#v, want edited %q", entries[1], second)
	}
}

func TestViewerEditorHistoryActionAndMenuRegistration(t *testing.T) {
	hm := NewHotkeyManager("")
	if got := hm.GetAction("Shell", "AltF11"); got != "Panel.ViewerEditorHistory" {
		t.Fatalf("AltF11 action = %q, want Panel.ViewerEditorHistory", got)
	}
	action, ok := GetAction("Panel.ViewerEditorHistory")
	if !ok || action.MenuPath != "Commands" {
		t.Fatalf("viewer/editor history action is not in Commands: %#v", action)
	}
}

func TestViewerEditorHistoryDialogDeletesEntry(t *testing.T) {
	initHistoryTestScreen(t)
	previous := vtui.GlobalHistoryProvider
	provider := stubHistoryProvider{}
	vtui.GlobalHistoryProvider = &provider
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	root := t.TempDir()
	rememberViewerEditorHistory(vfs.NewOSVFS(root), filepath.Join(root, "file.txt"), historyModeView)
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(120, 40)
	actionViewerEditorHistory(pf)
	menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok || menu.GetHelp() != "HistoryViewEdit" {
		t.Fatalf("history dialog = %T help=%q", vtui.FrameManager.GetTopFrame(), menu.GetHelp())
	}
	if activeHistorySearch == nil || activeHistorySearch.selectedSecondary() != Msg("History.Mode.View") {
		t.Fatal("viewer/editor mode column is missing")
	}

	menu.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_DELETE,
		ControlKeyState: vtinput.ShiftPressed,
	})
	if got := loadViewerEditorHistory(); len(got) != 0 {
		t.Fatalf("deleted viewer/editor history survived: %#v", got)
	}
	if !menu.IsDone() {
		t.Fatal("empty viewer/editor history menu stayed open")
	}
}

func TestViewerEditorHistoryInsertPersistsLockAndProtectsEntry(t *testing.T) {
	initHistoryTestScreen(t)
	previous := vtui.GlobalHistoryProvider
	provider := stubHistoryProvider{}
	vtui.GlobalHistoryProvider = &provider
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	root := t.TempDir()
	path := filepath.Join(root, "pinned.txt")
	fs := vfs.NewOSVFS(root)
	rememberViewerEditorHistory(fs, path, historyModeView)
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(120, 40)
	actionViewerEditorHistory(pf)
	menu := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)

	menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT})
	entries := loadViewerEditorHistory()
	if len(entries) != 1 || !entries[0].Lock {
		t.Fatalf("Insert did not persist viewer/editor lock: %#v", entries)
	}
	if activeHistorySearch.processKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3}) {
		t.Fatal("viewer/editor history F3 was intercepted by command details")
	}
	menu.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE,
		ControlKeyState: vtinput.ShiftPressed,
	})
	if entries = loadViewerEditorHistory(); len(entries) != 1 || !entries[0].Lock {
		t.Fatalf("Shift+Del removed locked viewer/editor entry: %#v", entries)
	}
	rememberViewerEditorHistory(fs, path, historyModeEdit)
	if entries = loadViewerEditorHistory(); len(entries) != 1 || !entries[0].Lock || entries[0].Mode != historyModeEdit {
		t.Fatalf("revisited viewer/editor entry lost its lock: %#v", entries)
	}
}
