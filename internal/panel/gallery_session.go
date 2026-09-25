package panel

import (
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/internal/ini"
	"strings"
)

func loadPanelGallerySessionState(ini *ini.File, section string) PanelGallerySessionState {
	state := DefaultPanelGallerySessionState()
	if mode, ok := ParseGalleryLayoutMode(ini.GetString(section, "GalleryLayout", "")); ok {
		state.LayoutMode = mode
	}
	if raw := ini.GetString(section, "GalleryColumns", ""); raw != "" {
		var columns int
		if _, err := fmt.Sscanf(raw, "%d", &columns); err == nil && columns >= MinGalleryColumnCount && columns <= MaxGalleryColumnCount {
			state.ColumnCount = columns
		}
	}
	for _, mode := range GalleryLayoutModes {
		key := "GalleryDensity" + strings.ToUpper(string(mode[:1])) + string(mode[1:])
		var density int
		if _, err := fmt.Sscanf(ini.GetString(section, key, ""), "%d", &density); err == nil {
			// 128 and then 32 were the untouched Icons defaults. Migrate both
			// exact defaults so existing sessions receive the current 64px
			// cells; preserve every other explicit user zoom value.
			if mode == GalleryLayoutIcons && (density == 128 || density == 32) {
				density = 64
			}
			state.Densities[mode] = ClampGalleryDensity(mode, density)
		}
	}
	if raw := ini.GetString(section, "FileFieldColumns", ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state.FileFieldColumns)
	}
	if raw := ini.GetString(section, "GalleryColumnWidths", ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state.GalleryColumnWidths)
	}
	if raw := ini.GetString(section, "FileFieldFilters", ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state.FileFieldFilters)
	}
	state.FileFieldFilterAny = strings.EqualFold(
		ini.GetString(section, "FileFieldFilterAny", "false"), "true")
	state.FileFieldSort = ini.GetString(section, "FileFieldSort", "")
	state.FileFieldGroup = ini.GetString(section, "FileFieldGroup", "")
	state.FileFieldGroupReverse = strings.EqualFold(
		ini.GetString(section, "FileFieldGroupReverse", "false"), "true")
	return state
}

func LoadUnifiedPanelGallerySessionState(ini *ini.File, section string,
	viewMode ViewMode,
) PanelGallerySessionState {
	state := loadPanelGallerySessionState(ini, section)
	const missing = "\x00"
	presentation := strings.ToLower(strings.TrimSpace(
		ini.GetString(section, "Presentation", missing)))
	galleryLayout := ini.GetString(section, "GalleryLayout", missing)
	if presentation == "gallery" ||
		(presentation == missing && galleryLayout != missing) {
		return state
	}

	switch viewMode {
	case ViewModeBrief:
		state.LayoutMode = GalleryLayoutColumns
		state.ColumnCount = 3
	case ViewModeDetailed:
		state.LayoutMode = GalleryLayoutDetails
	default:
		state.LayoutMode = GalleryLayoutColumns
		state.ColumnCount = 2
	}
	return state
}

func WritePanelGallerySessionState(sb *strings.Builder, state PanelGallerySessionState) {
	state = ClonePanelGallerySessionState(state)
	fmt.Fprintf(sb, "GalleryLayout = %s\nGalleryColumns = %d\n", state.LayoutMode, state.ColumnCount)
	for _, mode := range GalleryLayoutModes {
		if density, ok := state.Densities[mode]; ok {
			key := "GalleryDensity" + strings.ToUpper(string(mode[:1])) + string(mode[1:])
			fmt.Fprintf(sb, "%s = %d\n", key, density)
		}
	}
	if len(state.FileFieldColumns) > 0 {
		if encoded, err := json.Marshal(state.FileFieldColumns); err == nil {
			fmt.Fprintf(sb, "FileFieldColumns = %s\n", encoded)
		}
	}
	if len(state.GalleryColumnWidths) > 0 {
		if encoded, err := json.Marshal(state.GalleryColumnWidths); err == nil {
			fmt.Fprintf(sb, "GalleryColumnWidths = %s\n", encoded)
		}
	}
	if len(state.FileFieldFilters) > 0 {
		if encoded, err := json.Marshal(state.FileFieldFilters); err == nil {
			fmt.Fprintf(sb, "FileFieldFilters = %s\n", encoded)
		}
	}
	if state.FileFieldFilterAny {
		fmt.Fprintln(sb, "FileFieldFilterAny = true")
	}
	if state.FileFieldSort != "" {
		fmt.Fprintf(sb, "FileFieldSort = %s\n", state.FileFieldSort)
	}
	if state.FileFieldGroup != "" {
		fmt.Fprintf(sb, "FileFieldGroup = %s\n", state.FileFieldGroup)
	}
	if state.FileFieldGroupReverse {
		fmt.Fprintln(sb, "FileFieldGroupReverse = true")
	}
}

func RestorePanelGallerySessionState(fsp *FileSystemPanel, saved PanelGallerySessionState) {
	if fsp == nil {
		return
	}
	saved = ClonePanelGallerySessionState(saved)
	fsp.GalleryLayoutMode = saved.LayoutMode
	fsp.GalleryColumnCount = saved.ColumnCount
	fsp.GalleryDensities = CloneGalleryDensities(saved.Densities)
	fsp.GalleryLayoutRevision = 1
	fsp.FileFieldColumns = append([]FileFieldColumn(nil), saved.FileFieldColumns...)
	fsp.GalleryColumnWidths = CloneGalleryColumnWidths(saved.GalleryColumnWidths)
	fsp.FileFieldSort = saved.FileFieldSort
	fsp.FileFieldFilters = append([]FileFieldFilter(nil), saved.FileFieldFilters...)
	fsp.FileFieldFilterAny = saved.FileFieldFilterAny
	fsp.FileFieldGroup = saved.FileFieldGroup
	if saved.FileFieldGroup != "" {
		fsp.SetFileFieldGroup(saved.FileFieldGroup, saved.FileFieldGroupReverse)
	} else if saved.FileFieldSort != "" {
		fsp.sortEntriesKeepingCursor()
	}
	fsp.refilterEntries()
}

func CapturePanelGallerySessionState(fsp *FileSystemPanel) PanelGallerySessionState {
	if fsp == nil {
		return DefaultPanelGallerySessionState()
	}
	return ClonePanelGallerySessionState(PanelGallerySessionState{
		LayoutMode: fsp.GalleryLayoutMode, ColumnCount: fsp.GalleryColumnCount,
		Densities:             fsp.GalleryDensities,
		FileFieldColumns:      fsp.FileFieldColumns,
		GalleryColumnWidths:   fsp.GalleryColumnWidths,
		FileFieldFilters:      fsp.FileFieldFilters,
		FileFieldFilterAny:    fsp.FileFieldFilterAny,
		FileFieldSort:         fsp.FileFieldSort,
		FileFieldGroup:        fsp.FileFieldGroup,
		FileFieldGroupReverse: fsp.GroupBy == GroupFileField && fsp.GroupReverse,
	})
}

var LastLeftGalleryState = DefaultPanelGallerySessionState()
var LastRightGalleryState = DefaultPanelGallerySessionState()
