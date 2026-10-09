package app

import (
	"testing"

	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
)

func macroMenuLabels(location vfs.PluginCommandLocation) []string {
	var labels []string
	for _, command := range plughost.PluginCommandsSnapshot(location, nil) {
		labels = append(labels, command.Label)
	}
	return labels
}

// MenuItem{} entries of macro scripts are registered as plugin commands, and
// a reload (or no engine at all) takes the old ones back.
func TestSyncMacroMenuItemsRegistersAndReplaces(t *testing.T) {
	t.Cleanup(func() { syncMacroMenuItems(nil) })

	engine, err := macro.NewLuaMacroEngine(f4MacroHost{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	if err := engine.LoadString("menu.lua", `
		MenuItem { description = "Macro hello"; action = function() end }
		MenuItem { description = "Macro settings"; menu = "Config"; action = function() end }
		MenuItem { description = "Macro disk"; menu = "Disks"; action = function() end }
		CommandLine { description = "Macro command"; prefixes = "mtest"; action = function() end }
	`); err != nil {
		t.Fatal(err)
	}

	syncMacroMenuItems(engine)
	syncMacroMenuItems(engine) // a second sync does not add them again
	if got := macroMenuLabels(vfs.PluginCommandPanel); len(got) != 1 || got[0] != "Macro hello" {
		t.Fatalf("plugin menu = %v, want [Macro hello]", got)
	}
	if got := macroMenuLabels(vfs.PluginCommandConfig); len(got) != 1 || got[0] != "Macro settings" {
		t.Fatalf("config menu = %v, want [Macro settings]", got)
	}

	panel.CommandPrefixRegistry.RLock()
	_, prefixRegistered := panel.CommandPrefixRegistry.ByPrefix["mtest"]
	panel.CommandPrefixRegistry.RUnlock()
	if !prefixRegistered {
		t.Fatal("the CommandLine{} prefix was not registered")
	}

	syncMacroMenuItems(nil)
	if got := macroMenuLabels(vfs.PluginCommandPanel); len(got) != 0 {
		t.Fatalf("plugin menu after the engine went = %v", got)
	}
	panel.CommandPrefixRegistry.RLock()
	_, prefixRegistered = panel.CommandPrefixRegistry.ByPrefix["mtest"]
	panel.CommandPrefixRegistry.RUnlock()
	if prefixRegistered {
		t.Fatal("the CommandLine{} prefix outlived the engine")
	}
}
