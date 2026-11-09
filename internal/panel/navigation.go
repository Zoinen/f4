package panel

import (
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// ToggleNavigationMode switches the shared typing policy between Classic and
// Search first. Invoking it from Vim enters Search first as well.
func (pf *PanelsFrame) ToggleNavigationMode() {
	if pf.Closed || !pf.ShowPanels {
		return
	}
	before := config.App
	previous := before.NavigationMode
	if previous == config.NavigationSearchFirst {
		config.App.NavigationMode = config.NavigationClassic
	} else {
		config.App.NavigationMode = config.NavigationSearchFirst
	}
	cmdline.CloseActiveAutocompleteMenus()
	pf.applyNavigationToggle()
	// NavigationMode is global, so suspended workspaces must not retain the
	// old focus or a Fast Find query when the user switches back to them.
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			if other, ok := frame.(*PanelsFrame); ok && other != pf && !other.Closed {
				other.applyNavigationToggle()
			}
		}
	}
	if config.App.AutoSaveSettings && config.App.AutoSaveDialogSettings {
		if err := config.WriteSettingsCandidate(before, config.App); err != nil {
			vtui.DebugLog("CONFIG: Failed to save panel navigation mode: %v", err)
		}
	}
	vtui.DebugLog("[FIX:navigation-toggle] mode=%v -> %v", previous, config.App.NavigationMode)
	vtui.FrameManager.HardRefresh()
}

func (pf *PanelsFrame) applyNavigationToggle() {
	pf.CancelFastFind()
	pf.ApplyNavigationMode()
	for i, alt := range pf.AltPanels {
		if alt != nil {
			alt.SetFocus(pf.ShowPanels && i == pf.ActiveIdx)
		}
	}
	if !pf.ShowPanels {
		pf.CmdLine.SetFocus(true)
	}
	pf.ResizeConsole(pf.LastW, pf.LastH)
}
