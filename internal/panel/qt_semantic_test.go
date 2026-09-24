package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

func TestCommandLineSemanticClickFocus(t *testing.T) {
	oldMode := config.App.NavigationMode
	t.Cleanup(func() { config.App.NavigationMode = oldMode })
	config.App.NavigationMode = config.NavigationSearchFirst
	frame, _ := panelsFrameWithMouseSelect(t)
	frame.ShowPanels = true
	frame.CmdLine.SetVisible(true)
	frame.SetCommandLineFocus(false)
	active := frame.GetActivePanel()
	active.FastFindMode = true
	if !frame.HandleSemanticAction(map[string]any{"action": "commandLine.focus"}) {
		t.Fatal("command line click rejected")
	}
	if !frame.CommandLineFocused || !frame.CmdLine.IsFocused() || active.IsFocused() || active.FastFindMode {
		t.Fatal("click did not transfer input ownership from panel to command line")
	}
	if !active.showInactiveCursor {
		t.Fatal("panel lost its inactive cursor")
	}
	frame.HandleSemanticAction(map[string]any{"action": "commandLine.focus"})
	if !frame.CommandLineFocused {
		t.Fatal("repeated click toggled focus off")
	}
	frame.CmdLine.Edit.History = []string{"example command"}
	frame.CmdLine.Edit.SetText("example")
	suggestions := vtui.NewAutoCompleteMenu(frame.CmdLine.Edit)
	vtui.FrameManager.Push(suggestions)
	frame.HandleSemanticAction(map[string]any{"action": "panel.activate", "side": frame.ActiveIdx})
	if !suggestions.IsDone() {
		t.Fatal("panel activation left command suggestions open")
	}
	if frame.CommandLineFocused || !active.IsFocused() {
		t.Fatal("panel activation did not restore panel focus")
	}
}

func TestCommandLineSemanticCursorUsesUTF16AndPreservesText(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.NavigationMode = config.NavigationSearchFirst
	frame, _ := panelsFrameWithMouseSelect(t)
	frame.ShowPanels = true
	frame.CmdLine.SetVisible(true)
	frame.CmdLine.Edit.SetText("a😀bc")
	if !frame.HandleSemanticAction(map[string]any{
		"action": "commandLine.focus", "cursorPosition": 3,
	}) {
		t.Fatal("pointer position was rejected")
	}
	frame.CmdLine.InsertString("!")
	if got := frame.CmdLine.Edit.GetText(); got != "a😀!bc" {
		t.Fatalf("click insertion = %q", got)
	}
	if !frame.HandleSemanticAction(map[string]any{
		"action": "commandLine.cursor", "cursorPosition": 1,
	}) {
		t.Fatal("visual line navigation position was rejected")
	}
	frame.CmdLine.InsertString("?")
	if got := frame.CmdLine.Edit.GetText(); got != "a?😀!bc" {
		t.Fatalf("cursor insertion = %q", got)
	}
}

func TestCommandLineSemanticSelectionUsesUTF16AndBlocksMultilineHistory(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.CommandLineMultiline = true
	frame, _ := panelsFrameWithMouseSelect(t)
	frame.ShowPanels = true
	frame.CmdLine.SetVisible(true)
	frame.SetCommandLineFocus(true)
	frame.CmdLine.Edit.History = []string{"old command"}
	frame.CmdLine.Edit.SetText("a😀bc\nlast")
	if !frame.HandleSemanticAction(map[string]any{
		"action": "commandLine.select", "anchor": 1, "cursorPosition": 4,
	}) {
		t.Fatal("selection rejected")
	}
	model := frame.CmdLine.SemanticModel(nil)
	if model.SelectionStart != 1 || model.SelectionEnd != 4 {
		t.Fatalf("selection = [%d,%d]", model.SelectionStart, model.SelectionEnd)
	}
	frame.CmdLine.Edit.SetText("abcde\n12345")
	if !frame.HandleSemanticAction(map[string]any{
		"action": "commandLine.select", "anchor": 1, "cursorPosition": 9, "block": true,
		"blockAnchorRow": 0, "blockAnchorColumn": 1,
		"blockFocusRow": 1, "blockFocusColumn": 3, "blockWrapWidth": 20,
	}) {
		t.Fatal("block selection rejected")
	}
	model = frame.CmdLine.SemanticModel(nil)
	if !model.BlockSelection || model.BlockAnchorRow != 0 || model.BlockAnchorColumn != 1 ||
		model.BlockFocusRow != 1 || model.BlockFocusColumn != 3 ||
		model.SelectionStart != -1 || model.SelectionEnd != -1 {
		t.Fatalf("block selection model = %+v", model)
	}
	frame.CmdLine.Edit.SetText("a😀bc\nlast")
	if !frame.HandleSemanticAction(map[string]any{
		"action": "commandLine.history", "direction": -1,
	}) {
		t.Fatal("multiline history action rejected")
	}
	if got := frame.CmdLine.Edit.GetText(); got != "a😀bc\nlast" {
		t.Fatalf("history replaced multiline input: %q", got)
	}
}

func TestShellCommandLineNavigationOwnership(t *testing.T) {
	oldMode := config.App.NavigationMode
	t.Cleanup(func() { config.App.NavigationMode = oldMode })
	vtui.SetDefaultPalette()
	frame := &PanelsFrame{CmdLine: cmdline.NewCommandLine(">"), ShowPanels: true}
	for _, mode := range []config.PanelNavigationMode{config.NavigationClassic, config.NavigationSearchFirst} {
		config.App.NavigationMode = mode
		for _, focused := range []bool{true, false} {
			frame.CommandLineFocused = focused
			for _, text := range []string{"", "previous command", ""} {
				frame.CmdLine.Edit.SetText(text)
				full := frame.SemanticNode(nil)
				incremental, _, ok := frame.SemanticIncrementalShell(nil)
				if !ok {
					t.Fatal("incremental shell unavailable")
				}
				for _, model := range []map[string]any{full, incremental.ToMap()} {
					commandLine := model["commandLine"].(map[string]any)
					want := mode == config.NavigationSearchFirst && focused
					if got := commandLine["ownsNavigation"]; got != want {
						t.Fatalf("mode=%s focused=%v text=%q: ownership=%v, want %v", mode, focused, text, got, want)
					}
				}
			}
		}
	}
}

func TestShellSemanticBusyTerminalRetainsLastOutputRow(t *testing.T) {
	for _, incremental := range []bool{false, true} {
		name := "full"
		if incremental {
			name = "incremental"
		}
		t.Run(name, func(t *testing.T) {
			vtui.SetDefaultPalette()
			view := terminal.NewTerminalView(80, 8)
			defer view.Close()
			pty := &fakePTY{busy: true}
			frame := &PanelsFrame{
				Pty: pty, TermView: view, CmdLine: cmdline.NewCommandLine(">"),
			}
			parser := terminal.NewAnsiParser(view, pty)
			parser.Process([]byte("\x1b[32mMoving 50%\x1b[0m"))
			model := func() map[string]any {
				if !incremental {
					return frame.SemanticNode(nil)
				}
				shell, _, ok := frame.SemanticIncrementalShell(nil)
				if !ok {
					t.Fatal("incremental shell unavailable")
				}
				return shell.ToMap()
			}
			terminalText := func(shell map[string]any) string {
				term := shell["terminal"].(map[string]any)
				var text strings.Builder
				for _, row := range semantic.AppMapSlice(term["windowRows"]) {
					for _, run := range semantic.AppMapSlice(row["runs"]) {
						text.WriteString(semantic.String(run["text"]))
					}
				}
				return text.String()
			}
			busy := model()
			if !semantic.AppBool(busy["terminalBusy"]) {
				t.Fatal("fixture terminal is not busy")
			}
			if !strings.Contains(terminalText(busy), "Moving 50%") {
				t.Fatal("running command's bottom progress row was removed as though it were an idle prompt")
			}
			parser.Process([]byte("\r\x1b[2K\x1b[32mMoving 75%\x1b[0m"))
			if !strings.Contains(terminalText(model()), "Moving 75%") {
				t.Fatal("bottom progress row did not update")
			}
			parser.Process([]byte("\r\nidle-shell-prompt>"))
			pty.busy = false
			idle := terminalText(model())
			if strings.Contains(idle, "idle-shell-prompt") || !strings.Contains(idle, "Moving 75%") {
				t.Fatalf("idle prompt suppression lost command output or exposed the prompt: %q", idle)
			}
		})
	}
}
