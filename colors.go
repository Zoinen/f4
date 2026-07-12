package main

import "github.com/unxed/vtui"

const (
	ColPanelText = vtui.LastPaletteColor + iota
	ColPanelSelectedText
	ColPanelHighlightText
	ColPanelInfoText
	ColPanelCursor
	ColPanelSelectedCursor
	ColPanelTitle
	ColPanelSelectedTitle
	ColPanelColumnTitle
	ColPanelTotalInfo
	ColPanelSelectedInfo

	ColCommandLineUserScreen
	ColPanelBox
	ColPanelScrollbar
	ColPanelDir

	ColCommandLinePrompt
	ColCommandLineText
	ColCommandLineSelectedText

	ColViewerText
	ColViewerStatus
	ColViewerArrows
	ColViewerScrollbar

	ColEditorCrosshair

	LastF4PaletteColor
)

// SetDefaultF4Palette ensures the palette is large enough and sets f4-specific default colors.
func SetDefaultF4Palette() {
	// Initialize ThemePalette to match the "Tango/Ubuntu" gray theme from far2l
	vtui.ThemePalette[0] = 0x2E3436 // Default Background (Dark Gray)
	vtui.ThemePalette[7] = 0xD3D7CF // Default Text (Light Gray)
	vtui.ThemePalette[8] = 0x555753 // Bold/Intensity Background

	if len(vtui.Palette) < LastF4PaletteColor {
		newPal := make([]uint64, LastF4PaletteColor)
		copy(newPal, vtui.Palette)
		vtui.Palette = newPal
	}

	black := uint32(0x000000)
	//white := uint32(0xFFFFFF)
	cyan := uint32(0x00A0A0)
	blue := uint32(0x0000A0)
	yellow := uint32(0xFFFF00)
	//lightGray := uint32(0xC0C0C0)

	// Panels (LightCyan on Blue)
	vtui.Palette[ColPanelText] = vtui.SetRGBBoth(0, 0x00FFFF, blue)
	vtui.Palette[ColPanelSelectedText] = vtui.SetRGBBoth(0, yellow, blue)
	vtui.Palette[ColPanelCursor] = vtui.SetRGBBoth(0, black, cyan)
	vtui.Palette[ColPanelSelectedCursor] = vtui.SetRGBBoth(0, yellow, cyan)
	vtui.Palette[ColPanelBox] = vtui.SetRGBBoth(0, 0x00FFFF, blue)
	vtui.Palette[ColPanelTitle] = vtui.SetRGBBoth(0, 0x00FFFF, blue)
	vtui.Palette[ColPanelColumnTitle] = vtui.SetRGBBoth(0, yellow, blue)

	vtui.Palette[ColPanelHighlightText] = vtui.Palette[ColPanelText]
	vtui.Palette[ColPanelInfoText] = vtui.Palette[ColPanelText]
	vtui.Palette[ColPanelSelectedTitle] = vtui.Palette[ColPanelTitle]
	vtui.Palette[ColPanelTotalInfo] = vtui.Palette[ColPanelText]
	vtui.Palette[ColPanelDir] = vtui.SetRGBBoth(0, 0xFFFFFF, blue)
	vtui.Palette[ColPanelSelectedInfo] = vtui.Palette[ColPanelSelectedText]
	vtui.Palette[ColPanelScrollbar] = vtui.Palette[ColPanelBox]

	// Command line / User screen (Using terminal default background, Index 0)
	vtui.Palette[ColCommandLineUserScreen] = vtui.SetIndexBoth(0, 7, 0)
	vtui.Palette[ColCommandLinePrompt] = vtui.SetIndexBoth(0, 11, 0)       // Light Cyan on Black
	vtui.Palette[ColCommandLineText] = vtui.SetIndexBoth(0, 15, 0)         // White on Black
	vtui.Palette[ColCommandLineSelectedText] = vtui.SetIndexBoth(0, 0, 11) // Black on Light Cyan

	// Editor selection: inverse of UserScreen
	vtui.Palette[vtui.ColDialogEditSelected] = vtui.SetIndexBoth(0, 0, 7)
	// KeyBar (Match far2l: LightGray on DarkGray for numbers, DarkGray on Teal for labels)
	vtui.Palette[vtui.ColKeyBarNum] = vtui.SetRGBBoth(0, 0xD3D7CF, 0x2E3436)
	vtui.Palette[vtui.ColKeyBarText] = vtui.SetRGBBoth(0, 0x2E3436, 0x06989A)

	// Viewer (Match far2l: LightGray on Black)
	vtui.Palette[ColViewerText] = vtui.SetIndexBoth(0, 7, 0)
	vtui.Palette[ColViewerStatus] = vtui.SetIndexBoth(0, 0, 7)
	vtui.Palette[ColViewerArrows] = vtui.SetIndexBoth(0, 14, 0)
	vtui.Palette[ColViewerScrollbar] = vtui.SetIndexBoth(0, 8, 0)

	vtui.Palette[ColEditorCrosshair] = vtui.SetRGBBoth(0, 0xD3D7CF, 0x222222)
}

// colorMap links farcolors.ini keys to vtui.Palette indices.
var colorMap = map[string]int{
	"Menu.Text":                        vtui.ColMenuText,
	"Menu.Text.Selected":               vtui.ColMenuSelectedText,
	"Menu.Highlight":                   vtui.ColMenuHighlight,
	"Menu.Highlight.Selected":          vtui.ColMenuSelectedHighlight,
	"Menu.Box":                         vtui.ColMenuBox,
	"Menu.Title":                       vtui.ColMenuTitle,
	"MenuBar.Text":                     vtui.ColMenuBarItem,
	"MenuBar.Text.Selected":            vtui.ColMenuBarSelected,
	"MenuBar.Highlight":                vtui.ColMenuBarHighlight,
	"MenuBar.Highlight.Selected":       vtui.ColMenuBarSelectedHighlight,
	"Table.Box":                        vtui.ColTableBox,
	"Scrollbar":                        vtui.ColTableBox,
	"Panel.Text":                       ColPanelText,
	"Panel.Text.Selected":              ColPanelSelectedText,
	"Panel.Text.Highlight":             ColPanelHighlightText,
	"Panel.Text.Info":                  ColPanelInfoText,
	"Panel.Cursor":                     ColPanelCursor,
	"Panel.Cursor.Selected":            ColPanelSelectedCursor,
	"Panel.Title":                      ColPanelTitle,
	"Panel.Title.Selected":             ColPanelSelectedTitle,
	"Panel.Title.Column":               ColPanelColumnTitle,
	"Panel.Box":                        ColPanelBox,
	"Panel.Scrollbar":                  ColPanelScrollbar,
	"Panel.Dir":                        ColPanelDir,
	"Dialog.Text":                      vtui.ColDialogText,
	"Dialog.Highlight":                 vtui.ColDialogHighlightText,
	"Dialog.Box":                       vtui.ColDialogBox,
	"Dialog.Box.Title":                 vtui.ColDialogBoxTitle,
	"Dialog.Box.Title.Highlight":       vtui.ColDialogHighlightBoxTitle,
	"Dialog.Edit":                      vtui.ColDialogEdit,
	"Dialog.Button":                    vtui.ColDialogButton,
	"Dialog.Button.Selected":           vtui.ColDialogSelectedButton,
	"Dialog.Button.Highlight":          vtui.ColDialogHighlightButton,
	"Dialog.Button.Highlight.Selected": vtui.ColDialogHighlightSelectedButton,
	"Dialog.Edit.Unchanged":            vtui.ColDialogEditUnchanged,
	"Dialog.Edit.Selected":             vtui.ColDialogEditSelected,
	"CommandLine.UserScreen":           ColCommandLineUserScreen,
	"CommandLine.Prompt":               ColCommandLinePrompt,
	"CommandLine.Text":                 ColCommandLineText,
	"CommandLine.Text.Selected":        ColCommandLineSelectedText,
	"KeyBar.Numbers":                   vtui.ColKeyBarNum,
	"KeyBar.Labels":                    vtui.ColKeyBarText,
	"Viewer.Text":                      ColViewerText,
	"Viewer.Status":                    ColViewerStatus,
	"Viewer.Arrows":                    ColViewerArrows,
	"Viewer.Scrollbar":                 ColViewerScrollbar,
	"Editor.Scrollbar":                 vtui.ColTableBox,
}

// InitColors parses the farcolors section and applies it to the vtui.Palette
func InitColors(ini *IniFile) {
	for key, idx := range colorMap {
		expr := ini.GetString("farcolors", key, "")
		if expr != "" {
			vtui.Palette[idx] = ParseFarColor(expr, vtui.Palette[idx])
		}
	}

	// Terminal history uses indexed background color 0 for default and blank cells.
	// Keep it in sync with the configurable user-screen background.
	vtui.ThemePalette[0] = vtui.GetRGBBack(vtui.Palette[ColCommandLineUserScreen])
}
