package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/theme"
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
	theme.SetDefaultF4Palette()
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	fsp := pf.Panels[0].(*panel.FileSystemPanel)
	pf.ActiveIdx = 0

	// Имя файла со спецсимволами и пробелами
	complexName := "file with'quote & space.txt"
	fsp.Entries = []*panel.FileEntry{
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

	got := pf.CmdLine.Edit.GetText()

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
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()

	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	fsp := pf.Panels[0].(*panel.FileSystemPanel)
	pf.ActiveIdx = 0
	fsp.Vfs = vfs.NewOSVFS(tmp)
	fsp.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: "subdir", IsDir: true}}}
	fsp.Refresh()
	fsp.SetCursorIndex(0)

	mainCtrlEnter := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	pressKey(pf, mainCtrlEnter)
	if got := pf.CmdLine.Edit.GetText(); got != "subdir" {
		t.Fatalf("hotkey Ctrl+Enter inserted %q, want subdir", got)
	}
	if got := fsp.Vfs.GetPath(); got != tmp {
		t.Fatalf("hotkey Ctrl+Enter entered %q, want to stay in %q", got, tmp)
	}

	// Exercise the frame-level fallback independently of macro.MacroManager.Filter.
	pf.CmdLine.Clear()
	pf.ProcessKey(mainCtrlEnter)
	if got := pf.CmdLine.Edit.GetText(); got != "subdir" {
		t.Fatalf("direct Ctrl+Enter inserted %q, want subdir", got)
	}
	if got := fsp.Vfs.GetPath(); got != tmp {
		t.Fatalf("direct Ctrl+Enter entered %q, want to stay in %q", got, tmp)
	}
}

func TestPanelsFrame_CD_QuotedParsing(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fsp := pf.Panels[pf.ActiveIdx].(*panel.FileSystemPanel)

	// Мокаем VFS, чтобы не ходить на реальный диск
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "dir with space's")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := fsp.Vfs.SetPath(tmp); err != nil {
		t.Fatal(err)
	}

	// Симулируем ввод команды cd в одинарных кавычках (Unix-style)
	// Для Windows этот тест тоже должен работать, так как мы добавили поддержку '' и там.
	pf.CmdLine.Edit.SetText("cd 'dir with space'\\''s'")

	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	gotPath := fsp.Vfs.GetPath()
	if gotPath != targetDir {
		t.Errorf("CD parsing failed. Expected path %q, but VFS is at %q", targetDir, gotPath)
	}

	if !pf.CmdLine.IsEmpty() {
		t.Error("Command line should be cleared after successful CD")
	}
}

func TestPanelsFrame_PTY_SyncEscaping(t *testing.T) {
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pty := pf.Pty.(*paneltest.MockPty)

	tmp := t.TempDir()
	dirName := "space 'n' quotes"
	targetDir := filepath.Join(tmp, dirName)
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatal(err)
	}

	fsp := pf.Panels[pf.ActiveIdx].(*panel.FileSystemPanel)
	if err := fsp.Vfs.SetPath(tmp); err != nil {
		t.Fatal(err)
	}

	// Вводим команду перехода
	pf.CmdLine.Edit.SetText("cd \"" + dirName + "\"")
	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	written := string(pty.Written)
	if runtime.GOOS == "windows" {
		if !strings.Contains(written, "cd /d") {
			t.Errorf("Windows term.PTY sync failed. Expected 'cd /d', got: %q", written)
		}
	} else {
		// Проверяем, что в term.PTY ушла команда с одинарными кавычками и экранированием.
		// Так как путь абсолютный, проверяем наличие экранированного фрагмента имени.
		expectedPiece := "space '\\''n'\\'' quotes'"
		if !strings.Contains(written, " cd '") || !strings.Contains(written, expectedPiece) {
			t.Errorf("Unix term.PTY sync escaping failed.\nExpected to contain escaped name: %q\nFull output: %q", expectedPiece, written)
		}
	}
}

func TestPanelsFrame_LocalUnixCommandKeepsPersistentShellDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix term.PTY command composition")
	}

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pty := pf.Pty.(*paneltest.MockPty)

	tmp := t.TempDir()
	fsp := pf.Panels[pf.ActiveIdx].(*panel.FileSystemPanel)
	if err := fsp.Vfs.SetPath(tmp); err != nil {
		t.Fatal(err)
	}
	// Pretend the normal frame refresh has already synchronized this panel.
	// The command itself must not re-impose the panel path on the persistent
	// shell: an alias such as `cd:home` is allowed to change that shell's cwd.
	pf.LastPtyPath = tmp
	pf.LastPtyVFS = fsp.Vfs

	pf.CmdLine.Edit.SetText("cd:home")
	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	written := pty.String()
	panelSync := "cd '" + strings.ReplaceAll(tmp, "'", "'\\''") + "' &&"
	if strings.Contains(written, panelSync) {
		t.Fatalf("local Unix command re-imposed panel directory %q: %q", tmp, written)
	}
	if !strings.Contains(written, "cd:home") {
		t.Fatalf("alias command did not reach the persistent term.PTY shell: %q", written)
	}
}
