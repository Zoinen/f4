package app

import (
	"github.com/unxed/f4/internal/panel"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// The regions FarColorer reads the cross colors from.

// colorerCrossModeItems lists the "Show cross" choices in the order the
// ColorerCross* constants define, so that the combo box position is the mode.
func colorerCrossModeItems() []string {
	return []string{
		i18n.Msg("ColorerSettings.CrossOff"),
		i18n.Msg("ColorerSettings.CrossVertical"),
		i18n.Msg("ColorerSettings.CrossHorizontal"),
		i18n.Msg("ColorerSettings.CrossBoth"),
	}
}

// colorerIsActive reports whether Colorer is the highlighter in charge.

// crossModeAxes splits a cross mode into its horizontal and vertical parts.

// colorerCrossAttr resolves one of the cross regions of the active color
// style. The lookup is exact, the way the editor background one is: a cross
// color guessed from an unrelated region would paint a stripe across the whole
// editor, so a style without the region keeps the f4 palette.

// EditorCrossAttrs tells the editor which cross lines to draw and in which
// colors. The crosshair checkbox stays the master switch, the mode only picks
// the axes.

func ActionColorerSettings(pf *panel.PanelsFrame) {
	width, height := 74, 19
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("ColorerSettings.Title"))
	dlg.ShowClose = true

	// 1. Initialize Widgets
	chkEnabled := vtui.NewCheckbox(0, 0, i18n.Msg("ColorerSettings.Enabled"), false)
	if editor.ColorerIsActive() {
		chkEnabled.State = 1
	}

	// The catalog carries a machine name and a human description; the machine
	// name is what the config stores, so the two lists are kept in step.
	schemeNames := []string{}
	schemeItems := []string{}
	for _, scheme := range editor.ListColorerSchemes() {
		schemeNames = append(schemeNames, scheme.Name)
		schemeItems = append(schemeItems, editor.ColorerSchemeLabel(scheme))
	}
	if len(schemeItems) == 0 {
		schemeNames = append(schemeNames, "")
		schemeItems = append(schemeItems, "")
	}
	selectedScheme := 0
	for i := 0; i < len(schemeNames); i++ {
		if strings.EqualFold(schemeNames[i], config.App.EditorColorerScheme) {
			selectedScheme = i
			break
		}
	}
	comboScheme := vtui.NewComboBox(0, 0, 44, schemeItems)
	comboScheme.DropdownOnly = true
	comboScheme.Menu.SetSelectPos(selectedScheme)
	comboScheme.Edit.SetText(schemeItems[selectedScheme])
	lblScheme := vtui.NewLabel(0, 0, i18n.Msg("ColorerSettings.Style"), comboScheme)

	crossItems := colorerCrossModeItems()
	crossPos := config.App.EditorCrossMode
	if crossPos < 0 || crossPos >= len(crossItems) {
		crossPos = config.ColorerCrossBoth
	}
	comboCross := vtui.NewComboBox(0, 0, 44, crossItems)
	comboCross.DropdownOnly = true
	comboCross.Menu.SetSelectPos(crossPos)
	comboCross.Edit.SetText(crossItems[crossPos])
	lblCross := vtui.NewLabel(0, 0, i18n.Msg("ColorerSettings.Cross"), comboCross)

	chkSyntax := vtui.NewCheckbox(0, 0, i18n.Msg("ColorerSettings.Syntax"), false)
	if config.App.EditorColorerSyntax {
		chkSyntax.State = 1
	}

	chkBackground := vtui.NewCheckbox(0, 0, i18n.Msg("ColorerSettings.Background"), false)
	if config.App.EditorColorerBackground {
		chkBackground.State = 1
	}

	editCatalog := vtui.NewEdit(0, 0, width-6, config.App.EditorColorerCatalog)
	editCatalog.ClearSelection()
	lblCatalog := vtui.NewLabel(0, 0, i18n.Msg("ColorerSettings.Catalog"), editCatalog)

	btnReload := vtui.NewButton(0, 0, i18n.Msg("ColorerSettings.Reload"))
	btnDownload := vtui.NewButton(0, 0, i18n.Msg("ColorerSettings.Download"))
	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

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
			config.App.EditorHighlighter = "Colorer"
		} else if editor.ColorerIsActive() {
			config.App.EditorHighlighter = "Chroma"
		}
		config.App.EditorColorerScheme = ""
		if pos := comboScheme.Menu.SelectPos; pos > 0 && pos < len(schemeNames) {
			config.App.EditorColorerScheme = schemeNames[pos]
		}
		config.App.EditorCrossMode = comboCross.Menu.SelectPos
		config.App.EditorColorerSyntax = chkSyntax.State == 1
		config.App.EditorColorerBackground = chkBackground.State == 1
		config.App.EditorColorerCatalog = strings.TrimSpace(editCatalog.GetText())
		// The catalog may now point somewhere else, so the styles are dropped
		// instead of being kept under the same name.
		editor.ResetColorerScheme()
		editor.SetColorerScheme(config.App.EditorColorerScheme)
		config.SaveConfig()
	}

	btnCancel.OnClick = func() { dlg.Close() }

	btnOk.OnClick = func() {
		apply()
		dlg.Close()
	}

	btnReload.OnClick = func() {
		apply()
		editor.ResetColorerSessions()
		editor.ResetColorerRegions()
		vtui.FrameManager.Redraw()
	}

	btnDownload.OnClick = func() {
		apply()
		dlg.Close()
		editor.DownloadColorerSchemas(pf, func(success bool) {
			if !success {
				return
			}
			editor.ResetColorerSessions()
			editor.ResetColorerRegions()
			editor.ResetColorerScheme()
			editor.SetColorerScheme(config.App.EditorColorerScheme)
		})
	}

	vtui.FrameManager.Push(dlg)
}
