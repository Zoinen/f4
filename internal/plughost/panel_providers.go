package plughost

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

type registeredPanelProvider struct {
	provider vfs.PanelProvider
	token    *struct{}
}

var panelProviderRegistry = struct {
	sync.RWMutex
	byID map[string]registeredPanelProvider
}{byID: make(map[string]registeredPanelProvider)}

const panelProviderCommandPrefix = "panel."

// RegisterPanelProvider implements the optional panel-only contribution API.
// A provider is intentionally also exposed as a regular command so it is
// available from the plugin menu and command palette without a second API.
func RegisterPanelProvider(provider vfs.PanelProvider) (vfs.Registration, error) {
	provider.ID = strings.TrimSpace(provider.ID)
	provider.Title = strings.TrimSpace(provider.Title)
	if provider.ID == "" {
		return nil, errors.New("panel provider ID is empty")
	}
	if provider.Title == "" {
		return nil, fmt.Errorf("panel provider %q has an empty title", provider.ID)
	}
	if provider.Open == nil {
		return nil, fmt.Errorf("panel provider %q has no open handler", provider.ID)
	}

	registryID := strings.ToLower(provider.ID)
	commandID := panelProviderCommandPrefix + registryID
	token := &struct{}{}
	panelProviderRegistry.Lock()
	if _, exists := panelProviderRegistry.byID[registryID]; exists {
		panelProviderRegistry.Unlock()
		return nil, fmt.Errorf("panel provider %q is already registered", provider.ID)
	}
	panelProviderRegistry.byID[registryID] = registeredPanelProvider{provider: provider, token: token}
	panelProviderRegistry.Unlock()

	// The row is the plugin's own title, as in Far's plugin menu
	// ("ProcList", not "Open ProcList", f4#312); "open" stays a search term
	// so the palette still finds it that way.
	command, err := RegisterPluginCommand(vfs.PluginCommand{
		ID:          commandID,
		Location:    vfs.PluginCommandPanel,
		Label:       provider.Title,
		Description: provider.Description,
		SearchTerms: []string{"open", "panel", "plugin", provider.ID},
		Run: func(app vfs.App) {
			if App != nil {
				App.OpenPanelProvider(app, registryID)
			}
		},
	})
	if err != nil {
		panelProviderRegistry.Lock()
		if current, ok := panelProviderRegistry.byID[registryID]; ok && current.token == token {
			delete(panelProviderRegistry.byID, registryID)
		}
		panelProviderRegistry.Unlock()
		return nil, fmt.Errorf("register panel provider %q command: %w", provider.ID, err)
	}

	return &UnregisterFunc{fn: func() {
		command.Unregister()
		panelProviderRegistry.Lock()
		if current, ok := panelProviderRegistry.byID[registryID]; ok && current.token == token {
			delete(panelProviderRegistry.byID, registryID)
		}
		panelProviderRegistry.Unlock()
	}}, nil
}

func LookupPanelProvider(id string) (vfs.PanelProvider, bool) {
	panelProviderRegistry.RLock()
	entry, ok := panelProviderRegistry.byID[strings.ToLower(strings.TrimSpace(id))]
	panelProviderRegistry.RUnlock()
	return entry.provider, ok
}
