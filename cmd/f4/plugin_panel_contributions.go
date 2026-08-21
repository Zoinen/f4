package main

import (
	"errors"
	"sync"

	"github.com/unxed/f4/vfs"
)

var errNilPanelNavigationProvider = errors.New("panel navigation provider is nil")

type panelNavigationToken struct{ _ byte }

var panelNavigationRegistry = struct {
	sync.RWMutex
	providers map[*panelNavigationToken]vfs.PanelNavigationProvider
}{providers: make(map[*panelNavigationToken]vfs.PanelNavigationProvider)}

// RegisterPanelNavigationProvider lets an in-process plugin warm a
// session-only cache when a user enters a folder.  Providers receive no direct
// filesystem access from this API; their callbacks must dispatch workers.
func (c *coreAPI) RegisterPanelNavigationProvider(provider vfs.PanelNavigationProvider) (vfs.Registration, error) {
	if provider == nil {
		return nil, errNilPanelNavigationProvider
	}
	token := &panelNavigationToken{}
	panelNavigationRegistry.Lock()
	panelNavigationRegistry.providers[token] = provider
	panelNavigationRegistry.Unlock()
	return &unregisterFunc{fn: func() {
		panelNavigationRegistry.Lock()
		delete(panelNavigationRegistry.providers, token)
		panelNavigationRegistry.Unlock()
	}}, nil
}

func panelNavigationProvidersSnapshot() []vfs.PanelNavigationProvider {
	panelNavigationRegistry.RLock()
	providers := make([]vfs.PanelNavigationProvider, 0, len(panelNavigationRegistry.providers))
	for _, provider := range panelNavigationRegistry.providers {
		providers = append(providers, provider)
	}
	panelNavigationRegistry.RUnlock()
	return providers
}

func hasPanelNavigationProviders() bool {
	panelNavigationRegistry.RLock()
	hasProviders := len(panelNavigationRegistry.providers) != 0
	panelNavigationRegistry.RUnlock()
	return hasProviders
}

func notifyPanelNavigationProviders(host vfs.PanelHost, snapshot vfs.PanelSnapshot) {
	for _, provider := range panelNavigationProvidersSnapshot() {
		provider.PanelNavigated(host, snapshot)
	}
}

var _ vfs.PanelNavigationHost = (*coreAPI)(nil)
