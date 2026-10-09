package plughost

import "testing"

func TestPluginManagerNamesListsLoadedPluginsInLoadOrder(t *testing.T) {
	pm := NewPluginManager(nil)
	pm.keepPlugin(&pluginManagerTestPlugin{name: "first"})
	pm.keepPlugin(&pluginManagerTestPlugin{name: "second"})

	got := pm.Names()
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("Names() = %q, want [first second]", got)
	}

	var none *PluginManager
	if names := none.Names(); names != nil {
		t.Fatalf("Names() on a nil manager = %q, want nil", names)
	}
}
