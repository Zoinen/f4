package app

import (
	"fmt"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtui"
	"strings"
)

func commandPalettePrefixEntries(area string, pf *panel.PanelsFrame) []commandPaletteEntry {
	if pf == nil || (area != "Shell" && area != "Terminal") ||
		(area == "Terminal" && !keymap.ConditionTrue("TerminalQuiet")) {
		return nil
	}
	category := i18n.Msg("CommandPalette.CategoryCommandPrefix")
	aliases := commandPaletteTranslations(
		"CommandPalette.CategoryCommandPrefix",
		"CommandPalette.CommandPrefix.Desc",
		"CommandPalette.CategoryPlugin",
	)
	snapshot := panel.CommandPrefixSnapshot()
	entries := make([]commandPaletteEntry, 0, len(snapshot))
	for _, prefix := range snapshot {
		prefix := prefix
		entries = append(entries, commandPaletteEntry{
			Key:                "command-prefix:" + strings.ToLower(prefix.Id),
			Label:              prefix.Prefix + ":",
			EnglishLabel:       prefix.Prefix + ":",
			Description:        fmt.Sprintf(i18n.Msg("CommandPalette.CommandPrefix.Desc"), prefix.Id),
			EnglishDescription: "Insert a plugin command prefix",
			ID:                 prefix.Id,
			Category:           category,
			SearchFields:       append([]string{prefix.Prefix, prefix.Id}, aliases...),
			run: func() bool {
				return focusCommandPrefix(pf, prefix.Id, prefix.Prefix)
			},
		})
	}
	return entries
}

func focusCommandPrefix(pf *panel.PanelsFrame, id, prefix string) bool {
	if pf == nil || pf.Closed || pf.CmdLine == nil || panel.FindPanelsFrameAnyScreen() != pf {
		return false
	}
	panel.CommandPrefixRegistry.RLock()
	registration := panel.CommandPrefixRegistry.ByID[id]
	active := registration != nil && registration.Active && registration.Prefix == prefix
	panel.CommandPrefixRegistry.RUnlock()
	if !active {
		return false
	}
	pf.CancelFastFind()
	pf.CmdLine.Edit.SetText(prefix + ":")
	if pf.SearchFirstMode() {
		pf.SetCommandLineFocus(true)
	} else {
		pf.CmdLine.SetFocus(true)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
	return true
}
