package main

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPanelsFrame_CtrlEnter_Escaping(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fsp := pf.panels[0].(*FileSystemPanel)
	pf.activeIdx = 0

	// Имя файла со спецсимволами и пробелами
	complexName := "file with'quote & space.txt"
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: complexName}},
	}
	fsp.Refresh()
	fsp.SetCursorIndex(1)

	// Нажимаем Ctrl+Enter
	pressKey(pf, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	got := pf.cmdLine.Edit.GetText()

	if runtime.GOOS == "windows" {
		// На Windows ожидаем двойные кавычки
		expected := "\"" + complexName + "\""
		if got != expected {
			t.Errorf("Windows escaping failed. Got %q, want %q", got, expected)
		}
	} else {
		// На Unix ожидаем одинарные кавычки и экранирование внутренней кавычки
		expected := "'file with'\\''quote & space.txt'"
		if got != expected {
			t.Errorf("Unix escaping failed. Got %q, want %q", got, expected)
		}
	}
}

func TestPanelsFrame_CtrlEnterOnDirectoryInsertsWithoutEntering(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()

	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	fsp := pf.panels[0].(*FileSystemPanel)
	pf.activeIdx = 0
	fsp.vfs = vfs.NewOSVFS(tmp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "subdir", IsDir: true}}}
	fsp.Refresh()
	fsp.SetCursorIndex(0)

	mainCtrlEnter := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	pressKey(pf, mainCtrlEnter)
	if got := pf.cmdLine.Edit.GetText(); got != "subdir" {
		t.Fatalf("hotkey Ctrl+Enter inserted %q, want subdir", got)
	}
	if got := fsp.vfs.GetPath(); got != tmp {
		t.Fatalf("hotkey Ctrl+Enter entered %q, want to stay in %q", got, tmp)
	}

	// Exercise the frame-level fallback independently of MacroManager.Filter.
	pf.cmdLine.Clear()
	pf.ProcessKey(mainCtrlEnter)
	if got := pf.cmdLine.Edit.GetText(); got != "subdir" {
		t.Fatalf("direct Ctrl+Enter inserted %q, want subdir", got)
	}
	if got := fsp.vfs.GetPath(); got != tmp {
		t.Fatalf("direct Ctrl+Enter entered %q, want to stay in %q", got, tmp)
	}
}

func TestPanelsFrame_CD_QuotedParsing(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fsp := pf.panels[pf.activeIdx].(*FileSystemPanel)

	// Мокаем VFS, чтобы не ходить на реальный диск
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "dir with space's")
	os.MkdirAll(targetDir, 0755)
	fsp.vfs.SetPath(tmp)

	// Симулируем ввод команды cd в одинарных кавычках (Unix-style)
	// Для Windows этот тест тоже должен работать, так как мы добавили поддержку '' и там.
	pf.cmdLine.Edit.SetText("cd 'dir with space'\\''s'")

	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	gotPath := fsp.vfs.GetPath()
	if gotPath != targetDir {
		t.Errorf("CD parsing failed. Expected path %q, but VFS is at %q", targetDir, gotPath)
	}

	if !pf.cmdLine.IsEmpty() {
		t.Error("Command line should be cleared after successful CD")
	}
}

func TestPanelsFrame_PTY_SyncEscaping(t *testing.T) {
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pty := pf.pty.(*mockPty)

	tmp := t.TempDir()
	dirName := "space 'n' quotes"
	targetDir := filepath.Join(tmp, dirName)
	os.MkdirAll(targetDir, 0755)

	fsp := pf.panels[pf.activeIdx].(*FileSystemPanel)
	fsp.vfs.SetPath(tmp)

	// Вводим команду перехода
	pf.cmdLine.Edit.SetText("cd \"" + dirName + "\"")
	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	written := string(pty.written)
	if runtime.GOOS == "windows" {
		if !strings.Contains(written, "cd /d") {
			t.Errorf("Windows PTY sync failed. Expected 'cd /d', got: %q", written)
		}
	} else {
		// Проверяем, что в PTY ушла команда с одинарными кавычками и экранированием.
		// Так как путь абсолютный, проверяем наличие экранированного фрагмента имени.
		expectedPiece := "space '\\''n'\\'' quotes'"
		if !strings.Contains(written, " cd '") || !strings.Contains(written, expectedPiece) {
			t.Errorf("Unix PTY sync escaping failed.\nExpected to contain escaped name: %q\nFull output: %q", expectedPiece, written)
		}
	}
}
