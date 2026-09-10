package editor

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/gui"
)

// Which external editor the user configured, GUI build included. It reads
// the configuration and nothing else, which is why it can sit here rather
// than with the action that spawns the process.

// configuredExternalEditorCommand selects the editor configured for the
// renderer family that started this f4 session. The old single command
// remains a fallback so existing settings continue to work after the split
// configuration is introduced.
func ConfiguredExternalEditorCommand() string {
	if gui.Running {
		if config.App.ExternalEditorGUI != "" {
			return config.App.ExternalEditorGUI
		}
	} else if config.App.ExternalEditorConsole != "" {
		return config.App.ExternalEditorConsole
	}
	return config.App.ExternalEditorCommand
}
