package panel

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
)

func TestPanelsFrame_SearchFirstSelectionShortcuts(t *testing.T) {
	oldConfig, oldRunAction := config.App, RunAction
	t.Cleanup(func() { config.App, RunAction = oldConfig, oldRunAction })
	config.App.NavigationMode = config.NavigationSearchFirst
	pf := setupMockPanelsFrame(t)
	t.Cleanup(pf.Close)
	panel := pf.GetActivePanel()
	panel.SetFocus(true)
	for _, tt := range []struct {
		char   rune
		key    uint16
		shift  bool
		action string
	}{
		{'+', 0, true, "Panel.SelectGroup"},
		{'-', vtinput.VK_OEM_MINUS, false, "Panel.DeselectGroup"},
		{'*', 0, true, "Panel.InvertSelection"},
		{'+', vtinput.VK_ADD, false, "Panel.SelectGroup"},
		{'-', vtinput.VK_SUBTRACT, false, "Panel.DeselectGroup"},
		{'*', vtinput.VK_MULTIPLY, false, "Panel.InvertSelection"},
	} {
		t.Run(fmt.Sprintf("%s/VK_%02X", tt.action, tt.key), func(t *testing.T) {
			panel.FastFindMode, panel.FastFindStr = false, ""
			var action string
			RunAction = func(name string) bool { action = name; return true }
			event := &vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tt.key, Char: tt.char,
			}
			if tt.shift {
				event.ControlKeyState = vtinput.ShiftPressed
			}
			if !pf.ProcessKey(event) {
				t.Fatal("selection shortcut was not handled")
			}
			if action != tt.action || panel.FastFindMode {
				t.Fatalf("action=%q fastFind=%v query=%q; want %s without search", action, panel.FastFindMode, panel.FastFindStr, tt.action)
			}
		})
	}
}

func TestPanelsFrame_SearchFirstSelectionCharactersRespectTextInput(t *testing.T) {
	oldConfig, oldRunAction := config.App, RunAction
	t.Cleanup(func() { config.App, RunAction = oldConfig, oldRunAction })
	config.App.NavigationMode = config.NavigationSearchFirst
	pf := setupMockPanelsFrame(t)
	t.Cleanup(pf.Close)
	panel := pf.GetActivePanel()
	panel.SetFocus(true)
	RunAction = func(name string) bool {
		t.Fatalf("text input dispatched action %s", name)
		return false
	}
	for _, commandLine := range []bool{false, true} {
		pf.CommandLineFocused = commandLine
		panel.FastFindMode = !commandLine
		panel.FastFindStr = "abc"
		pf.CmdLine.Clear()
		for _, char := range "+-*" {
			pf.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true, Char: char,
			})
		}
		if commandLine {
			if got := pf.CmdLine.Edit.GetText(); got != "+-*" {
				t.Fatalf("command-line text = %q, want +-*", got)
			}
		} else if panel.FastFindStr != "abc+-*" {
			t.Fatalf("active search = %q, want abc+-*", panel.FastFindStr)
		}
	}
}
