package gitplugin

import (
	"context"
	"errors"
	"sync"

	"github.com/unxed/f4/vfs"
)

// Plugin is the built-in Git integration shell. Repository discovery, prompt
// state and ordinary-panel decorations are cache-only; status scans and Git
// mutations run through the fork's cancellable APIs off the UI goroutine.
type Plugin struct {
	mu          sync.RWMutex
	api         vfs.HostAPI
	discovery   *RepositoryDiscovery
	initialized bool

	registrations []vfs.Registration
	statuses      map[string]*repositoryStatus
	statusTasks   map[string]context.CancelFunc
	statusViews   map[*StatusVFS]struct{}
}

func NewPlugin() *Plugin {
	return &Plugin{discovery: NewRepositoryDiscovery()}
}

func (plugin *Plugin) GetName() string { return "Git" }

func (plugin *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("Git: nil host API")
	}
	plugin.mu.Lock()
	if plugin.initialized {
		plugin.mu.Unlock()
		return errors.New("Git: plugin is already initialized")
	}
	if plugin.discovery == nil {
		plugin.discovery = NewRepositoryDiscovery()
	}
	plugin.api = api
	plugin.statuses = make(map[string]*repositoryStatus)
	plugin.statusTasks = make(map[string]context.CancelFunc)
	plugin.statusViews = make(map[*StatusVFS]struct{})
	plugin.initialized = true
	plugin.mu.Unlock()

	if err := plugin.registerIntegration(api); err != nil {
		_ = plugin.Close()
		return err
	}
	return nil
}

// Discovery returns the session-only asynchronous repository cache. Callers
// must retain observer IDs per panel so one panel's navigation never cancels
// discovery for another panel.
func (plugin *Plugin) Discovery() *RepositoryDiscovery {
	plugin.mu.RLock()
	discovery := plugin.discovery
	plugin.mu.RUnlock()
	return discovery
}

func (plugin *Plugin) Close() error {
	plugin.mu.Lock()
	discovery := plugin.discovery
	registrations := append([]vfs.Registration(nil), plugin.registrations...)
	cancelers := make([]context.CancelFunc, 0, len(plugin.statusTasks))
	for _, cancel := range plugin.statusTasks {
		cancelers = append(cancelers, cancel)
	}
	views := make([]*StatusVFS, 0, len(plugin.statusViews))
	for view := range plugin.statusViews {
		views = append(views, view)
	}
	plugin.discovery = nil
	plugin.api = nil
	plugin.initialized = false
	plugin.registrations = nil
	plugin.statuses = nil
	plugin.statusTasks = nil
	plugin.statusViews = nil
	plugin.mu.Unlock()
	for _, cancel := range cancelers {
		cancel()
	}
	for _, view := range views {
		_ = view.Close()
	}
	for index := len(registrations) - 1; index >= 0; index-- {
		registrations[index].Unregister()
	}
	if discovery != nil {
		discovery.Close()
	}
	return nil
}
