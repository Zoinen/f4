package panel

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestSearchFirstCommandLineVisibility(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandHideUnfocused = true
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.ShowKeyBar = true
	pf.SetCommandLineFocus(false)
	pf.ResizeConsole(80, 25)
	pf.CmdLine.Edit.SetText("retained command")
	_, _, _, hiddenBottom := pf.Panels[0].GetPosition()
	if pf.CmdLine.IsVisible() || hiddenBottom != 23 {
		t.Fatalf("hidden visibility=%v bottom=%d", pf.CmdLine.IsVisible(), hiddenBottom)
	}
	for _, visible := range []bool{true, false, true} {
		pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 0xC0, Char: '`'})
		if pf.CmdLine.IsVisible() != visible || pf.CommandLineFocused != visible {
			t.Fatalf("tilde: visible=%v focused=%v want=%v", pf.CmdLine.IsVisible(), pf.CommandLineFocused, visible)
		}
		full := pf.SemanticNode(nil)["commandLine"].(map[string]any)
		incremental, _, ok := pf.SemanticIncrementalShell(nil)
		if !ok {
			t.Fatal("missing incremental scene")
		}
		for _, model := range []map[string]any{full, incremental.CommandLine.ToMap()} {
			if model["visible"] != visible || model["autoHide"] != true {
				t.Fatalf("incorrect command state: %v", model)
			}
		}
		if pf.CmdLine.Edit.GetText() != "retained command" {
			t.Fatal("toggle changed command text")
		}
	}
	pf.SetCommandLineFocus(false)
	pf.ShowPanels = false
	pf.ResizeConsole(80, 25)
	if pf.commandLineHiddenByFocus() || pf.commandLineRows(80, 12) == 0 {
		t.Fatal("terminal command input was hidden")
	}
	pf.ShowPanels = true
	for _, mode := range []config.PanelNavigationMode{config.NavigationClassic, config.NavigationVim} {
		config.App.NavigationMode = mode
		pf.ResizeConsole(80, 25)
		if !pf.CmdLine.IsVisible() {
			t.Fatal("option affected a different navigation mode")
		}
	}
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandHideUnfocused = false
	pf.ResizeConsole(80, 25)
	if !pf.CmdLine.IsVisible() {
		t.Fatal("disabled option hid input")
	}
}

func TestSearchFirstConsoleToggleRestoresCommandCursor(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandHideUnfocused = true
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.SetCommandLineFocus(false)
	for cycle := 0; cycle < 3; cycle++ {
		pf.TogglePanelsVisibility()
		if !pf.CmdLine.IsFocused() || pf.CommandLineFocused {
			t.Fatal("console input must gain focus without changing the saved panel input target")
		}
		pf.Show(vtui.FrameManager.Screen())
		for _, focused := range []bool{false, true} {
			pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: focused})
			model := pf.commandLineSemanticModel(nil)
			if model.CursorVisible != focused {
				t.Fatalf("application focus=%v: cursor visible=%v", focused, model.CursorVisible)
			}
		}
		pf.TogglePanelsVisibility()
		if pf.CmdLine.IsFocused() || !pf.commandLineHiddenByFocus() {
			t.Fatal("returning to panels did not restore hidden, unfocused input")
		}
	}
}

func TestPathBarVisibilityInIncrementalShell(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	pf, _ := panelsFrameWithMouseSelect(t)
	for _, hidden := range []bool{false, true, false} {
		config.App.HidePanelPathBar = hidden
		pf.SemanticNode(nil)
		shell, _, valid := pf.SemanticIncrementalShell(nil)
		if !valid || shell.ToMap()["hidePanelPathBar"] != hidden {
			t.Fatalf("incremental path bar state lost: valid=%v hidden=%v", valid, hidden)
		}
	}
}
