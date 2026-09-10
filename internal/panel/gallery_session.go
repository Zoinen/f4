package panel

import (
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
}

func CapturePanelGallerySessionState(fsp *FileSystemPanel) PanelGallerySessionState {
	if fsp == nil {
		return DefaultPanelGallerySessionState()
	}
	return ClonePanelGallerySessionState(PanelGallerySessionState{LayoutMode: fsp.GalleryLayoutMode, ColumnCount: fsp.GalleryColumnCount, Densities: fsp.GalleryDensities})
}

var LastLeftGalleryState = DefaultPanelGallerySessionState()
var LastRightGalleryState = DefaultPanelGallerySessionState()
