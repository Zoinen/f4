package theme

import (
	"github.com/unxed/vtui"
)

// UseTableColors makes a table embedded in a dialog follow the dialog
// palette instead of the panel-oriented ColTable palette.
func UseTableColors(table *vtui.Table) {
	table.ColorTextIdx = vtui.ColDialogText
	table.ColorSelectedTextIdx = vtui.ColDialogSelectedButton
	table.ColorItemSelectTextIdx = vtui.ColDialogHighlightText
	table.ColorItemSelectCursorIdx = vtui.ColDialogHighlightSelectedButton
	table.ColorTitleIdx = vtui.ColDialogHighlightText
	table.ColorBoxIdx = vtui.ColDialogBox
	if table.ScrollBar != nil {
		table.ScrollBar.ColorIdx = vtui.ColDialogBox
	}
}

// ThemedForeground applies only a theme color's foreground so row-specific
// accents keep the normal or selected background supplied by the table.
func ThemedForeground(attr uint64, paletteIdx int) uint64 {
	themeAttr := vtui.Palette[paletteIdx]
	if themeAttr&vtui.IsFgRGB != 0 {
		return vtui.SetRGBFore(attr, vtui.GetRGBFore(themeAttr))
	}
	return vtui.SetIndexFore(attr, vtui.GetIndexFore(themeAttr))
}
