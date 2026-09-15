package app

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestCommandLineRunInNewWorkspace(t *testing.T) {
	old := config.App
	defer func() { config.App = old }()
	config.App.NavigationMode = config.NavigationSearchFirst
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	fm.Push(pf)
	pf.SetCommandLineFocus(true)
	sourcePath := pf.GetActivePanel().Vfs.GetPath()
	targetPath := t.TempDir()
	command := fmt.Sprintf("cd \"%s\"", targetPath)
	pf.CmdLine.Edit.SetText(command)
	if !pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed}) {
		t.Fatal("action did not run")
	}
	clone, ok := fm.GetTopFrame().(*panel.PanelsFrame)
	if !ok || clone == pf || len(fm.Screens) != 2 {
		t.Fatal("command did not create and activate a distinct workspace")
	}
	defer clone.Close()
	if clone.GetActivePanel().Vfs.GetPath() != targetPath {
		t.Fatal("command did not run in the cloned workspace")
	}
	if pf.GetActivePanel().Vfs.GetPath() != sourcePath || pf.CmdLine.Edit.GetText() != command {
		t.Fatal("source workspace was changed")
	}
	if !clone.CmdLine.IsEmpty() {
		t.Fatal("new workspace did not submit the command")
	}
}

func TestCommandLineWorkspaceHotkeyDefaults(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("echo test")
	hm := keymap.NewHotkeyManager("")
	hm.InitDefaults()
	for _, area := range []string{"Shell", "Terminal"} {
		if got := keymap.ConfiguredHotkeyAction(hm, area, "CtrlShiftEnter"); got != "CommandLine.RunInNewWorkspace" {
			t.Fatalf("%s/CtrlShiftEnter = %q", area, got)
		}
		if got := keymap.ConfiguredHotkeyAction(hm, area, "AltShiftEnter"); got != "Panel.SystemExplorer" {
			t.Fatalf("%s/AltShiftEnter = %q", area, got)
		}
		hm.Bind(area, "CtrlShiftEnter", "None")
		hm.Bind(area, "CtrlF9", "CommandLine.RunInNewWorkspace")
		if got := keymap.ConfiguredHotkeyAction(hm, area, "CtrlF9"); got != "CommandLine.RunInNewWorkspace" {
			t.Fatal("action cannot be reassigned")
		}
	}
}
