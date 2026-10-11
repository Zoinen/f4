package svcmgr

import (
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// The start types the properties dialog offers, and the error controls, in the
// order of their drop-down lists.
var (
	propStartChoices = []startChoice{{startAuto, false}, {startAuto, true}, {startManual, false}, {startDisabled, false}}
	propErrorChoices = []uint32{errorIgnore, errorNormal, errorSevere, errorCritical}
)

// startChoiceIndex finds a start type in propStartChoices; a type the dialog
// does not offer (boot, system) maps to the first entry only for display.
func startChoiceIndex(t uint32, delayed bool) int {
	for i, c := range propStartChoices {
		if c.Type == t && c.Delayed == (delayed && t == startAuto) {
			return i
		}
	}
	return 0
}

func errorChoiceIndex(e uint32) int {
	for i, c := range propErrorChoices {
		if c == e {
			return i
		}
	}
	return 0
}

// configFromFields turns the dialog's field values into a serviceConfig; the
// combo boxes hold texts, so they are mapped back through the choice lists.
func configFromFields(display, path, startText, errText string) serviceConfig {
	cfg := serviceConfig{DisplayName: strings.TrimSpace(display), BinaryPath: strings.TrimSpace(path)}
	for _, c := range propStartChoices {
		if startTypeName(c.Type, c.Delayed) == startText {
			cfg.StartType, cfg.Delayed = c.Type, c.Delayed
		}
	}
	for _, e := range propErrorChoices {
		if errorControlName(e) == errText {
			cfg.ErrorControl = e
		}
	}
	return cfg
}

// showPropertiesDialog is FAR's "Service properties" window: the display name,
// the program, how the service starts and what happens when it fails to start
// at boot. onOk gets the edited values.
func showPropertiesDialog(svc service, d serviceDetails, onOk func(serviceConfig)) {
	if vtui.FrameManager == nil {
		return
	}
	width, height := 64, 15
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("SvcMgr.PropsTitle"))
	dlg.ShowClose = true
	x, y := dlg.X1+2, dlg.Y1+2
	inner := width - 4

	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("SvcMgr.PropsName")+" "+svc.Name, 0))
	y++
	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("SvcMgr.PropsDisplay"), 0))
	y++
	displayEdit := vtui.NewEdit(x, y, inner, svc.Display)
	dlg.AddItem(displayEdit)
	y++
	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("SvcMgr.PropsPath"), 0))
	y++
	pathEdit := vtui.NewEdit(x, y, inner, d.BinaryPath)
	dlg.AddItem(pathEdit)
	y++

	startNames := make([]string, len(propStartChoices))
	for i, c := range propStartChoices {
		startNames[i] = startTypeName(c.Type, c.Delayed)
	}
	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("SvcMgr.PropsStart"), 0))
	y++
	startCombo := vtui.NewComboBox(x, y, 30, startNames)
	startCombo.DropdownOnly = true
	idx := startChoiceIndex(d.StartType, d.Delayed)
	startCombo.Menu.SetSelectPos(idx)
	startCombo.Edit.SetText(startNames[idx])
	dlg.AddItem(startCombo)
	y++

	errNames := make([]string, len(propErrorChoices))
	for i, e := range propErrorChoices {
		errNames[i] = errorControlName(e)
	}
	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("SvcMgr.PropsError"), 0))
	y++
	errCombo := vtui.NewComboBox(x, y, 30, errNames)
	errCombo.DropdownOnly = true
	eidx := errorChoiceIndex(d.ErrorControl)
	errCombo.Menu.SetSelectPos(eidx)
	errCombo.Edit.SetText(errNames[eidx])
	dlg.AddItem(errCombo)

	okButton := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	cancelButton := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	okButton.IsDefault = true
	dlg.AddItem(okButton)
	dlg.AddItem(cancelButton)
	buttons := vtui.NewHBoxLayout(dlg.X1+2, dlg.Y2-2, inner, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(okButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancelButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Apply()

	okButton.OnClick = func() {
		if onOk != nil {
			onOk(configFromFields(displayEdit.GetText(), pathEdit.GetText(), startCombo.Edit.GetText(), errCombo.Edit.GetText()))
		}
		dlg.SetExitCode(1)
	}
	cancelButton.OnClick = func() { dlg.SetExitCode(-1) }
	vtui.FrameManager.Push(dlg)
}
