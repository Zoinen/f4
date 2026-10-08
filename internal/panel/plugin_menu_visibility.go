package panel

import (
	"path/filepath"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
)

// PluginMenuVisibilityFilePath is the file that lists the F11 menu entries the
// user has hidden, one action name per line (f4#918). It sits beside the
// drive-tool visibility file and has the same format.
func PluginMenuVisibilityFilePath() string {
	if config.IsPortableProfile() {
		return filepath.Join(config.GetF4ConfigDir(), "settings", "plugin-menu-visibility.txt")
	}
	configDir, _ := config.UserConfigDir()
	return filepath.Join(configDir, "f4", "settings", "plugin-menu-visibility.txt")
}

// hiddenPluginMenuEntries reads the hidden action names. A name whose plugin
// is not loaded stays in the file, so the entry is still hidden when the
// plugin comes back.
func hiddenPluginMenuEntries() map[string]bool {
	names, _ := LoadDisabledDriveTools(PluginMenuVisibilityFilePath())
	hidden := make(map[string]bool, len(names))
	for _, name := range names {
		hidden[name] = true
	}
	return hidden
}

// PluginMenuEntriesSnapshot is every entry the F11 menu could show, hidden or
// not, in menu order. Settings lists these to let the user choose.
func PluginMenuEntriesSnapshot() []PluginMenuEntry {
	var pf *PanelsFrame
	if frame := FindPanelsFrameAnyScreen(); frame != nil {
		pf = frame
	}
	items := plughost.PluginMenuItemsSnapshot()
	var commands []vfs.PluginCommand
	if pf != nil {
		commands = commandsForPluginMenu(plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, pf))
	}
	return buildPluginMenuEntries(items, commands)
}

// SavePluginMenuHidden stores the hidden action names.
func SavePluginMenuHidden(names []string) error {
	return SaveDisabledDriveTools(PluginMenuVisibilityFilePath(), names)
}

// LoadPluginMenuHidden is the stored list of hidden action names, in order.
func LoadPluginMenuHidden() ([]string, error) {
	return LoadDisabledDriveTools(PluginMenuVisibilityFilePath())
}
