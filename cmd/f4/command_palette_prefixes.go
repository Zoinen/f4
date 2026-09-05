package main

import (
	"fmt"
	"strings"

	"github.com/unxed/vtui"
)

func commandPalettePrefixEntries(area string, pf *PanelsFrame) []commandPaletteEntry {
	if pf == nil || (area != "Shell" && area != "Terminal") ||
		(area == "Terminal" && !commandPaletteConditionTrue("TerminalQuiet")) {
		return nil
	}
	category := Msg("CommandPalette.CategoryCommandPrefix")
	aliases := commandPaletteTranslations(
		"CommandPalette.CategoryCommandPrefix",
		"CommandPalette.CommandPrefix.Desc",
		"CommandPalette.CategoryPlugin",
	)
	snapshot := commandPrefixSnapshot()
	entries := make([]commandPaletteEntry, 0, len(snapshot))
	for _, prefix := range snapshot {
		prefix := prefix
		entries = append(entries, commandPaletteEntry{
			Key:                "command-prefix:" + strings.ToLower(prefix.id),
			Label:              prefix.prefix + ":",
			EnglishLabel:       prefix.prefix + ":",
			Description:        fmt.Sprintf(Msg("CommandPalette.CommandPrefix.Desc"), prefix.id),
			EnglishDescription: "Insert a plugin command prefix",
			ID:                 prefix.id,
			Category:           category,
			SearchFields:       append([]string{prefix.prefix, prefix.id}, aliases...),
			run: func() bool {
				return focusCommandPrefix(pf, prefix.id, prefix.prefix)
			},
		})
	}
	return entries
}

func focusCommandPrefix(pf *PanelsFrame, id, prefix string) bool {
	if pf == nil || pf.closed || pf.cmdLine == nil || findPanelsFrameAnyScreen() != pf {
		return false
	}
	commandPrefixRegistry.RLock()
	registration := commandPrefixRegistry.byID[id]
	active := registration != nil && registration.active && registration.prefix == prefix
	commandPrefixRegistry.RUnlock()
	if !active {
		return false
	}
	pf.cancelFastFind()
	pf.cmdLine.Edit.SetText(prefix + ":")
	if pf.searchFirstMode() {
		pf.setCommandLineFocus(true)
	} else {
		pf.cmdLine.SetFocus(true)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
	return true
}
