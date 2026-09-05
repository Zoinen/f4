package main

import (
	"github.com/unxed/vtui"
)

// startupBackendLabels renders a choice list for display: the automatic entry
// is localized, the backend names are not. They are identifiers the user also
// types on the command line and reads in the documentation, so translating
// them would make the two disagree.
func startupBackendLabels(choices []string) []string {
	labels := make([]string, len(choices))
	for i, choice := range choices {
		if choice == "" {
			labels[i] = Msg("StartupSettings.BackendAuto")
			continue
		}
		labels[i] = choice
	}
	return labels
}

// actionStartupSettings edits what f4 does when it is started with no
// --gui/--tty flag: which renderer family it opens, and which backend each
// family uses. The dialog only writes the configuration; the command line
// still overrides all three settings on any individual run.
func actionStartupSettings(pf *PanelsFrame) {
	width, height := 62, 14
	dlg := vtui.NewCenteredDialog(width, height, Msg("StartupSettings.Title"))
	dlg.ShowClose = true

	modeLabels := []string{
		Msg("StartupSettings.ModeAuto"),
		Msg("StartupSettings.ModeTTY"),
		Msg("StartupSettings.ModeGui"),
	}
	comboMode := vtui.NewComboBox(0, 0, 28, modeLabels)
	comboMode.DropdownOnly = true
	modeIndex := startupModeChoiceIndex(AppConfig.StartupMode)
	comboMode.Menu.SetSelectPos(modeIndex)
	comboMode.Edit.SetText(modeLabels[modeIndex])
	lblMode := vtui.NewLabel(0, 0, Msg("StartupSettings.Mode"), comboMode)

	guiChoices := startupBackendChoices(startupGuiBackends)
	guiLabels := startupBackendLabels(guiChoices)
	comboGui := vtui.NewComboBox(0, 0, 28, guiLabels)
	comboGui.DropdownOnly = true
	guiIndex := startupBackendChoiceIndex(guiChoices, AppConfig.GuiBackend)
	comboGui.Menu.SetSelectPos(guiIndex)
	comboGui.Edit.SetText(guiLabels[guiIndex])
	lblGui := vtui.NewLabel(0, 0, Msg("StartupSettings.GuiBackend"), comboGui)

	ttyChoices := startupBackendChoices(startupTTYBackends)
	ttyLabels := startupBackendLabels(ttyChoices)
	comboTTY := vtui.NewComboBox(0, 0, 28, ttyLabels)
	comboTTY.DropdownOnly = true
	ttyIndex := startupBackendChoiceIndex(ttyChoices, AppConfig.TTYBackend)
	comboTTY.Menu.SetSelectPos(ttyIndex)
	comboTTY.Edit.SetText(ttyLabels[ttyIndex])
	lblTTY := vtui.NewLabel(0, 0, Msg("StartupSettings.TTYBackend"), comboTTY)

	note := vtui.NewText(0, 0, Msg("StartupSettings.Note"), 0)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(lblMode)
	dlg.AddItem(comboMode)
	dlg.AddItem(lblGui)
	dlg.AddItem(comboGui)
	dlg.AddItem(lblTTY)
	dlg.AddItem(comboTTY)
	dlg.AddItem(note)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)

	rowMode := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowMode.Add(lblMode, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowMode.Add(comboMode, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowMode, vtui.Margins{}, vtui.AlignFill)

	rowGui := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowGui.Add(lblGui, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowGui.Add(comboGui, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowGui, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowTTY := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowTTY.Add(lblTTY, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTTY.Add(comboTTY, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowTTY, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Add(note, vtui.Margins{Top: 1}, vtui.AlignLeft)

	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		AppConfig.StartupMode = startupChoiceAt(startupModeChoices, comboMode.Menu.SelectPos)
		AppConfig.GuiBackend = startupChoiceAt(guiChoices, comboGui.Menu.SelectPos)
		AppConfig.TTYBackend = startupChoiceAt(ttyChoices, comboTTY.Menu.SelectPos)
		SaveConfig()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}
