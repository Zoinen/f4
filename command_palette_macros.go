package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func commandPaletteMacroEntries(area string) []commandPaletteEntry {
	if MacroMgr == nil {
		return nil
	}
	category := Msg("CommandPalette.CategoryMacro")
	aliases := commandPaletteTranslations(
		"CommandPalette.CategoryMacro",
		"Macro.AssignTitle",
		"PanelSettings.RecordMacrosTo",
	)
	entries := []commandPaletteEntry{commandPaletteMacroRecordEntry(category, aliases)}
	entries = append(entries, commandPaletteRecordedMacroEntries(area, category, aliases)...)
	entries = append(entries, commandPaletteLuaMacroEntries(area, category, aliases)...)
	return entries
}

func commandPaletteMacroRecordEntry(category string, aliases []string) commandPaletteEntry {
	labelKey := "CommandPalette.StartMacroRecording"
	if MacroMgr != nil && MacroMgr.Recording {
		labelKey = "CommandPalette.StopMacroRecording"
	}
	return commandPaletteEntry{
		Key:          "macro:record-toggle",
		Label:        Msg(labelKey),
		EnglishLabel: map[bool]string{true: "Stop macro recording", false: "Start macro recording"}[MacroMgr != nil && MacroMgr.Recording],
		Description:  Msg("CommandPalette.MacroRecording.Desc"),
		ID:           "Macro.RecordToggle",
		Category:     category,
		Shortcut:     FormatKeyForUI("Ctrl."),
		SearchFields: append(append([]string(nil), aliases...), commandPaletteTranslations(
			"CommandPalette.StartMacroRecording",
			"CommandPalette.StopMacroRecording",
			"CommandPalette.MacroRecording.Desc",
		)...),
		run: func() bool {
			return MacroMgr != nil && MacroMgr.ToggleRecording()
		},
	}
}

func commandPaletteRecordedMacroEntries(area, category string, aliases []string) []commandPaletteEntry {
	if MacroMgr == nil {
		return nil
	}
	seen := make(map[string]bool)
	var entries []commandPaletteEntry
	appendArea := func(bindingArea string) {
		for key, events := range MacroMgr.Macros[bindingArea] {
			if len(events) == 0 || seen[strings.ToLower(key)] {
				continue
			}
			seen[strings.ToLower(key)] = true
			capturedArea, capturedKey := bindingArea, key
			entries = append(entries, commandPaletteEntry{
				Key:                "recorded-macro:" + strings.ToLower(bindingArea) + ":" + strings.ToLower(key),
				Label:              fmt.Sprintf(Msg("CommandPalette.RecordedMacro"), FormatKeyForUI(key)),
				EnglishLabel:       "Recorded macro: " + FormatKeyForUI(key),
				Description:        fmt.Sprintf(Msg("CommandPalette.RecordedMacro.Desc"), bindingArea),
				EnglishDescription: "Play a recorded keyboard macro",
				ID:                 "Macro.Recorded." + bindingArea + "." + key,
				Category:           category,
				Shortcut:           FormatKeyForUI(key),
				SearchFields: append(append([]string{bindingArea}, aliases...), commandPaletteTranslations(
					"CommandPalette.RecordedMacro",
					"CommandPalette.RecordedMacro.Desc",
				)...),
				run: func() bool { return runRecordedMacro(capturedArea, capturedKey) },
			})
		}
	}
	appendArea(area)
	if !strings.EqualFold(area, "Common") {
		appendArea("Common")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}

func runRecordedMacro(area, key string) bool {
	if MacroMgr == nil || vtui.FrameManager == nil {
		return false
	}
	areaMacros := MacroMgr.Macros[area]
	sequence, ok := areaMacros[key]
	if !ok || len(sequence) == 0 {
		return false
	}
	copySequence := append([]*vtinput.InputEvent(nil), sequence...)
	vtui.FrameManager.InjectEvents(copySequence)
	return true
}

func commandPaletteLuaMacroEntries(area, category string, aliases []string) []commandPaletteEntry {
	if MacroMgr == nil || MacroMgr.Lua == nil {
		return nil
	}
	bindings := MacroMgr.Lua.Bindings(area)
	entries := make([]commandPaletteEntry, 0, len(bindings))
	for _, binding := range bindings {
		binding := binding
		bindingArea := binding.Area
		bindingKey := binding.Key
		label := strings.TrimSpace(binding.Description)
		if label == "" {
			label = fmt.Sprintf(Msg("CommandPalette.LuaMacro"), FormatKeyForUI(binding.Key))
		}
		entries = append(entries, commandPaletteEntry{
			Key:                "lua-macro:" + strings.ToLower(binding.Area) + ":" + strings.ToLower(binding.Key),
			Label:              label,
			EnglishLabel:       label,
			Description:        binding.Source,
			EnglishDescription: binding.Source,
			ID:                 "Macro.Lua." + binding.Area + "." + binding.Key,
			Category:           category,
			Shortcut:           FormatKeyForUI(binding.Key),
			SearchFields: append(append([]string{binding.Area, binding.Source}, aliases...), commandPaletteTranslations(
				"CommandPalette.LuaMacro",
			)...),
			run: func() bool {
				return MacroMgr != nil && MacroMgr.Lua != nil && MacroMgr.Lua.RunExact(bindingArea, bindingKey)
			},
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}
