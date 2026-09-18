package panel

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func (pf *PanelsFrame) commandLineAutoHide() bool {
	return pf.ShowPanels && pf.SearchFirstMode() && config.App.SearchCommandHideUnfocused
}

func (pf *PanelsFrame) commandLineHiddenByFocus() bool {
	return pf.commandLineAutoHide() && !pf.CommandLineFocused
}

func (pf *PanelsFrame) commandLineRows(width, maxRows int) int {
	if pf.commandLineHiddenByFocus() {
		return 0
	}
	return pf.CmdLine.RequiredRows(width, maxRows)
}

func (pf *PanelsFrame) commandLineSemanticModel(ctx *vtui.SemanticContext) *extui.CommandLineModel {
	model := pf.CmdLine.SemanticModel(ctx)
	model.OwnsNavigation = pf.SearchFirstMode() && pf.CommandLineFocused
	model.AutoHide = pf.commandLineAutoHide()
	if pf.commandLineHiddenByFocus() {
		model.Visible = false
	}
	return model
}
