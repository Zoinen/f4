package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/unxed/vtui"
)

func commandPalettePanelsContextEntries(pf *PanelsFrame) []commandPaletteEntry {
	if pf == nil || pf.closed || !pf.showPanels {
		return nil
	}
	category := plainLabel(Msg("Help.Area.Shell"))
	entries := []commandPaletteEntry{
		commandPaletteLocalizedPanelKeyEntry(pf, "Panel.ActivateSelected", "CommandPalette.Panel.ActivateSelected", "Activate selected item", "CommandPalette.Panel.ActivateSelected.Desc", "Open the selected item or execute it", "Enter", "Enter", category, nil, "Menu.Files.View"),
		commandPaletteLocalizedPanelKeyEntry(pf, "Panel.SwitchActive", "CommandPalette.Panel.SwitchActive", "Switch active panel", "CommandPalette.Panel.SwitchActive.Desc", "Move focus to the other panel", "Tab", "Tab", category, nil, "Panel.Other"),
	}
	if pf.getActivePanel() != nil {
		entries = append(entries, commandPaletteLocalizedPanelKeyEntry(pf, "Panel.ToggleSelection", "CommandPalette.Panel.ToggleSelection", "Toggle item selection", "CommandPalette.Panel.ToggleSelection.Desc", "Toggle selection of the current item and advance", "Ins", "Ins", category, nil, "Help.PanelNav"))
	}
	if panel := pf.getActivePanel(); panel != nil {
		if task := panel.providerOpenTask; task != nil {
			entries = append(entries, commandPaletteLocalizedPanelKeyEntry(
				pf,
				"Provider.CancelOpen",
				"CommandPalette.Provider.CancelOpen",
				"Cancel opening provider",
				"CommandPalette.Provider.CancelOpen.Desc",
				"Cancel the pending provider connection and restore the source panel",
				"Esc",
				"Esc",
				category,
				func() bool { return pf.getActivePanel() == panel && panel.providerOpenTask == task },
				"Provider.Opening",
			))
		}
		if panel.fastFindMode {
			entry := commandPaletteLocalizedPanelKeyEntry(
				pf,
				"FastFind.ToggleMatchMode",
				"CommandPalette.FastFind.ToggleMatchMode",
				"Toggle Fast Find match mode",
				"CommandPalette.FastFind.ToggleMatchMode.Desc",
				"Switch Fast Find between prefix matching and matching anywhere in the name",
				"F2",
				"F2",
				category,
				func() bool { return pf.getActivePanel() == panel && panel.fastFindMode },
				"Help.FastFind",
			)
			entry.Checked = strings.HasPrefix(panel.fastFindStr, "*")
			entries = append(entries, entry)
		}
	}

	if pf.searchFirstMode() {
		commandLineFocused := pf.commandLineFocused
		entry := commandPaletteLocalizedPanelKeyEntry(
			pf,
			"Panel.ToggleCommandLineFocus",
			"CommandPalette.Panel.ToggleCommandLineFocus",
			"Toggle command-line focus",
			"CommandPalette.Panel.ToggleCommandLineFocus.Desc",
			"Switch input focus between the active file panel and the command line",
			"` / ~ / ё",
			"`",
			category,
			func() bool {
				return pf.showPanels && pf.searchFirstMode() && pf.commandLineFocused == commandLineFocused
			},
			"Config.NavigationMode.SearchFirst",
		)
		entry.Checked = commandLineFocused
		entries = append(entries, entry)
	}

	if target := pf.currentRemotePTYInterruptTarget(); target != nil {
		entries = append(entries, commandPaletteLocalizedPanelKeyEntry(
			pf,
			"Panel.InterruptRemoteCommand",
			"CommandPalette.Panel.InterruptRemoteCommand",
			"Interrupt remote command",
			"CommandPalette.Panel.InterruptRemoteCommand.Desc",
			"Send the remote shell's interrupt sequence to the active panel PTY",
			"Ctrl+C",
			"CtrlC",
			category,
			func() bool { return target.matches(pf.currentRemotePTYInterruptTarget()) },
			"Help.Terminal",
		))
	}

	if pf.activeIdx >= 0 && pf.activeIdx < len(pf.altPanels) {
		switch panel := pf.altPanels[pf.activeIdx].(type) {
		case *InfoPanel:
			if panel != nil && panel.IsFocused() {
				entries = append(entries, commandPaletteLocalizedPanelKeyEntry(
					pf,
					"InfoPanel.CopyCurrent",
					"CommandPalette.Info.CopyCurrent",
					"Copy current information value",
					"CommandPalette.Info.CopyCurrent.Desc",
					"Copy the focused information value or selected rows to the clipboard",
					"C",
					"C",
					plainLabel(Msg("InfoPanel.Title")),
					func() bool { return commandPaletteInfoPanelFocused(pf, panel) },
					"InfoPanel.Title",
				))
			}
		case *QuickViewPanel:
			if panel != nil && panel.IsFocused() {
				labelKey := "KeyBar.F2Wrap"
				english := "Enable wrapping in Quick View"
				if panel.wrap {
					labelKey = "KeyBar.F2Unwrap"
					english = "Disable wrapping in Quick View"
				}
				entry := commandPaletteLocalizedPanelKeyEntry(
					pf,
					"QuickView.ToggleWrap",
					labelKey,
					english,
					"CommandPalette.QuickView.ToggleWrap.Desc",
					"Toggle long-line wrapping in Quick View",
					"F2",
					"F2",
					plainLabel(Msg("QuickView.Title")),
					func() bool { return commandPaletteQuickViewPanelFocused(pf, panel) },
					"QuickView.Title",
				)
				entry.Checked = panel.wrap
				entries = append(entries, entry)
			}
		case *AIChatPanel:
			if panel != nil && panel.IsFocused() {
				entries = append(entries, commandPaletteLocalizedPanelKeyEntry(
					pf,
					"AI.CopyLastResponse",
					"CommandPalette.AI.CopyLastResponse",
					"Copy last AI response",
					"CommandPalette.AI.CopyLastResponse.Desc",
					"Copy the latest assistant response to the clipboard",
					"Right Ctrl+C",
					"RCtrlC",
					plainLabel(Msg("Action.AI.ViewChat")),
					func() bool { return commandPaletteAIChatPanelFocused(pf, panel) },
					"Action.AI.ViewChat",
				))
				barKind := aiBarNone
				if panel.focusedLinkIdx == -2 {
					barKind = panel.barKind()
				}
				entries = append(entries, commandPaletteAIChatFocusedEntries(pf, panel, barKind)...)
			}
		}
	}

	entries = append(entries, commandPaletteBookmarkEntries(pf)...)
	return entries
}

func commandPaletteLocalizedPanelKeyEntry(
	pf *PanelsFrame,
	id, labelKey, englishLabel, descKey, englishDescription, shortcut, key, category string,
	valid func() bool,
	aliasKeys ...string,
) commandPaletteEntry {
	description := Msg(descKey)
	if description == "" || strings.HasPrefix(description, "{") {
		description = englishDescription
	}
	entry := commandPalettePanelKeyEntry(
		pf, id, labelKey, englishLabel, description, shortcut, key, category,
		append(aliasKeys, descKey)...,
	)
	entry.EnglishDescription = englishDescription
	entry.run = func() bool {
		if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != pf || pf.closed {
			return false
		}
		if valid != nil && !valid() {
			return false
		}
		return pf.ProcessKey(ParseFarKey(key))
	}
	return entry
}

func commandPaletteInfoPanelFocused(pf *PanelsFrame, panel *InfoPanel) bool {
	if pf == nil || panel == nil || pf.activeIdx < 0 || pf.activeIdx >= len(pf.altPanels) {
		return false
	}
	current, ok := pf.altPanels[pf.activeIdx].(*InfoPanel)
	return ok && current == panel && panel.IsFocused()
}

func commandPaletteQuickViewPanelFocused(pf *PanelsFrame, panel *QuickViewPanel) bool {
	if pf == nil || panel == nil || pf.activeIdx < 0 || pf.activeIdx >= len(pf.altPanels) {
		return false
	}
	current, ok := pf.altPanels[pf.activeIdx].(*QuickViewPanel)
	return ok && current == panel && panel.IsFocused()
}

func commandPaletteAIChatPanelFocused(pf *PanelsFrame, panel *AIChatPanel) bool {
	if pf == nil || panel == nil || pf.activeIdx < 0 || pf.activeIdx >= len(pf.altPanels) {
		return false
	}
	current, ok := pf.altPanels[pf.activeIdx].(*AIChatPanel)
	return ok && current == panel && panel.IsFocused()
}

// commandPaletteAIChatFocusedEntries exposes the keys owned by the currently
// focused response link or strip. barKind is passed in so discovery remains a
// pure snapshot; every callback revalidates the live focus and target before
// routing the key back through AIChatPanel.ProcessKey.
func commandPaletteAIChatFocusedEntries(pf *PanelsFrame, panel *AIChatPanel, barKind int) []commandPaletteEntry {
	if !commandPaletteAIChatPanelFocused(pf, panel) {
		return nil
	}
	category := plainLabel(Msg("Action.AI.ViewChat"))
	if panel.focusedLinkIdx == -1 {
		draft := panel.input.GetText()
		if strings.TrimSpace(draft) == "" {
			return nil
		}
		return []commandPaletteEntry{commandPaletteLocalizedPanelKeyEntry(
			pf,
			"AI.SendDraft",
			"CommandPalette.AI.SendDraft",
			"Send AI message",
			"CommandPalette.AI.SendDraft.Desc",
			"Send the current AI chat draft",
			"Enter",
			"Enter",
			category,
			func() bool {
				return commandPaletteAIChatPanelFocused(pf, panel) &&
					panel.focusedLinkIdx == -1 && panel.input.GetText() == draft
			},
			"AI.InputLabel",
		)}
	}
	if panel.focusedLinkIdx == -2 {
		if barKind == aiBarNone {
			return nil
		}
		labelKey := "CommandPalette.AI.OpenContextBar"
		englishLabel := "Open attached AI context"
		descKey := "CommandPalette.AI.OpenContextBar.Desc"
		englishDescription := "Open the attached context files shown in the focused AI bar"
		aliasKeys := []string{"Action.AI.ViewContext"}
		if barKind == aiBarPatch {
			labelKey = "Action.AI.ApplyPatch"
			englishLabel = "Apply AI patch"
			descKey = "Action.AI.ApplyPatch.Desc"
			englishDescription = "Apply the patch shown in the focused AI bar"
			aliasKeys = []string{"AI.ApplyPatchBar"}
		}
		barStillFocused := func() bool {
			return commandPaletteAIChatPanelFocused(pf, panel) &&
				panel.focusedLinkIdx == -2 && panel.barKind() == barKind
		}
		entries := []commandPaletteEntry{commandPaletteLocalizedPanelKeyEntry(
			pf,
			"AI.ActivateFocusedBar",
			labelKey,
			englishLabel,
			descKey,
			englishDescription,
			"Enter",
			"Enter",
			category,
			barStillFocused,
			aliasKeys...,
		)}
		if barKind == aiBarPatch {
			entries = append(entries, commandPaletteLocalizedPanelKeyEntry(
				pf,
				"AI.InspectFocusedPatch",
				"CommandPalette.AI.InspectPatch",
				"Inspect AI patch",
				"CommandPalette.AI.InspectPatch.Desc",
				"Open the focused patch in the viewer before applying it",
				"F3",
				"F3",
				category,
				barStillFocused,
				"AI.PatchTitle",
			))
		}
		return entries
	}

	linkIndex := panel.focusedLinkIdx
	if linkIndex < 0 || linkIndex >= len(panel.visibleLinks) {
		return nil
	}
	linkTarget := panel.visibleLinks[linkIndex].target
	linkStillFocused := func() bool {
		return commandPaletteAIChatPanelFocused(pf, panel) &&
			panel.focusedLinkIdx == linkIndex && linkIndex < len(panel.visibleLinks) &&
			panel.visibleLinks[linkIndex].target == linkTarget
	}
	return []commandPaletteEntry{
		commandPaletteLocalizedPanelKeyEntry(
			pf,
			"AI.OpenFocusedLink",
			"CommandPalette.AI.OpenFocusedLink",
			"Open focused AI link",
			"CommandPalette.AI.OpenFocusedLink.Desc",
			"Open the file or AI view referenced by the focused response link",
			"Enter",
			"Enter",
			category,
			linkStillFocused,
			"Action.AI.ViewOut",
		),
		commandPaletteLocalizedPanelKeyEntry(
			pf,
			"AI.CopyFocusedLinkTarget",
			"CommandPalette.AI.CopyFocusedLinkTarget",
			"Copy focused AI link target",
			"CommandPalette.AI.CopyFocusedLinkTarget.Desc",
			"Copy the file referenced by the focused response link to the other panel",
			"F5",
			"F5",
			category,
			linkStillFocused,
			"Menu.Files.Copy",
		),
	}
}

func commandPalettePanelKeyEntry(pf *PanelsFrame, id, labelKey, englishLabel, description, shortcut, key, category string, aliasKeys ...string) commandPaletteEntry {
	label := Msg(labelKey)
	if label == "" || strings.HasPrefix(label, "{") {
		label = englishLabel
	}
	translationKeys := append([]string{labelKey}, aliasKeys...)
	return commandPaletteEntry{
		Key:                "panel-context:" + strings.ToLower(id),
		Label:              plainLabel(label),
		EnglishLabel:       englishLabel,
		Description:        description,
		EnglishDescription: description,
		ID:                 id,
		Category:           category,
		Shortcut:           shortcut,
		SearchFields:       commandPaletteTranslations(translationKeys...),
		run: func() bool {
			if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != pf || pf.closed {
				return false
			}
			return pf.ProcessKey(ParseFarKey(key))
		},
	}
}

func commandPaletteBookmarkEntries(pf *PanelsFrame) []commandPaletteEntry {
	category := plainLabel(Msg("Menu.Commands.Bookmarks"))
	aliases := commandPaletteTranslations("Menu.Commands.Bookmarks", "Action.Panel.Bookmarks.Desc")
	bookmarks, _ := LoadBookmarks(BookmarksFilePath())
	entries := make([]commandPaletteEntry, 0, 21)
	for slot := range bookmarks {
		slot := slot
		if strings.TrimSpace(bookmarks[slot].Path) != "" {
			path := bookmarks[slot].Path
			entries = append(entries, commandPaletteEntry{
				Key:                fmt.Sprintf("bookmark:goto:%d", slot),
				Label:              fmt.Sprintf(Msg("CommandPalette.Bookmark.GoTo"), slot, path),
				EnglishLabel:       fmt.Sprintf("Go to bookmark %d: %s", slot, path),
				Description:        path,
				EnglishDescription: path,
				ID:                 fmt.Sprintf("Bookmark.GoTo.%d", slot),
				Category:           category,
				Shortcut:           fmt.Sprintf("Right Ctrl+%d, Ctrl+Alt+%d", slot, slot),
				SearchFields:       append([]string{path}, aliases...),
				run: func() bool {
					return runCommandPaletteBookmarkGoto(pf, slot)
				},
			})
		}
		entries = append(entries, commandPaletteEntry{
			Key:                fmt.Sprintf("bookmark:save:%d", slot),
			Label:              fmt.Sprintf(Msg("CommandPalette.Bookmark.Save"), slot),
			EnglishLabel:       fmt.Sprintf("Save current folder to bookmark %d", slot),
			Description:        Msg("CommandPalette.Bookmark.Save.Desc"),
			EnglishDescription: "Store the active panel folder in this bookmark slot",
			ID:                 fmt.Sprintf("Bookmark.Save.%d", slot),
			Category:           category,
			Shortcut:           fmt.Sprintf("Right Ctrl+Shift+%d, Ctrl+Alt+Shift+%d", slot, slot),
			SearchFields: append(append([]string(nil), aliases...), commandPaletteTranslations(
				"CommandPalette.Bookmark.Save", "CommandPalette.Bookmark.Save.Desc",
			)...),
			run: func() bool {
				return runCommandPaletteBookmarkSave(pf, slot)
			},
		})
	}
	entries = append(entries, commandPaletteEntry{
		Key:                "bookmark:home",
		Label:              Msg("CommandPalette.Bookmark.Home"),
		EnglishLabel:       "Go to home folder",
		Description:        Msg("CommandPalette.Bookmark.Home.Desc"),
		EnglishDescription: "Open the home folder in the active panel",
		ID:                 "Bookmark.Home",
		Category:           category,
		Shortcut:           "Right Ctrl+`, Ctrl+Alt+`",
		SearchFields: append(append([]string(nil), aliases...), commandPaletteTranslations(
			"CommandPalette.Bookmark.Home", "CommandPalette.Bookmark.Home.Desc",
		)...),
		run: func() bool {
			if !commandPaletteBookmarkFrameActive(pf) {
				return false
			}
			home, _ := os.UserHomeDir()
			fsp := pf.getActivePanel()
			if home == "" || fsp == nil {
				return false
			}
			pf.NavigateToPath(fsp, home)
			return true
		},
	})
	return entries
}

func runCommandPaletteBookmarkGoto(pf *PanelsFrame, slot int) bool {
	if !commandPaletteBookmarkFrameActive(pf) || slot < 0 || slot > 9 {
		return false
	}
	bookmarks, err := LoadBookmarks(BookmarksFilePath())
	fsp := pf.getActivePanel()
	if err != nil || fsp == nil || strings.TrimSpace(bookmarks[slot].Path) == "" {
		return false
	}
	pf.NavigateToPath(fsp, bookmarks[slot].Path)
	return true
}

func runCommandPaletteBookmarkSave(pf *PanelsFrame, slot int) bool {
	if !commandPaletteBookmarkFrameActive(pf) || slot < 0 || slot > 9 {
		return false
	}
	fsp := pf.getActivePanel()
	if fsp == nil || fsp.vfs == nil {
		return false
	}
	path := BookmarksFilePath()
	bookmarks, err := LoadBookmarks(path)
	if err != nil {
		return false
	}
	bookmarks[slot] = Bookmark{Path: fsp.vfs.GetPath()}
	return SaveBookmarks(path, bookmarks) == nil
}

func commandPaletteBookmarkFrameActive(pf *PanelsFrame) bool {
	return pf != nil && !pf.closed && vtui.FrameManager != nil && vtui.FrameManager.GetTopFrame() == pf
}
