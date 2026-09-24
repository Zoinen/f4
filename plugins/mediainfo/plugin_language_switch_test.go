package mediainfo

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

// Issue #618: the plugin menu has to follow a language change made while f4 is
// running. The host resolves PluginCommand.LabelKey on every render, so Init
// must hand it the catalog key; a label resolved inside Init keeps the language
// that happened to be active at startup and never changes again.
func TestPluginCommandsCarryLocalizationKeys(t *testing.T) {
	host := &pluginTestHost{}
	plugin := NewPlugin(t.TempDir())
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugin.Close() })

	byID := map[string]vfs.PluginCommand{}
	for _, command := range host.commands {
		byID[command.ID] = command
	}

	panelCommand, ok := byID[panelCommandID]
	if !ok {
		t.Fatalf("panel command %q was not registered: %#v", panelCommandID, host.commands)
	}
	if panelCommand.LabelKey != "MediaInfo.Menu" {
		t.Fatalf("panel LabelKey = %q, want MediaInfo.Menu", panelCommand.LabelKey)
	}
	if panelCommand.Label != "&Media information" {
		t.Fatalf("panel Label = %q, want the English fallback literal", panelCommand.Label)
	}
	if panelCommand.LocalizedLabels["ru"] != "&Информация о медиа" {
		t.Fatalf("panel LocalizedLabels = %#v", panelCommand.LocalizedLabels)
	}

	configCommand, ok := byID[configCommandID]
	if !ok {
		t.Fatalf("configuration command %q was not registered: %#v", configCommandID, host.commands)
	}
	if configCommand.LabelKey != "MediaInfo.ConfigMenu" {
		t.Fatalf("configuration LabelKey = %q, want MediaInfo.ConfigMenu", configCommand.LabelKey)
	}
	if configCommand.Label != "MediaInfo" {
		t.Fatalf("configuration Label = %q, want the English fallback literal", configCommand.Label)
	}
}
