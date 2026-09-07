package mediainfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

const (
	panelCommandID  = "f4.mediainfo.open"
	configCommandID = "f4.mediainfo.configure"
	prefixID        = "f4.mediainfo.prefix"
	quickViewID     = "f4.mediainfo.quickview"
	macroID         = "f4.mediainfo"
	legacyMacroGUID = "919C1FC6-A571-4642-99DF-BDACE840ED18"
)

// Plugin is the built-in, CGO-free MediaInfo-style plugin. Its backend reads
// through vfs.VFS only; it never materializes a remote file to the OS.
type Plugin struct {
	mu          sync.Mutex
	configDir   string
	api         vfs.HostAPI
	store       *settingsStore
	cache       *reportCache
	registrars  []vfs.Registration
	prefix      vfs.CommandPrefixRegistration
	initialized bool
}

func NewPlugin(configDir string) *Plugin {
	return &Plugin{configDir: configDir, cache: newReportCache(reportCacheCapacity)}
}

func (plugin *Plugin) GetName() string { return "MediaInfo" }

func (plugin *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("MediaInfo: nil host API")
	}
	host, ok := api.(vfs.ContributionHost)
	if !ok {
		return errors.New("MediaInfo: this host does not support plugin contributions")
	}

	plugin.mu.Lock()
	if plugin.initialized {
		plugin.mu.Unlock()
		return errors.New("MediaInfo: plugin is already initialized")
	}
	plugin.api = api
	if plugin.cache == nil {
		plugin.cache = newReportCache(reportCacheCapacity)
	}
	plugin.mu.Unlock()

	store, loadErr := newSettingsStore(plugin.settingsDirectory())
	plugin.mu.Lock()
	plugin.store = store
	plugin.mu.Unlock()
	if loadErr != nil {
		api.Log(loadErr.Error() + "; using defaults")
	}
	settings := store.snapshot()

	var registrations []vfs.Registration
	rollback := func(err error) error {
		for i := len(registrations) - 1; i >= 0; i-- {
			registrations[i].Unregister()
		}
		plugin.mu.Lock()
		plugin.api = nil
		plugin.store = nil
		plugin.prefix = nil
		plugin.mu.Unlock()
		return err
	}

	panelRegistration, err := host.RegisterPluginCommand(vfs.PluginCommand{
		ID:       panelCommandID,
		Location: vfs.PluginCommandPanel,
		Label:    plugin.text("MediaInfo.Menu", "&Media information", "&Информация о медиа"),
		Visible: func(vfs.App) bool {
			return plugin.settings().ShowInPluginMenu
		},
		Run: plugin.openCurrent,
	})
	if err != nil {
		return rollback(fmt.Errorf("MediaInfo: register panel command: %w", err))
	}
	registrations = append(registrations, panelRegistration)

	configRegistration, err := host.RegisterPluginCommand(vfs.PluginCommand{
		ID:       configCommandID,
		Location: vfs.PluginCommandConfig,
		Label:    plugin.text("MediaInfo.ConfigMenu", "MediaInfo", "MediaInfo"),
		Run:      plugin.configure,
	})
	if err != nil {
		return rollback(fmt.Errorf("MediaInfo: register configuration command: %w", err))
	}
	registrations = append(registrations, configRegistration)

	quickRegistration, err := host.RegisterQuickViewProvider(&mediaQuickViewProvider{plugin: plugin})
	if err != nil {
		return rollback(fmt.Errorf("MediaInfo: register Quick View provider: %w", err))
	}
	registrations = append(registrations, quickRegistration)

	prefixRegistration, err := host.RegisterCommandPrefix(prefixID, settings.Prefix, plugin.handlePrefix)
	if err != nil {
		return rollback(fmt.Errorf("MediaInfo: register command prefix: %w", err))
	}
	registrations = append(registrations, prefixRegistration)
	plugin.mu.Lock()
	plugin.prefix = prefixRegistration
	plugin.mu.Unlock()

	macroRegistration, err := host.RegisterMacroCallProvider(vfs.MacroCallProvider{
		IDs:  []string{macroID, legacyMacroGUID},
		Call: plugin.callMacro,
	})
	if err != nil {
		return rollback(fmt.Errorf("MediaInfo: register macro provider: %w", err))
	}
	registrations = append(registrations, macroRegistration)

	plugin.mu.Lock()
	plugin.registrars = registrations
	plugin.initialized = true
	plugin.mu.Unlock()
	return nil
}

func (plugin *Plugin) Close() error {
	plugin.mu.Lock()
	registrations := append([]vfs.Registration(nil), plugin.registrars...)
	plugin.registrars = nil
	plugin.prefix = nil
	plugin.api = nil
	plugin.initialized = false
	cache := plugin.cache
	plugin.mu.Unlock()
	for i := len(registrations) - 1; i >= 0; i-- {
		registrations[i].Unregister()
	}
	cache.clear()
	return nil
}

func (plugin *Plugin) settingsDirectory() string {
	if strings.TrimSpace(plugin.configDir) != "" {
		return plugin.configDir
	}
	if strings.TrimSpace(vfs.CustomConfigDir) != "" {
		return vfs.CustomConfigDir
	}
	if directory, err := os.UserConfigDir(); err == nil {
		return filepath.Join(directory, "f4")
	}
	return "."
}

func (plugin *Plugin) settings() Settings {
	plugin.mu.Lock()
	store := plugin.store
	plugin.mu.Unlock()
	if store == nil {
		return DefaultSettings()
	}
	return store.snapshot()
}

func (plugin *Plugin) log(message string, args ...any) {
	plugin.mu.Lock()
	api := plugin.api
	plugin.mu.Unlock()
	if api != nil {
		api.Log(fmt.Sprintf(message, args...))
	}
}

func (plugin *Plugin) analyzePath(ctx context.Context, fs vfs.VFS, path string, item vfs.VFSItem, mode Mode) (Report, error) {
	if fs == nil {
		return Report{}, errors.New("media source has no VFS")
	}
	if item.Name == "" && !item.IsDir {
		stat, err := fs.Stat(ctx, path)
		if err != nil {
			return Report{}, err
		}
		item = stat
	}
	if item.IsDir {
		return Report{}, errors.New("media source is a directory")
	}
	plugin.mu.Lock()
	cache := plugin.cache
	if cache == nil {
		cache = newReportCache(reportCacheCapacity)
		plugin.cache = cache
	}
	plugin.mu.Unlock()
	key := reportCacheKey(fs, path, item, mode)
	if report, ok := cache.get(key); ok {
		return report, nil
	}
	reader, err := fs.Open(ctx, path)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = reader.Close() }() // The media source is read-only.
	size := reader.Size()
	if size < 0 {
		size = item.Size
	}
	report, err := Analyze(ctx, Source{
		Name:    fs.Base(path),
		Size:    size,
		ModTime: item.MTime,
		Reader:  reader,
	}, DefaultOptions(mode))
	if err != nil {
		return Report{}, err
	}
	cache.put(key, report)
	return report, nil
}
