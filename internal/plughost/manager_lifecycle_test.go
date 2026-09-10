package plughost

import (
	"errors"
	"github.com/unxed/f4/vfs"
	"testing"
	"time"
)

type pluginManagerTestPlugin struct {
	name     string
	closed   *[]string
	closeErr error
}

func (p *pluginManagerTestPlugin) Init(vfs.HostAPI) error { return nil }

func (p *pluginManagerTestPlugin) Close() error {
	*p.closed = append(*p.closed, p.name)
	return p.closeErr
}

func (p *pluginManagerTestPlugin) GetName() string { return p.name }

func TestPluginManager_LoadExternalRunsLoaderOnce(t *testing.T) {
	pm := NewPluginManager(newLuaTestHostAPI())
	loads := 0
	pm.externalLoader = func() { loads++ }

	pm.LoadExternal()
	pm.LoadExternal()

	if loads != 1 {
		t.Fatalf("external loader called %d times, want once", loads)
	}
}

func TestPluginManager_StartExternalSkipsClosedManager(t *testing.T) {
	pm := NewPluginManager(newLuaTestHostAPI())
	pm.CloseAll()
	started := make(chan struct{})
	pm.externalLoader = func() { close(started) }

	pm.StartExternal()
	select {
	case <-started:
		t.Fatal("closed manager started external loading")
	default:
	}

	var nilManager *PluginManager
	nilManager.StartExternal()
}

func TestPluginManager_StartExternalStartsLoader(t *testing.T) {
	pm := NewPluginManager(newLuaTestHostAPI())
	started := make(chan struct{})
	pm.externalLoader = func() { close(started) }

	pm.StartExternal()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("external loader did not start")
	}
}

func TestPluginManager_CloseAllClosesInReverseAndRejectsLatePlugins(t *testing.T) {
	var closed []string
	pm := NewPluginManager(newLuaTestHostAPI())
	first := &pluginManagerTestPlugin{name: "first", closed: &closed}
	second := &pluginManagerTestPlugin{name: "second", closed: &closed, closeErr: errors.New("close failed")}
	if !pm.keepPlugin(first) || !pm.keepPlugin(second) {
		t.Fatal("keepPlugin rejected an open manager")
	}

	pm.CloseAll()
	if got, want := closed, []string{"second", "first"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("close order = %v, want %v", got, want)
	}

	late := &pluginManagerTestPlugin{name: "late", closed: &closed}
	if pm.keepPlugin(late) {
		t.Fatal("keepPlugin accepted a closed manager")
	}
	if len(closed) != 3 || closed[2] != "late" {
		t.Fatalf("late plugin close history = %v, want late close", closed)
	}

	pm.CloseAll()
	if len(closed) != 3 {
		t.Fatalf("second CloseAll closed plugins again: %v", closed)
	}
}

func TestPluginMenuItemsSnapshotIsIndependent(t *testing.T) {
	PluginRegistryMu.Lock()
	original := append([]PluginMenuItem(nil), PluginMenuItems...)
	PluginRegistryMu.Unlock()
	t.Cleanup(func() {
		PluginRegistryMu.Lock()
		PluginMenuItems = original
		PluginRegistryMu.Unlock()
	})

	RegisterPluginMenuItem("snapshot-test", func(vfs.App) {})
	snapshot := PluginMenuItemsSnapshot()
	if len(snapshot) == 0 {
		t.Fatal("plugin menu snapshot is empty")
	}
	last := len(snapshot) - 1
	snapshot[last].Label = "changed outside registry"
	current := PluginMenuItemsSnapshot()
	if current[last].Label != "snapshot-test" {
		t.Fatalf("registry item changed through snapshot: %q", current[last].Label)
	}
}
