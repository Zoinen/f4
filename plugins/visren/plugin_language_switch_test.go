package visren

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

// Issue #618: the plugin configuration menu has to follow a language change
// made while f4 is running. The host resolves PluginCommand.LabelKey and
// DescriptionKey on every render, so Init must hand it the catalog keys; text
// resolved inside Init keeps the language active at startup for the rest of
// the session.
func TestConfigurationCommandCarriesLocalizationKeys(t *testing.T) {
	host := &contributionHostMock{hostAPIMock: &hostAPIMock{}}
	if err := (&Plugin{}).Init(host); err != nil {
		t.Fatal(err)
	}

	var configure *vfs.PluginCommand
	for i := range host.commands {
		if host.commands[i].ID == "visren.configure" {
			configure = &host.commands[i]
		}
	}
	if configure == nil {
		t.Fatalf("configuration command was not registered: %#v", host.commands)
	}
	if configure.LabelKey != "VisRen.ConfigMenu" {
		t.Fatalf("LabelKey = %q, want VisRen.ConfigMenu", configure.LabelKey)
	}
	if configure.DescriptionKey != "VisRen.ConfigDescription" {
		t.Fatalf("DescriptionKey = %q, want VisRen.ConfigDescription", configure.DescriptionKey)
	}
	if configure.Label != "Visual File Renamer" {
		t.Fatalf("Label = %q, want the English fallback literal", configure.Label)
	}
	if configure.Description != "Configure the Visual File Renamer editor" {
		t.Fatalf("Description = %q, want the English fallback literal", configure.Description)
	}
}
