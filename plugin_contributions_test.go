package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestPluginCommandRegistrationVisibilityAndCleanup(t *testing.T) {
	api := &coreAPI{}
	visible := false
	registration, err := api.RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.plugin-command",
		Location: vfs.PluginCommandPanel,
		Label:    "Test command",
		Visible:  func(vfs.App) bool { return visible },
		Run:      func(vfs.App) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	if commands := pluginCommandsSnapshot(vfs.PluginCommandPanel, nil); len(commands) != 0 {
		t.Fatalf("hidden command was returned: %#v", commands)
	}
	visible = true
	commands := pluginCommandsSnapshot(vfs.PluginCommandPanel, nil)
	if len(commands) != 1 || commands[0].ID != "test.plugin-command" {
		t.Fatalf("visible command snapshot = %#v", commands)
	}
	if config := pluginCommandsSnapshot(vfs.PluginCommandConfig, nil); len(config) != 0 {
		t.Fatalf("command leaked into config menu: %#v", config)
	}

	registration.Unregister()
	registration.Unregister()
	if commands := pluginCommandsSnapshot(vfs.PluginCommandPanel, nil); len(commands) != 0 {
		t.Fatalf("unregistered command remains: %#v", commands)
	}
}

func TestPluginCommandRegistrationRejectsDuplicateID(t *testing.T) {
	api := &coreAPI{}
	command := vfs.PluginCommand{ID: "test.duplicate-command", Label: "One", Run: func(vfs.App) {}}
	registration, err := api.RegisterPluginCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)
	if _, err := api.RegisterPluginCommand(command); err == nil {
		t.Fatal("duplicate command ID was accepted")
	}
}

func TestPluginConfigurationActionIsRegistered(t *testing.T) {
	action, ok := GetAction("Settings.PluginConfiguration")
	if !ok || action.Handler == nil || action.MenuPath != "Options" {
		t.Fatalf("plugin configuration action = %#v, registered=%t", action, ok)
	}
}
