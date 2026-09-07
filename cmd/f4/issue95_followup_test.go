package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestIssue95_HostConsoleTabCompletesBareDirectory(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig })
	oldProvider := vtui.PathHintProvider
	t.Cleanup(func() { vtui.PathHintProvider = oldProvider })
	oldAutoCompleteEnabled := vtui.AutoCompleteEnabled
	t.Cleanup(func() { vtui.AutoCompleteEnabled = oldAutoCompleteEnabled })

	AppConfig.CommandLineAutoComplete = true
	AppConfig.ConsoleMode = ConsoleViewFar
	AppConfig.ConsoleOverlayUI = true
	vtui.PathHintProvider = pathHintProvider
	vtui.AutoCompleteEnabled = true

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.shellMode = ShellModeHost
	pf.showPanels = false
	pf.ResizeConsole(80, 25)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	pf.getActivePanel().vfs = vfs.NewOSVFS(root)
	pf.cmdLine.Edit.SetText("cd sub")
	vtui.FrameManager.Push(pf)
	pf.enterHostConsole()

	mock := pf.pty.(*mockPty)
	beforePTY := mock.String()
	if handled := pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_TAB,
	}); !handled {
		t.Fatal("host console should consume Tab for command completion")
	}
	want := "cd subdir" + string(filepath.Separator)
	if got := pf.cmdLine.Edit.GetText(); got != want {
		t.Fatalf("Tab completion text = %q, want %q", got, want)
	}
	if got := mock.String(); got != beforePTY {
		t.Fatalf("Tab completion leaked to PTY: before=%q after=%q", beforePTY, got)
	}
}
