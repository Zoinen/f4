package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// keyRemapSuspended answers keymap.Suspended: with the panels hidden and an
// AltScreen program or a busy child running, every key is forwarded to that
// program verbatim, and substituting there would send vim or htop a chord the
// user never pressed. It is the same handover the noaltscreenapp and
// noterminalapp hotkey conditions respect.
func KeyRemapSuspended() bool {
	if vtui.FrameManager == nil {
		return false
	}
	pf, ok := vtui.FrameManager.GetTopFrame().(*panel.PanelsFrame)
	if !ok || pf.ShowPanels {
		return false
	}
	if pf.ShellMode == terminal.ShellModeSimpleInline {
		// No terminal.PTY in this mode, so no foreign program can be holding the
		// keyboard; the console view on screen is f4's own overlay.
		return false
	}
	return (pf.TermView != nil && pf.TermView.UseAltScreen) || pf.IsPtyBusy()
}
