package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestIssue95_HostConsoleTabCompletesBareDirectory(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	oldConfig := config.App
	t.Cleanup(func() { config.App = oldConfig })
	oldProvider := vtui.PathHintProvider
	t.Cleanup(func() { vtui.PathHintProvider = oldProvider })
	oldAutoCompleteEnabled := vtui.AutoCompleteEnabled
	t.Cleanup(func() { vtui.AutoCompleteEnabled = oldAutoCompleteEnabled })

	config.App.CommandLineAutoComplete = true
	config.App.ConsoleMode = terminal.ConsoleViewFar
	config.App.ConsoleOverlayUI = true
	vtui.PathHintProvider = panel.PathHintProvider
	vtui.AutoCompleteEnabled = true

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ShellMode = terminal.ShellModeHost
	pf.ShowPanels = false
	pf.ResizeConsole(80, 25)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	pf.GetActivePanel().Vfs = vfs.NewOSVFS(root)
	pf.CmdLine.Edit.SetText("cd sub")
	vtui.FrameManager.Push(pf)
	pf.EnterHostConsole()

	mock := pf.Pty.(*paneltest.MockPty)
	beforePTY := mock.String()
	if handled := pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_TAB,
	}); !handled {
		t.Fatal("host console should consume Tab for command completion")
	}
	want := "cd subdir" + string(filepath.Separator)
	if got := pf.CmdLine.Edit.GetText(); got != want {
		t.Fatalf("Tab completion text = %q, want %q", got, want)
	}
	if got := mock.String(); got != beforePTY {
		t.Fatalf("Tab completion leaked to term.PTY: before=%q after=%q", beforePTY, got)
	}
}
