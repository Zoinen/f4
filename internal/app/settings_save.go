package app

import (
	"github.com/unxed/f4/internal/config"
)

// saveSettingsGroups writes the groups the "Save settings" dialog ticked. It
// stays in the root because the three groups belong to three layers: the window
// geometry comes from the GUI backend, the settings file from internal/config,
// and the panel state from the session writer.
func saveSettingsGroups(general, pnl, window bool) {
	if !general && !pnl && !window {
		return
	}
	if window {
		CaptureCurrentWindowSize()
		CaptureCurrentWindowPosition()
	}
	if general {
		config.SaveWithWindowSize(window)
	} else if window {
		config.SaveGuiWindowSize()
	}
	if pnl {
		SaveSessionFile(GetSessionIniPath())
	}
}
