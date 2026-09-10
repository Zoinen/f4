package editor

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
	"strings"
)

const (
	ColorerHorzCrossRegion = "def:HorzCross"
	ColorerVertCrossRegion = "def:VertCross"
)

func ColorerIsActive() bool {
	return strings.EqualFold(config.App.EditorHighlighter, "Colorer")
}

func CrossModeAxes(mode int) (horz, vert bool) {
	switch mode {
	case config.ColorerCrossVertical:
		return false, true
	case config.ColorerCrossHorizontal:
		return true, false
	case config.ColorerCrossBoth:
		return true, true
	}
	return false, false
}

func ColorerCrossAttr(region string, base uint64) uint64 {
	if !ColorerIsActive() {
		return base
	}
	// Crosshair colours are part of the render path. Use the cache populated by
	// the asynchronously initialized Colorer session; until then the semantic
	// palette is the correct non-blocking fallback.
	rd := CachedColorerRegionDefine(region)
	if rd == nil {
		return base
	}
	attr := base
	if rd.IsForeSet {
		attr = vtui.SetRGBFore(attr, rd.Fore)
	}
	if rd.IsBackSet {
		attr = vtui.SetRGBBack(attr, rd.Back)
	}
	return attr
}

func EditorCrossAttrs() (horz, vert bool, horzAttr, vertAttr uint64) {
	if !config.App.EditorCrosshair {
		return false, false, 0, 0
	}
	horz, vert = CrossModeAxes(config.App.EditorCrossMode)
	if !horz && !vert {
		return false, false, 0, 0
	}
	base := vtui.Palette[theme.ColEditorCrosshair]
	return horz, vert,
		ColorerCrossAttr(ColorerHorzCrossRegion, base),
		ColorerCrossAttr(ColorerVertCrossRegion, base)
}
