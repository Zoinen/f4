package plughost

import (
	"errors"
	"fmt"
	"github.com/unxed/f4/vfs"
	"strings"
	"sync"
)

type RegisteredPanelProvider struct {
	provider vfs.PanelProvider
	token    *struct{}
}

var PanelProviderRegistry = struct {
	sync.RWMutex
	byID map[string]RegisteredPanelProvider
}{byID: make(map[string]RegisteredPanelProvider)}

const PanelProviderCommandPrefix = "panel."

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
	commandID := PanelProviderCommandPrefix + registryID
	token := &struct{}{}
	PanelProviderRegistry.Lock()
	if _, exists := PanelProviderRegistry.byID[registryID]; exists {
		PanelProviderRegistry.Unlock()
		return nil, fmt.Errorf("panel provider %q is already registered", provider.ID)
	}
	PanelProviderRegistry.byID[registryID] = RegisteredPanelProvider{provider: provider, token: token}
	PanelProviderRegistry.Unlock()

	command, err := RegisterPluginCommand(vfs.PluginCommand{
		ID:          commandID,
		Location:    vfs.PluginCommandPanel,
		Label:       "Open " + provider.Title,
		Description: provider.Description,
		SearchTerms: []string{"panel", "plugin", provider.ID},
		Run: func(app vfs.App) {
			if App != nil {
				App.OpenPanelProvider(app, registryID)
			}
		},
	})
	if err != nil {
		PanelProviderRegistry.Lock()
		if current, ok := PanelProviderRegistry.byID[registryID]; ok && current.token == token {
			delete(PanelProviderRegistry.byID, registryID)
		}
		PanelProviderRegistry.Unlock()
		return nil, fmt.Errorf("register panel provider %q command: %w", provider.ID, err)
	}

	return &UnregisterFunc{Fn: func() {
		command.Unregister()
		PanelProviderRegistry.Lock()
		if current, ok := PanelProviderRegistry.byID[registryID]; ok && current.token == token {
			delete(PanelProviderRegistry.byID, registryID)
		}
		PanelProviderRegistry.Unlock()
	}}, nil
}

func LookupPanelProvider(id string) (vfs.PanelProvider, bool) {
	PanelProviderRegistry.RLock()
	entry, ok := PanelProviderRegistry.byID[strings.ToLower(strings.TrimSpace(id))]
	PanelProviderRegistry.RUnlock()
	return entry.provider, ok
}
