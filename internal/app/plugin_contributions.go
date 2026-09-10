package app

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var _ vfs.ContributionHost = (*CoreAPI)(nil)

// The two registries these write to live in internal/plughost, because
// internal/panel reads them too and may not import the composition root.

func (c *CoreAPI) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	return plughost.RegisterPluginCommand(command)
}

func (c *CoreAPI) RegisterPanelProvider(provider vfs.PanelProvider) (vfs.Registration, error) {
	return plughost.RegisterPanelProvider(provider)
}

func actionPluginConfiguration(pf *panel.PanelsFrame) {
	commands := plughost.PluginCommandsSnapshot(vfs.PluginCommandConfig, pf)
	if len(commands) == 0 {
		vtui.ShowMessage(i18n.Msg("Plugins.ConfigTitle"), i18n.Msg("Plugins.ConfigEmpty"), []string{i18n.Msg("vtui.Ok")})
		return
	}

	labels := make([]string, len(commands))
	for i := range commands {
		labels[i] = plughost.PluginCommandDisplayLabel(commands[i])
	}
	pf.Menu(i18n.Msg("Plugins.ConfigTitle"), labels, func(index int) {
		if index >= 0 && index < len(commands) {
			plughost.ExecutePluginCommand(vfs.PluginCommandConfig, commands[index].ID, pf)
		}
	})
}
