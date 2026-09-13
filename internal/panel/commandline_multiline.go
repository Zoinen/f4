package panel

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
)

func (pf *PanelsFrame) InsertCommandLineBreak() {
	if pf.MultilineCommandInputActive() {
		pf.CmdLine.InsertString("\n")
	}
}

// Directly injected events still use the configured action, so unbinding a
// chord cannot revive an old hard-coded Enter shortcut.
func (pf *PanelsFrame) processModifiedEnter(e *vtinput.InputEvent) bool {
	key := keymap.EventToFarString(e)
	area := "Shell"
	if !pf.ShowPanels {
		area = "Terminal"
	}
	name := ""
	if manager := keymap.GlobalHotkeysMgr; manager != nil {
		name = keymap.ConfiguredHotkeyAction(manager, area, key)
	}
	switch name {
	case "Panel.InsertFileName":
		pf.InsertSelectedFileName()
	case "CommandLine.InsertLineBreak":
		pf.InsertCommandLineBreak()
	default:
		if name != "" && name != "None" {
			RunAction(name)
		}
	}
	return true
}

// MultilineCommandInputActive distinguishes the command editor from the panel
// and from a foreground terminal application for modified Enter handling.
func (pf *PanelsFrame) MultilineCommandInputActive() bool {
	if !config.App.CommandLineMultiline || pf.CmdLine == nil || !pf.CmdLine.IsVisible() {
		return false
	}
	if !pf.ShowPanels {
		return pf.hiddenConsoleCommandLineOwnsInput()
	}
	return !pf.SearchFirstMode() || pf.CommandLineFocused
}
