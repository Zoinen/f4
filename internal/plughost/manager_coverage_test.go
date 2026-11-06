package plughost

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/plugins/envman"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
)

type hostIdentityPlugin struct {
	want vfs.HostAPI
}

func (p *hostIdentityPlugin) Init(api vfs.HostAPI) error {
	if api != p.want {
		return fmt.Errorf("plugin received %T instead of original %T; optional host interfaces are hidden", api, p.want)
	}
	return nil
}
func (*hostIdentityPlugin) Close() error    { return nil }
func (*hostIdentityPlugin) GetName() string { return "host capability probe" }

func TestPluginManagerPreservesInProcessHostCapabilities(t *testing.T) {
	api := newLuaTestHostAPI()
	manager := NewPluginManager(api)
	if err := manager.initPlugin(&hostIdentityPlugin{want: api}); err != nil {
		t.Fatal(err)
	}
}

type environmentPluginTestHost struct {
	*luaTestHostAPI
	commands []vfs.PluginCommand
	settings int
}

type testPrefixRegistration struct{ *UnregisterFunc }

func (*testPrefixRegistration) SetPrefix(string) error { return nil }

func (h *environmentPluginTestHost) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return NewUnregisterFunc(func() {}), nil
}
func (h *environmentPluginTestHost) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	h.commands = append(h.commands, command)
	return NewUnregisterFunc(func() {}), nil
}
func (h *environmentPluginTestHost) RegisterCommandPrefix(
	id, prefix string, handler func(vfs.App, string),
) (vfs.CommandPrefixRegistration, error) {
	return &testPrefixRegistration{UnregisterFunc: NewUnregisterFunc(func() {})}, nil
}
func (h *environmentPluginTestHost) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return NewUnregisterFunc(func() {}), nil
}
func (h *environmentPluginTestHost) SnapshotProcessEnvironment() vfs.ProcessEnvironmentSnapshot {
	return vfs.ProcessEnvironmentSnapshot{Variables: []vfs.ProcessEnvironmentVariable{}}
}
func (h *environmentPluginTestHost) ApplyProcessEnvironment(
	changes []vfs.ProcessEnvironmentChange,
) (vfs.ProcessEnvironmentSnapshot, error) {
	return h.SnapshotProcessEnvironment(), nil
}
func (h *environmentPluginTestHost) RegisterSettingsProvider(f4settings.Provider) (vfs.Registration, error) {
	h.settings++
	return NewUnregisterFunc(func() {}), nil
}

func TestPluginManagerInitializesEnvironmentManager(t *testing.T) {
	host := &environmentPluginTestHost{luaTestHostAPI: newLuaTestHostAPI(), commands: []vfs.PluginCommand{}}
	manager := NewPluginManager(host)
	plugin := envman.NewPlugin(t.TempDir())
	if err := manager.initPlugin(plugin); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Error(err)
		}
	})
	if len(host.commands) != 2 || host.commands[0].ID != "f4.envman.open" || host.settings != 1 {
		t.Fatalf("Environment Manager contributions missing: commands=%v settings=%d", host.commands, host.settings)
	}
}

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
