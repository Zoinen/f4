package panel

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// processCommandLinePaste keeps bracketed clipboard text in the editor's
// transaction. Paste markers have no KeyDown flag and must reach the editor
// before the normal key-only routing. Contents are literal text, not shortcuts
// or autocomplete requests.
func (pf *PanelsFrame) processCommandLinePaste(event *vtinput.InputEvent) bool {
	if pf.CmdLine == nil {
		return false
	}
	if event.Type == vtinput.PasteEventType && event.PasteStart {
		if pf.ShowPanels {
			if panel := pf.GetActivePanel(); panel != nil && panel.FastFindMode {
				return false
			}
			if panel := pf.AltPanels[pf.ActiveIdx]; panel != nil && panel.IsFocused() {
				return false
			}
		}
		ownsInput := pf.hiddenConsoleCommandLineOwnsInput() ||
			(pf.ShowPanels && pf.CmdLine.IsVisible() && (!pf.SearchFirstMode() || pf.CommandLineFocused))
		if !ownsInput {
			return false
		}
		pf.commandLinePasting = true
		pf.CmdLine.SyncInputOptions()
		return pf.CmdLine.Edit.ProcessKey(event)
	}
	if !pf.commandLinePasting {
		return false
	}
	if event.Type != vtinput.KeyEventType && event.Type != vtinput.PasteEventType {
		return false
	}
	handled := pf.CmdLine.Edit.ProcessKey(event)
	if event.Type == vtinput.PasteEventType && !event.PasteStart {
		pf.commandLinePasting = false
		pf.CmdLine.Edit.HistoryPos = -1
		vtui.DebugLog("[FIX] command-line clipboard paste committed")
	}
	return handled
}
