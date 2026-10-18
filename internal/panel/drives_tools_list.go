package panel

import (
	"sort"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/sysinfo"
)

// DriveToolChoice is one tool of the drive menu as its settings window lists
// it: Key is what the order and visibility files store, Label what it shows.
type DriveToolChoice struct{ Key, Label string }

// DriveMenuToolChoices lists every tool of the drive menu, shown or hidden, in
// the order the menu has them: Other panel, Temporary panel and, where there
// is one, Windows Registry, then the plugins' tools; arranged as the user left
// them with Shift+Up/Down, or with the plugins by name when SortToolsByHotkey
// is on (mitek1977-coder, f4#1148).
func DriveMenuToolChoices() []DriveToolChoice {
	var choices []DriveToolChoice
	platform := driveMenuPlatformDrives()
	// Physical Disks is a tool too, the first one by default (f4#1148).
	for _, drv := range platform {
		if isPhysicalDisksDrive(drv.Name) {
			choices = append(choices, DriveToolChoice{Key: driveToolKeyPhysicalDisks, Label: driveToolLabel(drv.Name)})
			break
		}
	}
	choices = append(choices,
		DriveToolChoice{Key: driveToolKeyOtherPanel, Label: driveToolLabel(i18n.Msg("Panel.Other"))},
		DriveToolChoice{Key: driveToolKeyTemporaryPanel, Label: driveToolLabel(i18n.Msg("TempPanel.Drive"))},
	)
	for _, drv := range platform {
		if strings.EqualFold(driveMenuNameWithoutMarker(drv.Name), "Windows Registry") {
			choices = append(choices, DriveToolChoice{Key: driveToolKeyRegistry, Label: driveToolLabel(drv.Name)})
			break
		}
	}
	plugins := sysinfo.DriveRegistrySnapshot()
	sortByName := DriveToolsSortedByName()
	if sortByName {
		sort.SliceStable(plugins, func(i, j int) bool {
			return strings.ToLower(driveMenuNameWithoutMarker(plugins[i].Name)) <
				strings.ToLower(driveMenuNameWithoutMarker(plugins[j].Name))
		})
	}
	for _, drv := range plugins {
		choices = append(choices, DriveToolChoice{Key: drv.Name, Label: driveToolLabel(drv.Name)})
	}
	if sortByName {
		return choices
	}
	order, err := LoadDriveToolsOrder(DriveToolsOrderFilePath())
	if err != nil || len(order) == 0 {
		return choices
	}
	entries := make([]sysinfo.DriveEntry, len(choices))
	byKey := map[string]DriveToolChoice{}
	for i, c := range choices {
		entries[i] = sysinfo.DriveEntry{Name: c.Key}
		byKey[c.Key] = c
	}
	ordered := make([]DriveToolChoice, 0, len(choices))
	for _, e := range orderDriveTools(entries, order) {
		ordered = append(ordered, byKey[e.Name])
	}
	return ordered
}

// DriveToolsSortedByName reports whether SortToolsByHotkey is on: the plugins'
// tools are then in name order and cannot be moved.
func DriveToolsSortedByName() bool {
	return driveMenuOptionEnabled(config.App.DriveMenuOptions, config.DriveMenuSortPluginsByHotkey)
}

// driveToolLabel is a tool's name without its accelerator mark.
func driveToolLabel(name string) string {
	return strings.TrimSpace(strings.ReplaceAll(name, "&", ""))
}
