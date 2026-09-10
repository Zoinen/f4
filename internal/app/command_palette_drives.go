package app

import (
	"fmt"
	"github.com/unxed/f4/internal/panel"
	"strings"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

const (
	commandPaletteDriveLeft = iota
	commandPaletteDriveRight
)

func commandPaletteDriveEntries(pf *panel.PanelsFrame) []commandPaletteEntry {
	if pf == nil {
		return nil
	}

	entries := commandPaletteDrivePair(pf, "other", "Panel.Other", action.PlainLabel(i18n.Msg("Panel.Other")), func(panelIndex int) bool {
		return executeCommandPaletteOtherPanel(pf, panelIndex)
	}, "Panel.Other")
	for _, drive := range sysinfo.GetPlatformDrives() {
		registryName := drive.Name
		displayName := commandPaletteDriveDisplayName(registryName)
		if displayName == "" || drive.Factory == nil {
			continue
		}
		entries = append(entries, commandPaletteDrivePair(pf, "platform", "Platform."+registryName, displayName, func(panelIndex int) bool {
			return executeCommandPalettePlatformDrive(pf, panelIndex, registryName)
		})...)
	}
	if bookmarks, err := panel.LoadBookmarks(panel.BookmarksFilePath()); err == nil {
		for slot := range bookmarks {
			if bookmarks[slot].IsEmpty() || strings.TrimSpace(bookmarks[slot].Path) == "" {
				continue
			}
			path := bookmarks[slot].Path
			displayName := fmt.Sprintf("%s %d: %s", action.PlainLabel(i18n.Msg("Menu.Commands.Bookmarks")), slot, path)
			entries = append(entries, commandPaletteDrivePair(pf, "bookmark", fmt.Sprintf("Bookmark.%d", slot), displayName, func(panelIndex int) bool {
				return executeCommandPaletteBookmark(pf, panelIndex, slot)
			}, "Menu.Commands.Bookmarks")...)
		}
	}

	drives := sysinfo.DriveRegistrySnapshot()
	for _, drive := range drives {
		registryName := drive.Name
		displayName := commandPaletteDriveDisplayName(registryName)
		if displayName == "" || drive.Factory == nil {
			continue
		}
		entries = append(entries, commandPaletteDrivePair(pf, "registry", registryName, displayName, func(panelIndex int) bool {
			return executeCommandPaletteDrive(pf, panelIndex, registryName)
		})...)
	}
	return entries
}

func commandPaletteDrivePair(pf *panel.PanelsFrame, source, id, displayName string, run func(panelIndex int) bool, extraTranslationKeys ...string) []commandPaletteEntry {
	entries := make([]commandPaletteEntry, 0, 2)
	for panelIndex := commandPaletteDriveLeft; panelIndex <= commandPaletteDriveRight; panelIndex++ {
		panelIndex := panelIndex
		formatKey := "CommandPalette.DriveLeft"
		sideKey := "left"
		sideDescriptionKey := "Action.Panel.LeftDriveMenu.Desc"
		englishLabel := fmt.Sprintf("Open %s in left panel", displayName)
		englishDescription := "Show the drive menu for the left panel"
		if panelIndex == commandPaletteDriveRight {
			formatKey = "CommandPalette.DriveRight"
			sideKey = "right"
			sideDescriptionKey = "Action.Panel.RightDriveMenu.Desc"
			englishLabel = fmt.Sprintf("Open %s in right panel", displayName)
			englishDescription = "Show the drive menu for the right panel"
		}
		translationKeys := []string{"CommandPalette.CategoryDrive", formatKey, "Drive.Title", sideDescriptionKey}
		translationKeys = append(translationKeys, extraTranslationKeys...)
		category := i18n.Msg("CommandPalette.CategoryDrive")
		searchFields := []string{id, displayName, category, i18n.Msg("Drive.Title"), i18n.Msg(sideDescriptionKey)}
		searchFields = append(searchFields, commandPaletteTranslations(translationKeys...)...)
		entries = append(entries, commandPaletteEntry{
			Key:                fmt.Sprintf("drive:%s:%s:%s", sideKey, source, normalizeCommandPaletteText(id)),
			Label:              fmt.Sprintf(i18n.Msg(formatKey), displayName),
			EnglishLabel:       englishLabel,
			Description:        i18n.Msg(sideDescriptionKey),
			EnglishDescription: englishDescription,
			ID:                 id,
			Category:           category,
			SearchFields:       searchFields,
			panels:             pf,
			run:                func() bool { return run(panelIndex) },
		})
	}
	return entries
}

func commandPaletteDriveDisplayName(name string) string {
	if index := strings.Index(name, ". "); index >= 0 {
		name = name[index+2:]
	}
	return strings.TrimSpace(strings.ReplaceAll(name, "&", ""))
}

// executeCommandPaletteDrive deliberately re-resolves the named drive. A
// command palette may stay open while a plugin replaces or removes its drive;
// retaining the old Factory would call unloaded plugin code.
func executeCommandPaletteDrive(pf *panel.PanelsFrame, panelIndex int, registryName string) bool {
	if !commandPaletteDrivePanelValid(pf, panelIndex) {
		return false
	}
	var factory func() vfs.VFS
	for _, drive := range sysinfo.DriveRegistrySnapshot() {
		if drive.Name == registryName {
			factory = drive.Factory
			break
		}
	}
	if factory == nil {
		return false
	}
	return switchCommandPaletteDriveVFS(pf, panelIndex, factory())
}

func executeCommandPalettePlatformDrive(pf *panel.PanelsFrame, panelIndex int, name string) bool {
	if !commandPaletteDrivePanelValid(pf, panelIndex) {
		return false
	}
	for _, drive := range sysinfo.GetPlatformDrives() {
		if drive.Name == name && drive.Factory != nil {
			return switchCommandPaletteDriveVFS(pf, panelIndex, drive.Factory())
		}
	}
	return false
}

func commandPaletteDrivePanelValid(pf *panel.PanelsFrame, panelIndex int) bool {
	if pf == nil || pf.Closed || panel.FindPanelsFrameAnyScreen() != pf ||
		panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		return false
	}
	fsp, ok := pf.Panels[panelIndex].(*panel.FileSystemPanel)
	return ok && fsp != nil
}

func switchCommandPaletteDriveVFS(pf *panel.PanelsFrame, panelIndex int, newVFS vfs.VFS) bool {
	if pf == nil || pf.Closed || panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		if newVFS != nil {
			newVFS.Close()
		}
		return false
	}
	fsp, ok := pf.Panels[panelIndex].(*panel.FileSystemPanel)
	if !ok || fsp == nil {
		if newVFS != nil {
			newVFS.Close()
		}
		return false
	}
	if newVFS == nil {
		return false
	}
	pf.SwitchToVFS(fsp, newVFS)
	return true
}

func executeCommandPaletteOtherPanel(pf *panel.PanelsFrame, panelIndex int) bool {
	if pf == nil || pf.Closed || panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		return false
	}
	other, ok := pf.Panels[1-panelIndex].(*panel.FileSystemPanel)
	if !ok || other == nil || other.Vfs == nil {
		return false
	}
	return switchCommandPaletteDriveVFS(pf, panelIndex, other.Vfs.Clone())
}

func executeCommandPaletteBookmark(pf *panel.PanelsFrame, panelIndex, slot int) bool {
	if pf == nil || pf.Closed || panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		return false
	}
	fsp, ok := pf.Panels[panelIndex].(*panel.FileSystemPanel)
	if !ok || fsp == nil {
		return false
	}
	bookmarks, err := panel.LoadBookmarks(panel.BookmarksFilePath())
	if err != nil || slot < 0 || slot >= len(bookmarks) || bookmarks[slot].IsEmpty() || strings.TrimSpace(bookmarks[slot].Path) == "" {
		return false
	}
	pf.NavigateToBookmark(fsp, bookmarks[slot])
	return true
}
