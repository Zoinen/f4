package main

import (
	"time"

	"github.com/unxed/vtui"
)

// togglePanelsVisibility switches this workspace between its panels and its
// console view -- what Ctrl+O does. It lives here, and not inline in the
// Panel.Toggle handler, so that a freshly forked terminal workspace can reach
// its console through exactly the same, already-tested path a keypress takes,
// including the per-shell-mode work the switch involves (issue #128).
func (pf *PanelsFrame) togglePanelsVisibility() {
	pf.showPanels = !pf.showPanels
	if pf.showPanels && !pf.showLeftPanel && !pf.showRightPanel {
		pf.showLeftPanel = true
		pf.showRightPanel = true
	}
	// ShellModeSimpleInline manages its own geometry refresh below,
	// timed to when the real terminal screen is actually the one f4
	// is about to draw on (see the two branches). Calling the full,
	// layout-and-repaint-triggering ResizeConsole() here, before that
	// screen switch happens, let vtui's own panel/keybar repaint land
	// on whichever screen (primary or alt) happened to still be
	// active at that exact moment: sometimes the host console the
	// user just switched to, leaving a stray copy of the keybar that
	// a later Ctrl+O toggle would reveal stacked on top of the next
	// one. Every other shell mode keeps the previous unconditional
	// call.
	if pf.shellMode == ShellModeSimpleInline {
		pf.lastShowPanels = pf.showPanels
	} else if pf.menuBar != nil && pf.lastW > 0 && pf.lastH > 0 {
		pf.ResizeConsole(pf.lastW, pf.lastH)
		pf.lastShowPanels = pf.showPanels
	}
	switch pf.shellMode {
	case ShellModeHost:
		if pf.showPanels {
			pf.leaveHostConsole()
		} else {
			pf.enterHostConsole()
		}
	case ShellModeSimpleInline:
		if !pf.showPanels {
			vtui.SetAltScreen(false)
			pf.SetBusy(true)
			pf.syncAutoCompleteSuppression()
			if w, h, err := vtui.GetTerminalSize(); err == nil && w > 0 && h > 0 {
				pf.lastW, pf.lastH = w, h
			}
			clearConsoleViewBackground(pf.lastW, pf.lastH)
			if pf.consoleStyle() == ConsoleViewFar {
				pf.drawConsoleOverlay()
			}
		} else {
			pf.clearConsoleOverlay()
			vtui.SetAltScreen(true)
			pf.SetBusy(false)
			pf.syncAutoCompleteSuppression()
			if pf.menuBar != nil && pf.lastW > 0 && pf.lastH > 0 {
				pf.ResizeConsole(pf.lastW, pf.lastH)
			}
			vtui.FrameManager.HardRefresh()
		}
	case ShellModeSimpleCaptured:
		// Captured mode has no separate console view to switch to;
		// output already went to a dialog, so panels stay visible.
		pf.showPanels = true
		showToast(Msg("Terminal.NotAvailableInEnv"), 3*time.Second)
	default:
		vtui.FrameManager.HardRefresh()
	}
}

// forkTerminalWorkspace opens the console in a workspace of its own instead of
// hiding the panels of the current one. It is the Ctrl+N, Ctrl+O sequence
// users were told to type by hand (issue #128) performed as a single step: the
// panels stay untouched where they are, the new workspace shows nothing but
// the terminal, and Ctrl+Tab flips between the two.
func (pf *PanelsFrame) forkTerminalWorkspace() bool {
	if vtui.FrameManager == nil {
		return false
	}
	if pf.shellMode == ShellModeSimpleCaptured {
		// This environment has no console view to switch to: command output
		// goes to a dialog and Ctrl+O says so while staying on the panels.
		// Forking first would leave the user with a second, identical copy of
		// the panels and the very same message.
		showToast(Msg("Terminal.NotAvailableInEnv"), 3*time.Second)
		return true
	}
	clone := pf.forkPanelsClone()
	vtui.FrameManager.AddScreen(clone)
	// AddScreen activates the workspace it creates, so the switch below runs
	// against the frame the user is now looking at, exactly as Ctrl+O would.
	if clone.showPanels {
		clone.togglePanelsVisibility()
	}
	return true
}

// actionWorkspaceNewTerminal serves the Workspace.NewTerminal action. Like
// Workspace.New it works from any workspace, including full-screen editor,
// viewer and queue screens that keep no panels of their own: the terminal is
// forked from the panels those screens were opened from.
func actionWorkspaceNewTerminal() bool {
	panels := findPanelsFrameAnyScreen()
	return panels != nil && panels.forkTerminalWorkspace()
}
