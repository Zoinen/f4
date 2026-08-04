package main

import (
	"strings"

	"github.com/unxed/vtui"
)

// Cross drawing modes. FarColorer calls the option "Show cross" and lets the
// scheme pick the axes through its parameters; those are not reachable through
// the WASM build, so the axes are chosen by hand instead.
const (
	ColorerCrossOff = iota
	ColorerCrossVertical
	ColorerCrossHorizontal
	ColorerCrossBoth
)

// The regions FarColorer reads the cross colors from.
const (
	colorerHorzCrossRegion = "def:HorzCross"
	colorerVertCrossRegion = "def:VertCross"
)

// colorerCrossModeItems lists the "Show cross" choices in the order the
// ColorerCross* constants define, so that the combo box position is the mode.
func colorerCrossModeItems() []string {
	return []string{
		Msg("ColorerSettings.CrossOff"),
		Msg("ColorerSettings.CrossVertical"),
		Msg("ColorerSettings.CrossHorizontal"),
		Msg("ColorerSettings.CrossBoth"),
	}
}

// colorerIsActive reports whether Colorer is the highlighter in charge.
func colorerIsActive() bool {
	return strings.EqualFold(AppConfig.EditorHighlighter, "Colorer")
}

// crossModeAxes splits a cross mode into its horizontal and vertical parts.
func crossModeAxes(mode int) (horz, vert bool) {
	switch mode {
	case ColorerCrossVertical:
		return false, true
	case ColorerCrossHorizontal:
		return true, false
	case ColorerCrossBoth:
		return true, true
	}
	return false, false
}

// colorerCrossAttr resolves one of the cross regions of the active color
// style. The lookup is exact, the way the editor background one is: a cross
// color guessed from an unrelated region would paint a stripe across the whole
// editor, so a style without the region keeps the f4 palette.
func colorerCrossAttr(region string, base uint64) uint64 {
	if !colorerIsActive() {
		return base
	}
	style, ok := colorerSchemeExactStyle(region)
	if !ok {
		return base
	}
	attr := base
	if style.hasFore {
		attr = vtui.SetRGBFore(attr, style.fore)
	}
	if style.hasBack {
		attr = vtui.SetRGBBack(attr, style.back)
	}
	return attr
}

// EditorCrossAttrs tells the editor which cross lines to draw and in which
// colors. The crosshair checkbox stays the master switch, the mode only picks
// the axes.
func EditorCrossAttrs() (horz, vert bool, horzAttr, vertAttr uint64) {
	if !AppConfig.EditorCrosshair {
		return false, false, 0, 0
	}
	horz, vert = crossModeAxes(AppConfig.EditorCrossMode)
	if !horz && !vert {
		return false, false, 0, 0
	}
	base := vtui.Palette[ColEditorCrosshair]
	return horz, vert,
		colorerCrossAttr(colorerHorzCrossRegion, base),
		colorerCrossAttr(colorerVertCrossRegion, base)
}

func actionColorerSettings(pf *PanelsFrame) {
	width, height := 74, 19
	dlg := vtui.NewCenteredDialog(width, height, Msg("ColorerSettings.Title"))
	dlg.ShowClose = true

	// 1. Initialize Widgets
	chkEnabled := vtui.NewCheckbox(0, 0, Msg("ColorerSettings.Enabled"), false)
	if colorerIsActive() {
		chkEnabled.State = 1
	}

	// The catalog carries a machine name and a human description; the machine
	// name is what the config stores, so the two lists are kept in step.
	schemeNames := []string{""}
	schemeItems := []string{Msg("ColorerSettings.BuiltIn")}
	for _, scheme := range ListColorerSchemes() {
		schemeNames = append(schemeNames, scheme.Name)
		schemeItems = append(schemeItems, colorerSchemeLabel(scheme))
	}
	selectedScheme := 0
	for i := 1; i < len(schemeNames); i++ {
		if strings.EqualFold(schemeNames[i], AppConfig.EditorColorerScheme) {
			selectedScheme = i
			break
		}
	}
	comboScheme := vtui.NewComboBox(0, 0, 44, schemeItems)
	comboScheme.DropdownOnly = true
	comboScheme.Menu.SetSelectPos(selectedScheme)
	comboScheme.Edit.SetText(schemeItems[selectedScheme])
	lblScheme := vtui.NewLabel(0, 0, Msg("ColorerSettings.Style"), comboScheme)

	crossItems := colorerCrossModeItems()
	crossPos := AppConfig.EditorCrossMode
	if crossPos < 0 || crossPos >= len(crossItems) {
		crossPos = ColorerCrossBoth
	}
	comboCross := vtui.NewComboBox(0, 0, 44, crossItems)
	comboCross.DropdownOnly = true
	comboCross.Menu.SetSelectPos(crossPos)
	comboCross.Edit.SetText(crossItems[crossPos])
	lblCross := vtui.NewLabel(0, 0, Msg("ColorerSettings.Cross"), comboCross)

	chkSyntax := vtui.NewCheckbox(0, 0, Msg("ColorerSettings.Syntax"), false)
	if AppConfig.EditorColorerSyntax {
		chkSyntax.State = 1
	}

	chkBackground := vtui.NewCheckbox(0, 0, Msg("ColorerSettings.Background"), false)
	if AppConfig.EditorColorerBackground {
		chkBackground.State = 1
	}

	editCatalog := vtui.NewEdit(0, 0, width-6, AppConfig.EditorColorerCatalog)
	editCatalog.ClearSelection()
	lblCatalog := vtui.NewLabel(0, 0, Msg("ColorerSettings.Catalog"), editCatalog)

	btnReload := vtui.NewButton(0, 0, Msg("ColorerSettings.Reload"))
	btnDownload := vtui.NewButton(0, 0, Msg("ColorerSettings.Download"))
	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	// 2. Add to Dialog in desired focus order
	dlg.AddItem(chkEnabled)
	dlg.AddItem(lblScheme)
	dlg.AddItem(comboScheme)
	dlg.AddItem(lblCross)
	dlg.AddItem(comboCross)
	dlg.AddItem(chkSyntax)
	dlg.AddItem(chkBackground)
	dlg.AddItem(lblCatalog)
	dlg.AddItem(editCatalog)
	dlg.AddItem(btnReload)
	dlg.AddItem(btnDownload)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	// 3. Layout Configuration
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)

	vbox.Add(chkEnabled, vtui.Margins{}, vtui.AlignLeft)

	rowScheme := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowScheme.Add(lblScheme, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowScheme.Add(comboScheme, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowScheme, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowCross := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowCross.Add(lblCross, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowCross.Add(comboCross, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowCross, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowChecks := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowChecks.Add(chkSyntax, vtui.Margins{Right: 2}, vtui.AlignLeft)
	rowChecks.Add(chkBackground, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowChecks, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Add(lblCatalog, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editCatalog, vtui.Margins{}, vtui.AlignFill)

	rowTools := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowTools.HorizontalAlign = vtui.AlignCenter
	rowTools.Spacing = 2
	rowTools.Add(btnReload, vtui.Margins{}, vtui.AlignTop)
	rowTools.Add(btnDownload, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(rowTools, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowButtons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowButtons.HorizontalAlign = vtui.AlignCenter
	rowButtons.Spacing = 2
	rowButtons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	rowButtons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(rowButtons, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Apply()

	// 4. Logic
	apply := func() {
		if chkEnabled.State == 1 {
			AppConfig.EditorHighlighter = "Colorer"
		} else if colorerIsActive() {
			AppConfig.EditorHighlighter = "Chroma"
		}
		AppConfig.EditorColorerScheme = ""
		if pos := comboScheme.Menu.SelectPos; pos > 0 && pos < len(schemeNames) {
			AppConfig.EditorColorerScheme = schemeNames[pos]
		}
		AppConfig.EditorCrossMode = comboCross.Menu.SelectPos
		AppConfig.EditorColorerSyntax = chkSyntax.State == 1
		AppConfig.EditorColorerBackground = chkBackground.State == 1
		AppConfig.EditorColorerCatalog = strings.TrimSpace(editCatalog.GetText())
		// The catalog may now point somewhere else, so the styles are dropped
		// instead of being kept under the same name.
		ResetColorerScheme()
		SetColorerScheme(AppConfig.EditorColorerScheme)
		SaveConfig()
	}

	btnCancel.OnClick = func() { dlg.Close() }

	btnOk.OnClick = func() {
		apply()
		dlg.Close()
	}

	btnReload.OnClick = func() {
		apply()
		ResetColorerSessions()
		ResetColorerRegions()
		vtui.FrameManager.Redraw()
	}

	btnDownload.OnClick = func() {
		apply()
		dlg.Close()
		DownloadColorerSchemas(pf, func(success bool) {
			if !success {
				return
			}
			ResetColorerSessions()
			ResetColorerRegions()
			ResetColorerScheme()
			SetColorerScheme(AppConfig.EditorColorerScheme)
		})
	}

	vtui.FrameManager.Push(dlg)
}
