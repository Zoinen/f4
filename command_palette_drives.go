package main

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
)

const (
	commandPaletteDriveLeft = iota
	commandPaletteDriveRight
)

func commandPaletteDriveEntries(pf *PanelsFrame) []commandPaletteEntry {
	if pf == nil {
		return nil
	}

	entries := commandPaletteDrivePair(pf, "other", "Panel.Other", plainLabel(Msg("Panel.Other")), func(panelIndex int) bool {
		return executeCommandPaletteOtherPanel(pf, panelIndex)
	}, "Panel.Other")
	for _, drive := range getPlatformDrives() {
		registryName := drive.Name
		displayName := commandPaletteDriveDisplayName(registryName)
		if displayName == "" || drive.Factory == nil {
			continue
		}
		entries = append(entries, commandPaletteDrivePair(pf, "platform", "Platform."+registryName, displayName, func(panelIndex int) bool {
			return executeCommandPalettePlatformDrive(pf, panelIndex, registryName)
		})...)
	}
	if bookmarks, err := LoadBookmarks(BookmarksFilePath()); err == nil {
		for slot := range bookmarks {
			if bookmarks[slot].IsEmpty() || strings.TrimSpace(bookmarks[slot].Path) == "" {
				continue
			}
			path := bookmarks[slot].Path
			displayName := fmt.Sprintf("%s %d: %s", plainLabel(Msg("Menu.Commands.Bookmarks")), slot, path)
			entries = append(entries, commandPaletteDrivePair(pf, "bookmark", fmt.Sprintf("Bookmark.%d", slot), displayName, func(panelIndex int) bool {
				return executeCommandPaletteBookmark(pf, panelIndex, slot)
			}, "Menu.Commands.Bookmarks")...)
		}
	}

	drives := driveRegistrySnapshot()
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

func commandPaletteDrivePair(pf *PanelsFrame, source, id, displayName string, run func(panelIndex int) bool, extraTranslationKeys ...string) []commandPaletteEntry {
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
		category := Msg("CommandPalette.CategoryDrive")
		searchFields := []string{id, displayName, category, Msg("Drive.Title"), Msg(sideDescriptionKey)}
		searchFields = append(searchFields, commandPaletteTranslations(translationKeys...)...)
		entries = append(entries, commandPaletteEntry{
			Key:                fmt.Sprintf("drive:%s:%s:%s", sideKey, source, normalizeCommandPaletteText(id)),
			Label:              fmt.Sprintf(Msg(formatKey), displayName),
			EnglishLabel:       englishLabel,
			Description:        Msg(sideDescriptionKey),
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
func executeCommandPaletteDrive(pf *PanelsFrame, panelIndex int, registryName string) bool {
	if !commandPaletteDrivePanelValid(pf, panelIndex) {
		return false
	}
	var factory func() vfs.VFS
	for _, drive := range driveRegistrySnapshot() {
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

func executeCommandPalettePlatformDrive(pf *PanelsFrame, panelIndex int, name string) bool {
	if !commandPaletteDrivePanelValid(pf, panelIndex) {
		return false
	}
	for _, drive := range getPlatformDrives() {
		if drive.Name == name && drive.Factory != nil {
			return switchCommandPaletteDriveVFS(pf, panelIndex, drive.Factory())
		}
	}
	return false
}

func commandPaletteDrivePanelValid(pf *PanelsFrame, panelIndex int) bool {
	if pf == nil || pf.closed || findPanelsFrameAnyScreen() != pf ||
		panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		return false
	}
	fsp, ok := pf.panels[panelIndex].(*FileSystemPanel)
	return ok && fsp != nil
}

func switchCommandPaletteDriveVFS(pf *PanelsFrame, panelIndex int, newVFS vfs.VFS) bool {
	if pf == nil || pf.closed || panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		if newVFS != nil {
			newVFS.Close()
		}
		return false
	}
	fsp, ok := pf.panels[panelIndex].(*FileSystemPanel)
	if !ok || fsp == nil {
		if newVFS != nil {
			newVFS.Close()
		}
		return false
	}
	if newVFS == nil {
		return false
	}
	pf.switchToVFS(fsp, newVFS)
	return true
}

func executeCommandPaletteOtherPanel(pf *PanelsFrame, panelIndex int) bool {
	if pf == nil || pf.closed || panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		return false
	}
	other, ok := pf.panels[1-panelIndex].(*FileSystemPanel)
	if !ok || other == nil || other.vfs == nil {
		return false
	}
	return switchCommandPaletteDriveVFS(pf, panelIndex, other.vfs.Clone())
}

func executeCommandPaletteBookmark(pf *PanelsFrame, panelIndex, slot int) bool {
	if pf == nil || pf.closed || panelIndex < commandPaletteDriveLeft || panelIndex > commandPaletteDriveRight {
		return false
	}
	fsp, ok := pf.panels[panelIndex].(*FileSystemPanel)
	if !ok || fsp == nil {
		return false
	}
	bookmarks, err := LoadBookmarks(BookmarksFilePath())
	if err != nil || slot < 0 || slot >= len(bookmarks) || bookmarks[slot].IsEmpty() || strings.TrimSpace(bookmarks[slot].Path) == "" {
		return false
	}
	pf.NavigateToPath(fsp, bookmarks[slot].Path)
	return true
}
