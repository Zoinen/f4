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
