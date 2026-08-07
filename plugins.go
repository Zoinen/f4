package main

import (
	"path/filepath"
	"sync"

	androidfs "github.com/unxed/f4/plugins/android"
	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/f4/plugins/dummy_internal"
	iosfs "github.com/unxed/f4/plugins/ios"
	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/plugins/visren"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Plugin represents a loaded module.
type Plugin interface {
	Init(api vfs.HostAPI) error
	Close() error
	GetName() string
}
type PluginMenuItem struct {
	Label   string
	Handler func(app vfs.App)
}

var PluginMenuItems []PluginMenuItem

func RegisterPluginMenuItem(label string, handler func(app vfs.App)) {
	PluginMenuItems = append(PluginMenuItems, PluginMenuItem{Label: label, Handler: handler})
}

type PluginManager struct {
	mu      sync.Mutex
	api     vfs.HostAPI
	plugins []Plugin
}

var GlobalPluginManager *PluginManager

func NewPluginManager() *PluginManager {
	return &PluginManager{
		api: &coreAPI{},
	}
}

func (pm *PluginManager) LoadAll() {
	vtui.DebugLog("--- Loading Plugins ---")

	// 1. Load Internal Plugins
	pm.loadInternal()

	// 2. Load External Plugins from Config
	for _, path := range AppConfig.RegisteredPlugins {
		pm.LoadExternalPlugin(path)
	}

	// 3. Load PlugRing plugins
	pm.loadPlugRing()
}

func (pm *PluginManager) LoadExternalPlugin(path string) {
	p := newPluginForEntrypoint("", path)
	if err := p.Init(pm.api); err == nil {
		pm.mu.Lock()
		pm.plugins = append(pm.plugins, p)
		pm.mu.Unlock()
		vtui.DebugLog("Loaded plugin: %s", p.GetName())
	} else {
		vtui.DebugLog("Failed plugin %s: %v", path, err)
	}
}
func (pm *PluginManager) loadPlugRing() {
	installed := GetInstalledPlugRingItems()
	plugringDir := filepath.Join(GetF4ConfigDir(), "plugring")
	for id, item := range installed {
		if item.Entrypoint != "" {
			// We build a pseudo-path that NewRPCPlugin will handle specifically later if needed,
			// but since NewRPCPlugin uses exec.Command directly, we pass the command.
			// The RPCPlugin execution logic will need to handle splitting by spaces if it's a shell command.
			p := newPluginForPlugRingItem(filepath.Join(plugringDir, id), item)
			if err := p.Init(pm.api); err == nil {
				pm.mu.Lock()
				pm.plugins = append(pm.plugins, p)
				pm.mu.Unlock()
				vtui.DebugLog("Loaded PlugRing RPC plugin: %s", p.GetName())
			} else {
				vtui.DebugLog("Failed PlugRing RPC plugin %s: %v", id, err)
			}
		}
	}
}
func (pm *PluginManager) loadSinglePlugRingItem(item PlugRingItem) {
	if item.Entrypoint == "" {
		return
	}
	plugringDir := filepath.Join(GetF4ConfigDir(), "plugring")
	pluginDir := filepath.Join(plugringDir, item.ID)

	p := newPluginForPlugRingItem(pluginDir, item)
	if err := p.Init(pm.api); err == nil {
		pm.mu.Lock()
		pm.plugins = append(pm.plugins, p)
		pm.mu.Unlock()
		vtui.DebugLog("Hot-loaded PlugRing RPC plugin: %s", p.GetName())
	} else {
		vtui.DebugLog("Failed to hot-load PlugRing RPC plugin %s: %v", item.ID, err)
	}
}

func (pm *PluginManager) loadInternal() {
	plugins := []Plugin{
		&chroma.Plugin{},
		&dummy_internal.InternalDummyPlugin{},
		&archive.ArchivePlugin{},
		androidfs.NewPlugin(),
		iosfs.NewPlugin(),
		&netfox.NetFoxPlugin{},
		&visren.Plugin{},
	}

	for _, p := range plugins {
		if err := p.Init(pm.api); err == nil {
			pm.mu.Lock()
			pm.plugins = append(pm.plugins, p)
			pm.mu.Unlock()
			vtui.DebugLog("Loaded internal plugin: %s", p.GetName())
		} else {
			vtui.DebugLog("Failed to init internal plugin %T: %v", p, err)
		}
	}
}

func (pm *PluginManager) CloseAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, p := range pm.plugins {
		p.Close()
	}
	pm.plugins = nil
}
