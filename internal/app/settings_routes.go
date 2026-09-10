package app

import (
	"strings"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/settings"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
)

// The old action names remain bindable and become category deep links.
var settingsDeepLinks = map[string]string{
	"app.savesettings":  "workspaces",
	"settings.language": "appearance", "settings.helplanguage": "appearance", "settings.panel": "panels", "settings.editor": "editor", "settings.viewer": "editor", "settings.colorer": "syntax", "settings.appearance": "appearance", "settings.startup": "startup", "settings.portable": "startup", "settings.confirmations": "operations", "settings.mousewheel": "keyboard", "settings.pathhints": "terminal", "settings.hotkeys": "keyboard", "settings.autoupdate": "updates", "settings.proxy": "network", "settings.pluginconfiguration": "plugins", "settings.plugins": "plugins", "settings.mackeyboard": "keyboard", "editor.settings": "editor", "viewer.settings": "editor", "panel.fileassociations": "associations", "app.plugring": "plugins", "ai.setup": "ai",
}

func registerSettingsRoutes() {
	// Preserve custom bindings to the removed category.
	RegisterAction(action.Action{Name: "Settings.Category.navigation", Area: "Common", Label: "Panel navigation", LabelKey: "SettingsCenter.NavigationMode.Label", Handler: func() bool { return settings.Open("panels") }})
	RegisterAction(action.Action{Name: "Settings.Open", Area: "Common", Label: "Settings", LabelKey: "SettingsCenter.Title", Description: "Open the searchable Settings Center", MenuPath: "Options", Handler: func() bool { return settings.Open("") }})
	for _, cat := range settings.Categories {
		id := cat.ID
		RegisterAction(action.Action{Name: "Settings.Category." + id, Area: "Common", Label: cat.Label.English, Description: "Open Settings Center at " + cat.Label.English, Handler: func() bool { return settings.Open(id) }})
	}
}

func redirectLegacySettingsActions() {
	for name, category := range settingsDeepLinks {
		if a, ok := action.Lookup(name); ok {
			a.HideFromMenu = true
			a.Handler = func() bool { return settings.Open(category) }
			a.Checked = nil
			RegisterAction(a)
		}
	}
}

func init() {
	settings.Configure(settingsHost{})
	registerSettingsRoutes()
	redirectLegacySettingsActions()
	panel.OpenSettingsAt = settings.OpenAt
	panel.SetSettingsRecordDefault = settings.SetRecordDefault
	panel.OpenUserMenuSettings = settings.OpenUserMenu
	plughost.SettingsCommand = func(id string) bool {
		category, ok := map[string]string{"visren.configure": "operations", "f4.envman.configure": "terminal", "f4.mediainfo.configure": "metadata"}[strings.ToLower(id)]
		return ok && settings.Open(category)
	}
}
func (*CoreAPI) RegisterSettingsProvider(p f4settings.Provider) (vfs.Registration, error) {
	return settings.RegisterProvider(p)
}
func (*CoreAPI) OpenSettings(category, collection, record string, create bool) bool {
	return settings.OpenAt(category, collection, record, create)
}

var _ vfs.SettingsContributionHost = (*CoreAPI)(nil)

func RegisterAction(a action.Action) {
	if category, ok := settingsDeepLinks[strings.ToLower(a.Name)]; ok {
		a.HideFromMenu = true
		a.Checked = nil
		a.Handler = func() bool { return settings.Open(category) }
	}
	action.RegisterAction(a)
}
