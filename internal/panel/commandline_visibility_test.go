package panel

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestTogglePanelsKeepsUnchangedPanelRows(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationClassic
	pf, _ := panelsFrameWithMouseSelect(t)
	t.Cleanup(pf.Close)
	pf.ShellMode = terminal.ShellModeOwn
	pf.ShowPanels = true
	pf.ResizeConsole(80, 25)

	left := pf.Panels[0].(*FileSystemPanel)
	right := pf.Panels[1].(*FileSystemPanel)
	revisions := [2]uint64{left.entriesRevision, right.entriesRevision}
	for _, show := range []bool{false, true} {
		pf.TogglePanelsVisibility()
		if pf.ShowPanels != show {
			t.Fatalf("panel visibility = %v, want %v", pf.ShowPanels, show)
		}
		if got := [2]uint64{left.entriesRevision, right.entriesRevision}; got != revisions {
			t.Fatalf("visibility toggle rebuilt panel rows: %v -> %v", revisions, got)
		}
	}
}

func TestSearchFirstUnfocusedCommandHidesOnlyWhenEmpty(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandHideUnfocused = true
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("pending command")
	pf.SetCommandLineFocus(false)
	pf.ResizeConsole(80, 25)
	if !pf.CmdLine.IsVisible() || pf.commandLineRows(80, 12) == 0 {
		t.Fatal("nonempty unfocused input was hidden")
	}
	if model := pf.commandLineSemanticModel(nil); !model.Visible {
		t.Fatal("GUI hid nonempty unfocused input")
	}
	pf.CmdLine.Clear()
	pf.ResizeConsole(80, 25)
	if pf.CmdLine.IsVisible() || pf.commandLineRows(80, 12) != 0 {
		t.Fatal("empty unfocused input remained visible")
	}
	pf.CmdLine.Edit.SetText("injected while unfocused")
	pf.SetCommandLineFocus(false)
	if !pf.CmdLine.IsVisible() {
		t.Fatal("new text did not reveal previously hidden input")
	}
}

func TestSearchFirstRetainedCommandDoesNotRunFromPanel(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	pf, _ := panelsFrameWithMouseSelect(t)
	t.Cleanup(pf.Close)
	pf.ShowPanels = true
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	panel := pf.GetActivePanel()
	if err := panel.SetKnownDirectoryPath(root); err != nil {
		t.Fatal(err)
	}
	panel.ReadDirectory()
	waitForLoad(t, panel)
	panel.SelectName("child")
	pf.SetCommandLineFocus(true)
	command := "cd " + t.TempDir()
	pf.CmdLine.Edit.SetText(command)
	pf.SetCommandLineFocus(false)
	waitForLoad(t, panel)
	panel.SelectName("child")
	if panel.GetSelectedName() != "child" {
		t.Fatalf("test cursor is on %q, expected child", panel.GetSelectedName())
	}
	if !pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("Enter was not handled")
	}
	if pf.CmdLine.Edit.GetText() != command || pf.CommandLineFocused || !pf.ShowPanels {
		t.Fatal("panel Enter changed or submitted the inactive command")
	}
	if panel.Vfs.GetPath() != child {
		t.Fatalf("Enter did not activate the panel directory: %q", panel.Vfs.GetPath())
	}
}

func TestEnterStillSubmitsOwnedCommand(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	for _, test := range []struct {
		name    string
		mode    config.PanelNavigationMode
		focused bool
	}{
		{name: "search-first focused", mode: config.NavigationSearchFirst, focused: true},
		{name: "classic", mode: config.NavigationClassic},
	} {
		t.Run(test.name, func(t *testing.T) {
			config.App.NavigationMode = test.mode
			pf, _ := panelsFrameWithMouseSelect(t)
			t.Cleanup(pf.Close)
			pf.ShowPanels = true
			pf.SetCommandLineFocus(test.focused)
			pf.CmdLine.Edit.SetText("cd " + t.TempDir())
			pf.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
			})
			if !pf.CmdLine.IsEmpty() {
				t.Fatal("Enter did not submit the owned command")
			}
		})
	}
}

func TestSearchFirstAutocompleteTildeTogglesFocus(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("pending")
	suggestions := cmdline.NewCompletionMenu(pf.CmdLine.Edit)
	vtui.FrameManager.Push(suggestions)
	if !pf.HandleAutocompleteFocusToggle(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_OEM_3, Char: '`'}) {
		t.Fatal("autocomplete consumed focus toggle")
	}
	if pf.CommandLineFocused || !suggestions.IsDone() || pf.CmdLine.Edit.GetText() != "pending" {
		t.Fatalf("focus=%v menuDone=%v text=%q", pf.CommandLineFocused, suggestions.IsDone(), pf.CmdLine.Edit.GetText())
	}
}

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
	_, _, _, hiddenBottom := pf.Panels[0].GetPosition()
	if pf.CmdLine.IsVisible() || hiddenBottom != 23 {
		t.Fatalf("hidden visibility=%v bottom=%d", pf.CmdLine.IsVisible(), hiddenBottom)
	}
	for _, focused := range []bool{true, false, true} {
		pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 0xC0, Char: '`'})
		if focused && pf.CmdLine.Edit.GetText() == "" {
			pf.CmdLine.Edit.SetText("retained command")
			pf.ResizeConsole(80, 25)
		}
		if !pf.CmdLine.IsVisible() || pf.CommandLineFocused != focused {
			t.Fatalf("tilde: visible=%v focused=%v want focus=%v", pf.CmdLine.IsVisible(), pf.CommandLineFocused, focused)
		}
		full := pf.SemanticNode(nil)["commandLine"].(map[string]any)
		incremental, _, ok := pf.SemanticIncrementalShell(nil)
		if !ok {
			t.Fatal("missing incremental scene")
		}
		for _, model := range []map[string]any{full, incremental.CommandLine.ToMap()} {
			if model["visible"] != true || model["autoHide"] != true {
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
	vtui.FrameManager.Push(pf)
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

func TestHiddenCommandLineEscapeDismissesCompletionBeforeClearing(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst

	pf, pty := panelsFrameWithMouseSelect(t)
	t.Cleanup(pf.Close)
	pf.ShellMode = terminal.ShellModeOwn
	vtui.FrameManager.Push(pf)
	pf.CmdLine.SetVisible(true)
	pf.CmdLine.Edit.AutoCompletePreview = true
	pf.CmdLine.Edit.History = []string{"echo previous"}
	pf.CmdLine.Edit.SetText("echo")
	menu := cmdline.NewCompletionMenu(pf.CmdLine.Edit)
	if !menu.HasMatches() {
		t.Fatal("no command completion")
	}
	vtui.FrameManager.Push(menu)
	menu.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN,
	})

	pressEscape := func(repeat uint16) {
		vtui.FrameManager.PostEvent(vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE,
			Char: 27, RepeatCount: repeat, InputSource: "extui",
		})
		for range 4 {
			vtui.FrameManager.Step(time.Millisecond)
		}
	}
	pressEscape(1)
	if !menu.IsDone() || pf.CmdLine.Edit.GetText() != "echo" || pf.ShowPanels {
		t.Fatalf("first Escape changed more than the popup: text=%q, panels=%v",
			pf.CmdLine.Edit.GetText(), pf.ShowPanels)
	}
	pressEscape(2)
	if pf.CmdLine.Edit.GetText() != "echo" {
		t.Fatal("holding the popup's Escape cleared the command")
	}
	pressEscape(1)
	if !pf.CmdLine.IsEmpty() || pf.ShowPanels {
		t.Fatalf("next Escape did not clear the command in place: text=%q, panels=%v",
			pf.CmdLine.Edit.GetText(), pf.ShowPanels)
	}
	if bytes.IndexByte(pty.writes, 27) >= 0 {
		t.Fatalf("Escape reached the PTY: %q", pty.writes)
	}
}
