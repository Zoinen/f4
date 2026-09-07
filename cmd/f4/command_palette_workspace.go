package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unxed/vtui"
)

// Workspace tab reordering is intentionally excluded until vtui exposes a
// semantic action for it. Today it exists only as a captured pointer-drag
// gesture, which cannot be replayed safely from a palette snapshot after tabs
// have moved. Activation and closing instead re-resolve stable screen numbers.
const commandPaletteWorkspaceReorderExclusion = "pointer-drag-only; vtui has no stable workspace reorder semantic action"

// commandPaletteWorkspaceEntries exposes every live workspace as a separate
// activation and close command. Entries capture the stable workspace number,
// not its current slice index: tabs can be reordered while the palette is open.
func commandPaletteWorkspaceEntries() []commandPaletteEntry {
	if vtui.FrameManager == nil || len(vtui.FrameManager.Screens) == 0 {
		return nil
	}

	category := Msg("CommandPalette.CategoryWorkspace")
	aliases := commandPaletteTranslations(
		"CommandPalette.CategoryWorkspace",
		"CommandPalette.Workspace.Switch",
		"CommandPalette.Workspace.Switch.Desc",
		"CommandPalette.Workspace.Close",
		"CommandPalette.Workspace.Close.Desc",
		"AppearanceSettings.WorkspaceTabs",
		"AppearanceSettings.RestoreWorkspaceTabs",
	)
	entries := make([]commandPaletteEntry, 0, len(vtui.FrameManager.Screens)*2)
	for index, screen := range vtui.FrameManager.Screens {
		if screen == nil || screen.Number < 1 {
			continue
		}
		number := screen.Number
		info := screen.GetMenuInfo()
		primary := strings.TrimSpace(info.Primary)
		if primary == "" {
			primary = strings.TrimSpace(screen.GetTabTitle())
		}
		if primary == "" {
			primary = "Workspace"
		}
		secondary := strings.TrimSpace(info.Secondary)
		activateDescription := fmt.Sprintf(Msg("CommandPalette.Workspace.Switch.Desc"), number)
		if secondary != "" {
			activateDescription += ": " + secondary
		}
		closeDescription := fmt.Sprintf(Msg("CommandPalette.Workspace.Close.Desc"), number)
		if secondary != "" {
			closeDescription += ": " + secondary
		}

		searchFields := []string{
			strconv.Itoa(number),
			primary,
			secondary,
			strings.TrimSpace(screen.GetTabTitle()),
			strings.TrimSpace(screen.GetTitle()),
			strings.TrimSpace(info.Icon),
			"workspace",
			"screen",
		}
		searchFields = append(searchFields, aliases...)
		entries = append(entries, commandPaletteEntry{
			Key:                fmt.Sprintf("workspace:activate:%d", number),
			Label:              fmt.Sprintf(Msg("CommandPalette.Workspace.Switch"), number, primary),
			EnglishLabel:       fmt.Sprintf("Switch to workspace %d: %s", number, primary),
			Description:        activateDescription,
			EnglishDescription: fmt.Sprintf("Activate workspace %d", number),
			ID:                 fmt.Sprintf("Workspace.Activate.%d", number),
			Category:           category,
			Shortcut:           strings.Join(workspaceNumberShortcuts(number), ", "),
			SearchFields:       searchFields,
			Checked:            index == vtui.FrameManager.ActiveIdx,
			run:                func() bool { return actionActivateWorkspaceNumber(number) },
		}, commandPaletteEntry{
			Key:                fmt.Sprintf("workspace:close:%d", number),
			Label:              fmt.Sprintf(Msg("CommandPalette.Workspace.Close"), number, primary),
			EnglishLabel:       fmt.Sprintf("Close workspace %d: %s", number, primary),
			Description:        closeDescription,
			EnglishDescription: fmt.Sprintf("Close workspace %d", number),
			ID:                 fmt.Sprintf("Workspace.Close.%d", number),
			Category:           category,
			SearchFields:       append([]string(nil), searchFields...),
			run:                func() bool { return actionWorkspaceCloseNumber(number) },
		})
	}
	return entries
}

func workspaceNumberShortcuts(number int) []string {
	if vtui.FrameManager == nil || !vtui.FrameManager.WorkspaceAltNumberSwitch || number < 1 || number > 9 {
		return nil
	}
	shortcuts := []string{FormatKeyForUI(fmt.Sprintf("Alt%d", number))}
	if vtui.FrameManager.WorkspaceTabMode == vtui.WorkspaceTabsOnCtrl {
		shortcuts = append(shortcuts, FormatKeyForUI(fmt.Sprintf("CtrlAlt%d", number)))
	}
	return mergeCommandPaletteShortcuts(shortcuts)
}
