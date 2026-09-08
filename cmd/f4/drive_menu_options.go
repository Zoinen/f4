package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/unxed/vtui"
)

// These flags mirror the useful part of Far's Change Drive Menu Options.
// They are intentionally kept as one persisted bit field so old profiles can
// carry the setting without another configuration section.
const (
	driveMenuShowType uint32 = 1 << iota
	driveMenuShowLabel
	driveMenuUseShellName
	driveMenuShowFilesystem
	driveMenuShowSize
	driveMenuShowSizeFloat
	driveMenuShowNetworkName
	driveMenuShowPlugins
	driveMenuSortPluginsByHotkey
	driveMenuShowRemovable
	driveMenuShowCD
	driveMenuShowRemote
	driveMenuDetectVirtual
	driveMenuShowBookmarks
)

const defaultDriveMenuOptions = driveMenuShowType |
	driveMenuShowLabel |
	driveMenuShowFilesystem |
	driveMenuShowSize |
	driveMenuShowSizeFloat |
	driveMenuShowPlugins |
	driveMenuShowRemovable |
	driveMenuShowCD |
	driveMenuShowRemote |
	driveMenuDetectVirtual |
	driveMenuShowBookmarks

func parseDriveMenuOptions(value string) uint32 {
	if strings.TrimSpace(value) == "" {
		return defaultDriveMenuOptions
	}
	var options uint32
	if _, err := fmt.Sscanf(value, "%d", &options); err != nil {
		return defaultDriveMenuOptions
	}
	return options
}

type driveMenuKind uint8

const (
	driveMenuKindUnknown driveMenuKind = iota
	driveMenuKindFixed
	driveMenuKindRemovable
	driveMenuKindCD
	driveMenuKindRemote
	driveMenuKindSubstitute
	driveMenuKindPhysical
	driveMenuKindRAM
)

type driveMenuOptionSpec struct {
	flag  uint32
	label string
}

var driveMenuOptionSpecs = []driveMenuOptionSpec{
	{driveMenuShowType, "Drive.ShowType"},
	{driveMenuShowLabel, "Drive.ShowLabel"},
	{driveMenuUseShellName, "Drive.UseShellName"},
	{driveMenuShowFilesystem, "Drive.ShowFilesystem"},
	{driveMenuShowSize, "Drive.ShowSize"},
	{driveMenuShowSizeFloat, "Drive.ShowSizeFloat"},
	{driveMenuShowNetworkName, "Drive.ShowNetworkName"},
	{driveMenuShowPlugins, "Drive.ShowPlugins"},
	{driveMenuSortPluginsByHotkey, "Drive.SortPluginsByHotkey"},
	{driveMenuShowRemovable, "Drive.ShowRemovable"},
	{driveMenuShowCD, "Drive.ShowCD"},
	{driveMenuShowRemote, "Drive.ShowRemote"},
	{driveMenuDetectVirtual, "Drive.DetectVirtual"},
	{driveMenuShowBookmarks, "Drive.ShowBookmarks"},
}

func driveMenuOptionEnabled(options, flag uint32) bool { return options&flag != 0 }

func driveMenuNameWithoutMarker(name string) string {
	return strings.TrimSpace(strings.ReplaceAll(name, "&", ""))
}

func driveMenuBaseName(name string) string {
	name = driveMenuNameWithoutMarker(name)
	switch {
	case strings.HasPrefix(name, "/ Root"):
		return "/"
	case strings.HasPrefix(name, "~ Home"):
		return "~"
	case len(name) >= 2 && name[1] == ':':
		return name[:2]
	default:
		return name
	}
}

func driveMenuInfoPath(name string) string {
	clean := driveMenuNameWithoutMarker(name)
	switch {
	case strings.HasPrefix(clean, "/ Root"):
		return "/"
	case strings.HasPrefix(clean, "~ Home"):
		home, _ := os.UserHomeDir()
		return home
	case len(clean) >= 2 && clean[1] == ':':
		// GetDiskFreeSpaceEx and GetVolumeInformation both want a root.
		return clean[:2] + string(os.PathSeparator)
	default:
		return ""
	}
}

func driveMenuKindFor(name, path string) driveMenuKind {
	if strings.Contains(strings.ToLower(name), "physical disk") {
		return driveMenuKindPhysical
	}
	if path == "" {
		return driveMenuKindUnknown
	}
	return driveMenuPlatformKind(path)
}

func driveMenuKindLabel(kind driveMenuKind) string {
	switch kind {
	case driveMenuKindFixed, driveMenuKindRAM:
		return Msg("Drive.TypeFixed")
	case driveMenuKindRemovable:
		return Msg("Drive.TypeRemovable")
	case driveMenuKindCD:
		return Msg("Drive.TypeCD")
	case driveMenuKindRemote:
		return Msg("Drive.TypeRemote")
	case driveMenuKindPhysical:
		// The row already says "Physical Disks"; repeating "physical" is
		// noise and is not how Far presents this synthetic entry.
		return ""
	case driveMenuKindSubstitute:
		return Msg("Drive.TypeSubstitute")
	default:
		return ""
	}
}

func driveMenuSize(b uint64, decimal bool) string {
	if decimal {
		return formatBytesHuman(b)
	}
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(1024), 0
	for n := b / 1024; n >= 1024 && exp < 6; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%d %ciB", b/div, "KMGTPE"[exp])
}

type driveMenuPlatformRow struct {
	isDrive                       bool
	base, kind, label, filesystem string
	total, free, network          string
	icon                          string
	totalBytes, freeBytes         uint64
}

type driveMenuPlatformColumn struct {
	text       string
	rightAlign bool
}

// driveMenuPlatformRowFor collects the metadata for one built-in drive. fsInfo
// is deliberately used only for built-in local rows: plugin VFSes can be
// remote and may block while resolving their metadata.
func driveMenuPlatformRowFor(drv DriveEntry, options uint32) driveMenuPlatformRow {
	row := driveMenuPlatformRow{base: driveMenuBaseName(drv.Name)}
	path := driveMenuInfoPath(drv.Name)
	row.isDrive = path != ""
	kind := driveMenuKindFor(drv.Name, path)
	row.icon = driveMenuKindIcon(kind)
	if path == "" && drv.Icon != "" {
		row.icon = drv.Icon
	}

	if driveMenuOptionEnabled(options, driveMenuShowType) {
		row.kind = driveMenuKindLabel(kind)
	}

	info, infoOK := FSInfo{}, false
	if path != "" {
		info, infoOK = fsInfo(path)
	}
	if infoOK {
		if driveMenuOptionEnabled(options, driveMenuShowLabel) && info.Label != "" {
			row.label = info.Label
		}
		if driveMenuOptionEnabled(options, driveMenuShowFilesystem) && info.Type != "" {
			row.filesystem = info.Type
		}
		if driveMenuOptionEnabled(options, driveMenuShowSize) {
			decimal := driveMenuOptionEnabled(options, driveMenuShowSizeFloat)
			row.totalBytes, row.freeBytes = info.Total, info.Free
			row.total = driveMenuSize(info.Total, decimal)
			row.free = driveMenuSize(info.Free, decimal)
		}
		if driveMenuOptionEnabled(options, driveMenuShowNetworkName) && info.Mount != "" && info.Mount != path {
			row.network = info.Mount
		}
	}
	return row
}

func (row driveMenuPlatformRow) columns(options uint32) []driveMenuPlatformColumn {
	columns := make([]driveMenuPlatformColumn, 0, 7)
	if driveMenuOptionEnabled(options, driveMenuShowType) {
		columns = append(columns, driveMenuPlatformColumn{text: row.kind})
	}
	if driveMenuOptionEnabled(options, driveMenuShowLabel) {
		columns = append(columns, driveMenuPlatformColumn{text: row.label})
	}
	if driveMenuOptionEnabled(options, driveMenuShowFilesystem) {
		columns = append(columns, driveMenuPlatformColumn{text: row.filesystem})
	}
	if driveMenuOptionEnabled(options, driveMenuShowSize) {
		columns = append(columns,
			driveMenuPlatformColumn{text: row.total, rightAlign: true},
			driveMenuPlatformColumn{text: row.free, rightAlign: true})
	}
	if driveMenuOptionEnabled(options, driveMenuShowNetworkName) {
		columns = append(columns, driveMenuPlatformColumn{text: row.network})
	}
	return columns
}

func driveMenuPlatformRowHasDetails(columns []driveMenuPlatformColumn) bool {
	for _, column := range columns {
		if column.text != "" {
			return true
		}
	}
	return false
}

func driveMenuPadColumn(column driveMenuPlatformColumn, width int, last bool) string {
	if last {
		if column.rightAlign {
			return strings.Repeat(" ", width-vtui.StringWidth(column.text)) + column.text
		}
		return column.text
	}
	padding := width - vtui.StringWidth(column.text)
	if column.rightAlign {
		return strings.Repeat(" ", padding) + column.text
	}
	return column.text + strings.Repeat(" ", padding)
}

// driveMenuPlatformRowsText renders the platform rows as Far-style columns.
// Widths are calculated across the complete visible platform list, so a long
// label on one drive no longer makes the following columns jump between rows.
func driveMenuPlatformRowsText(rows []driveMenuPlatformRow, options uint32) []string {
	columnWidths := make([]int, 0, 7)
	rowColumns := make([][]driveMenuPlatformColumn, len(rows))
	for i, row := range rows {
		rowColumns[i] = row.columns(options)
		if len(rowColumns[i]) > len(columnWidths) {
			columnWidths = append(columnWidths, make([]int, len(rowColumns[i])-len(columnWidths))...)
		}
		for column, value := range rowColumns[i] {
			if width := vtui.StringWidth(value.text); width > columnWidths[column] {
				columnWidths[column] = width
			}
		}
	}

	texts := make([]string, len(rows))
	for i, row := range rows {
		columns := rowColumns[i]
		if !driveMenuPlatformRowHasDetails(columns) {
			texts[i] = row.base
			continue
		}
		last := len(columns) - 1
		for column := last; column >= 0; column-- {
			if columns[column].text != "" {
				last = column
				break
			}
		}
		parts := make([]string, 1, last+2)
		parts[0] = row.base
		for column := 0; column <= last; column++ {
			parts = append(parts, driveMenuPadColumn(columns[column], columnWidths[column], column == last))
		}
		texts[i] = strings.Join(parts, " | ")
	}
	return texts
}

// driveMenuPlatformItemText is kept for callers and small formatting tests;
// the live menu uses driveMenuPlatformRowsText so all rows share widths.
func driveMenuPlatformItemText(drv DriveEntry, options uint32) string {
	return driveMenuPlatformRowsText([]driveMenuPlatformRow{driveMenuPlatformRowFor(drv, options)}, options)[0]
}

func driveMenuOptionsDialogSize() (int, int) {
	width := vtui.StringWidth(Msg("Drive.OptionsTitle")) + 6
	for _, spec := range driveMenuOptionSpecs {
		label, _, _ := vtui.ParseAmpersandString(Msg(spec.label))
		// Four columns are the checkbox prefix and four are dialog chrome.
		if candidate := vtui.StringWidth(label) + 8; candidate > width {
			width = candidate
		}
	}
	buttonWidth := vtui.StringWidth(Msg("vtui.Ok")) + vtui.StringWidth(Msg("vtui.Cancel")) + 8
	if buttonWidth > width {
		width = buttonWidth
	}
	return width, len(driveMenuOptionSpecs) + 7
}

func driveMenuPlatformItemVisible(drv DriveEntry, options uint32) bool {
	kind := driveMenuKindFor(drv.Name, driveMenuInfoPath(drv.Name))
	switch kind {
	case driveMenuKindRemovable:
		return driveMenuOptionEnabled(options, driveMenuShowRemovable)
	case driveMenuKindCD:
		return driveMenuOptionEnabled(options, driveMenuShowCD)
	case driveMenuKindRemote:
		return driveMenuOptionEnabled(options, driveMenuShowRemote)
	default:
		return true
	}
}

func (pf *PanelsFrame) openDriveMenuOptions(panelIdx int, menu *vtui.VMenu) {
	width, height := driveMenuOptionsDialogSize()
	dlg := vtui.NewCenteredDialog(width, height, Msg("Drive.OptionsTitle"))
	dlg.ShowClose = true

	options := AppConfig.DriveMenuOptions
	checks := make([]*vtui.Checkbox, 0, len(driveMenuOptionSpecs))
	for _, spec := range driveMenuOptionSpecs {
		check := vtui.NewCheckbox(0, 0, Msg(spec.label), false)
		if driveMenuOptionEnabled(options, spec.flag) {
			check.State = 1
		}
		checks = append(checks, check)
		dlg.AddItem(check)
	}

	ok := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	ok.IsDefault = true
	cancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	dlg.AddItem(ok)
	dlg.AddItem(cancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	for _, check := range checks {
		vbox.Add(check, vtui.Margins{}, vtui.AlignLeft)
	}
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(ok, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	dlg.SetFocusedItem(checks[0])

	cancel.OnClick = func() { dlg.Close() }
	ok.OnClick = func() {
		var updated uint32
		for i, check := range checks {
			if check.State == 1 {
				updated |= driveMenuOptionSpecs[i].flag
			}
		}
		AppConfig.DriveMenuOptions = updated
		SaveConfig()
		pos := menu.SelectPos
		dlg.Close()
		menu.Close()
		vtui.FrameManager.PostTask(func() { pf.showDriveMenuAt(panelIdx, pos) })
	}

	vtui.FrameManager.Push(dlg)
}

// Preserve the named fields before the console renderer pads them to cells.
func driveMenuKindIcon(kind driveMenuKind) string {
	switch kind {
	case driveMenuKindFixed:
		return "hard-drive"
	case driveMenuKindRemovable:
		return "usb-flash-drive"
	case driveMenuKindCD:
		return "disc"
	case driveMenuKindRemote:
		return "network"
	case driveMenuKindSubstitute:
		return "folder-symlink"
	case driveMenuKindPhysical:
		return "database"
	case driveMenuKindRAM:
		return "memory-stick"
	default:
		return "circle-question-mark"
	}
}

func (row driveMenuPlatformRow) semanticDetails() map[string]string {
	details := map[string]string{"name": row.base, "type": row.kind, "label": row.label,
		"filesystem": row.filesystem, "total": row.total, "free": row.free, "network": row.network,
		"icon": row.icon, "isDrive": fmt.Sprint(row.isDrive)}
	// Send a bounded ratio from raw bytes, never from rounded display strings.
	if row.totalBytes > 0 && row.total != "" {
		free := min(row.freeBytes, row.totalBytes)
		details["usedFraction"] = fmt.Sprintf("%.9f", float64(row.totalBytes-free)/float64(row.totalBytes))
	}
	return details
}
