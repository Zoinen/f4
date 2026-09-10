package app

import (
	"github.com/unxed/f4/internal/media"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// termApplication is the application side of terminal.Application. Most of it is
// what a Unix client attach needs: a daemon rebuilds the interface for the
// terminal that just connected, and the interface is not the terminal's to
// build.
type termApplication struct{}

func (termApplication) InitCore() *vtui.ScreenBuf { return InitCore() }

func (termApplication) OpenEditFile() { OpenDashEFileIfRequested() }

func (termApplication) ClientAttached(startLeft, startRight, editPath string) {
	top := vtui.FrameManager.GetTopFrame()
	pf, ok := top.(*panel.PanelsFrame)
	if !ok || pf == nil {
		if editPath != "" {
			vtui.DebugLog("SERVER: -e %q: top frame is not a *PanelsFrame (%T)", editPath, top)
		}
		return
	}
	// A workspace that had its panels hidden gets its host console back.
	if pf.ShellMode == terminal.ShellModeHost && !pf.ShowPanels {
		pf.EnterHostConsole()
	}
	// A client that attached to a running daemon moves its workspace to its
	// own directory, as a normal start would.
	if startLeft != "" {
		panel.ApplyStartupDirs(pf, startLeft, startRight)
	}
	if editPath != "" {
		OpenEditFileIn(pf, editPath)
	}
}

func (termApplication) ClientDetached() {
	for _, s := range vtui.FrameManager.Screens {
		if s == nil {
			continue
		}
		for _, f := range s.Frames {
			if pf, ok := f.(*panel.PanelsFrame); ok && pf != nil {
				if pf.ShellMode == terminal.ShellModeHost && pf.IsHostConsoleActive() {
					pf.LeaveHostConsole()
				}
			}
		}
	}
}

func (termApplication) DecodeImage(data []byte) (*vtui.ImageSurface, error) {
	return media.DecodeImageWithStdlib(data)
}

func (termApplication) VersionInfo() string { return GetFormattedVersionInfo() }

func (termApplication) EditFilePath() string { return editFilePath }

func (termApplication) StartupDirs() (string, string) { return startupDirs() }

var _ terminal.Application = termApplication{}

func init() { terminal.App = termApplication{} }
