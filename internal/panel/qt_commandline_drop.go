package panel

import (
	"strings"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtinput"
)

func (pf *PanelsFrame) handleCommandLineDrop(action map[string]any) bool {
	if pf.Closed || pf.CmdLine == nil || !pf.CmdLine.IsVisible() ||
		(!pf.ShowPanels && !pf.hiddenConsoleCommandLineOwnsInput()) {
		return false
	}
	paths := semantic.StringSlice(action["paths"])
	if source, ok := action["source"].(map[string]any); ok {
		panel, _ := pf.semanticDragSource(source)
		if panel == nil {
			return false
		}
		paths = nil
		for _, id := range semantic.StringSlice(source["entryIds"]) {
			index, valid := panel.semanticEntryIndex(map[string]any{"entryId": id, "catalogRevision": source["catalogRevision"]})
			if !valid || index < 0 || index >= len(panel.Entries) {
				return false
			}
			name := panel.Entries[index].Name
			if name == "" || name == "." || name == ".." {
				return false
			}
			paths = append(paths, panel.Vfs.Join(panel.Vfs.GetPath(), name))
		}
	}
	if len(paths) == 0 {
		return false
	}
	words := make([]string, 0, len(paths))
	for _, path := range paths {
		word, err := cmdline.QuoteCommandPath(userMenuCommandDialect(pf), path)
		if err != nil || path == "" || strings.ContainsAny(path, "\r\n\x00") {
			return false
		}
		words = append(words, word)
	}
	// Use the editor's transaction so selection replacement and large drops
	// commit once, just like clipboard paste. Nothing is submitted to the PTY.
	pf.CommandLineFocused = true
	if panel := pf.GetActivePanel(); panel != nil {
		panel.clearFastFindForSemanticPointerIntent()
	}
	if panel := pf.AltPanels[pf.ActiveIdx]; panel != nil {
		panel.SetFocus(false)
	}
	pf.CmdLine.SetFocus(true)
	pf.CmdLine.Edit.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	for _, r := range strings.Join(words, " ") + " " {
		pf.CmdLine.Edit.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	pf.CmdLine.Edit.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType})
	pf.CmdLine.Edit.HistoryPos = -1
	return true
}
