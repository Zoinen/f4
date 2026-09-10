package terminal

import (
	"github.com/unxed/vtui"
)

// InvertAttrColors swaps foreground and background colors in a cell
// attribute word, preserving other flags (bold, dim, underline, …).
// Physical swap instead of the CommonLvbReverse flag — GUI renderers
// (X11, Wayland, gogpu) don't inspect that flag, so relying on it
// leaves the selection invisible outside the tty backend.
func InvertAttrColors(attr uint64) uint64 {
	fgIsRGB := attr&vtui.IsFgRGB != 0
	bgIsRGB := attr&vtui.IsBgRGB != 0

	var fgRGB, bgRGB uint32
	var fgIdx, bgIdx uint8
	if fgIsRGB {
		fgRGB = vtui.GetRGBFore(attr)
	} else {
		fgIdx = vtui.GetIndexFore(attr)
	}
	if bgIsRGB {
		bgRGB = vtui.GetRGBBack(attr)
	} else {
		bgIdx = vtui.GetIndexBack(attr)
	}

	result := attr
	if bgIsRGB {
		result = vtui.SetRGBFore(result, bgRGB)
	} else {
		result = vtui.SetIndexFore(result, bgIdx)
	}
	if fgIsRGB {
		result = vtui.SetRGBBack(result, fgRGB)
	} else {
		result = vtui.SetIndexBack(result, fgIdx)
	}
	return result
}
