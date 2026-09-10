package panel

import (
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"strings"
)

func (pf *PanelsFrame) SemanticIncrementalShell(ctx *vtui.SemanticContext) (extui.ShellModel, map[int]*FileSystemPanel, bool) {
	totalStarted := navtrace.SemanticIncrementalStageStart()
	started := navtrace.SemanticIncrementalStageStart()
	title := strings.TrimSpace(pf.GetTitle())
	navtrace.SemanticIncrementalStageDone("shell.title", started)
	started = navtrace.SemanticIncrementalStageStart()
	terminalBusy := pf.IsPtyBusy()
	navtrace.SemanticIncrementalStageDone("shell.pty_busy", started)
	started = navtrace.SemanticIncrementalStageStart()
	panelLayout := pf.semanticPanelLayoutModel(ctx)
	navtrace.SemanticIncrementalStageDone("shell.panel_layout", started)
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          title,
		Mode:           "panels",
		ActivePanel:    pf.ActiveIdx,
		ShowPanels:     pf.ShowPanels,
		ShowLeftPanel:  pf.ShowLeftPanel,
		ShowRightPanel: pf.ShowRightPanel,
		Wide:           pf.Wide,
		WidePanel:      pf.WidePanel,
		PanelLayout:    panelLayout,
		ShowKeyBar:     pf.ShowKeyBar,
		TerminalBusy:   terminalBusy,
		TerminalActive: !pf.ShowPanels,
	}
	if !pf.ShowPanels {
		shell.Mode = "terminal"
	}
	panels := make(map[int]*FileSystemPanel)
	for side, panel := range pf.Panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			started = navtrace.SemanticIncrementalStageStart()
			header, valid := fsp.semanticPanelHeaderModel(ctx, side, side == pf.ActiveIdx)
			navtrace.SemanticIncrementalStageDone("shell.panel_header", started, "side", side)
			if !valid {
				navtrace.SemanticIncrementalStageDone("shell.total", totalStarted, "valid", false)
				return extui.ShellModel{}, nil, false
			}
			shell.Panels = append(shell.Panels, header)
			panels[side] = fsp
		}
		if info, ok := pf.AltPanels[side].(*InfoPanel); ok {
			shell.InfoPanels = append(shell.InfoPanels, info.semanticModel(side, side == pf.ActiveIdx))
		}
		if quick, ok := pf.AltPanels[side].(*QuickViewPanel); ok {
			sourceSide := -1
			for candidate, source := range pf.Panels {
				if source == quick.Source() {
					sourceSide = candidate
					break
				}
			}
			shell.QuickViews = append(shell.QuickViews,
				quick.semanticModel(side, sourceSide, side == pf.ActiveIdx))
		}
	}
	if pf.CmdLine != nil {
		started = navtrace.SemanticIncrementalStageStart()
		shell.CommandLine = pf.CmdLine.SemanticModel(ctx)
		navtrace.SemanticIncrementalStageDone("shell.command_line", started)
	}
	if pf.TermView != nil {
		started = navtrace.SemanticIncrementalStageStart()
		shell.Terminal = pf.TermView.SemanticModelWithBottomOverlay(
			ctx, terminalCommandLineOverlayRows(shell.CommandLine))
		navtrace.SemanticIncrementalStageDone("shell.terminal", started)
	}
	if macro.MacroMgr != nil && macro.MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}
	navtrace.SemanticIncrementalStageDone("shell.total", totalStarted, "valid", true)
	return shell, panels, true
}
