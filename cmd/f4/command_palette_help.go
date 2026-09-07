package main

import (
	"reflect"
	"strings"

	"github.com/unxed/vtui"
)

type commandPaletteHelpFrame interface {
	vtui.Frame
	PopTopic()
	SwitchTopic(string)
}

// commandPaletteHelpEntries mirrors the commands HelpView owns before vtui's
// framework fallbacks. Query-dependent entries are intentionally absent when
// no live search belongs to this exact HelpView.
func commandPaletteHelpEntries(help commandPaletteHelpFrame) []commandPaletteEntry {
	value := reflect.ValueOf(help)
	if help == nil || ((value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil()) {
		return nil
	}
	category := Msg("CommandPalette.CategoryHelp")
	newEntry := func(id, labelKey, english, description, shortcut string, visible func() bool, run func() bool) commandPaletteEntry {
		label := Msg(labelKey)
		if label == "" || strings.HasPrefix(label, "{") {
			label = english
		}
		return commandPaletteEntry{
			Key:                "help:" + strings.ToLower(id),
			Label:              plainLabel(label),
			EnglishLabel:       english,
			Description:        plainLabel(label),
			EnglishDescription: description,
			ID:                 "Help." + id,
			Category:           category,
			Shortcut:           shortcut,
			SearchFields:       commandPaletteTranslations("CommandPalette.CategoryHelp", labelKey),
			run: func() bool {
				if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != help || (visible != nil && !visible()) {
					return false
				}
				return run()
			},
		}
	}

	queryActive := func() bool {
		topicName, _, topicOK := helpTopicForFrame(help)
		return topicOK && currentHelpSearch != nil && currentHelpSearch.frame == help &&
			currentHelpSearch.topicName == topicName && len(currentHelpSearch.query) > 0
	}
	hasHistory := func() bool {
		length, ok := nestedHelpLen(reflect.ValueOf(help), "history")
		return ok && length > 0
	}
	closeShortcut := "Esc, F10"
	if queryActive() {
		// Escape belongs to ClearSearch until the live query is gone.
		closeShortcut = "F10"
	}
	entries := []commandPaletteEntry{
		newEntry("Close", "CommandPalette.Help.Close", "Close help", "Close the Help window", closeShortcut, nil, func() bool {
			help.Close()
			return true
		}),
		newEntry("Zoom", "CommandPalette.Help.Zoom", "Toggle Help zoom", "Toggle the Help window between normal and full-screen size", "F5", nil, func() bool {
			return toggleHelpZoom(help)
		}),
		newEntry("Contents", "CommandPalette.Help.Contents", "Help contents", "Open the Help contents topic", "", nil, func() bool {
			if vtui.GlobalHelpEngine == nil || vtui.GlobalHelpEngine.GetTopic("Contents") == nil {
				return false
			}
			help.SwitchTopic("Contents")
			vtui.FrameManager.Redraw()
			return true
		}),
	}
	if hasHistory() {
		entries = append(entries, newEntry("Back", "CommandPalette.Help.Back", "Previous Help topic", "Return to the previous Help topic", "Backspace", hasHistory, func() bool {
			help.PopTopic()
			vtui.FrameManager.Redraw()
			return true
		}))
	}
	if queryActive() {
		entries = append(entries,
			newEntry("FindNext", "CommandPalette.Help.FindNext", "Next Help search result", "Move to the next match in the active Help search", "F3, Ctrl+Enter", queryActive, func() bool {
				return moveHelpSearch(help, false)
			}),
			newEntry("FindPrevious", "CommandPalette.Help.FindPrevious", "Previous Help search result", "Move to the previous match in the active Help search", "Shift+F3, Ctrl+Shift+Enter", queryActive, func() bool {
				return moveHelpSearch(help, true)
			}),
			newEntry("ClearSearch", "CommandPalette.Help.ClearSearch", "Clear Help search", "Clear the active Help search query", "Esc", queryActive, func() bool {
				currentHelpSearch = nil
				vtui.FrameManager.Redraw()
				return true
			}),
		)
	}
	return entries
}
