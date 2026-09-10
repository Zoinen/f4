package plughost

import (
	"testing"
)

func TestNewPluginManagerInitializesOpenManager(t *testing.T) {
	api := newLuaTestHostAPI()
	manager := NewPluginManager(api)

	if manager.api == nil {
		t.Fatal("NewPluginManager did not retain the host API")
	}
	if manager.closed || len(manager.plugins) != 0 || manager.externalLoader != nil {
		t.Fatalf("new manager state = %#v", manager)
	}
}

func TestPluginManagerLoadExternalPluginDropsFailedPlugin(t *testing.T) {
	manager := NewPluginManager(newLuaTestHostAPI())
	manager.LoadExternalPlugin(" \t")

	if len(manager.plugins) != 0 {
		t.Fatalf("failed external plugin was retained: %v", manager.plugins)
	}
}

func TestPluginManagerLoadSinglePlugRingItemIgnoresEmptyEntrypoint(t *testing.T) {
	manager := NewPluginManager(newLuaTestHostAPI())
	manager.LoadSinglePlugRingItem(PlugRingItem{})

	if len(manager.plugins) != 0 {
		t.Fatalf("empty PlugRing item was retained: %v", manager.plugins)
	}
}
