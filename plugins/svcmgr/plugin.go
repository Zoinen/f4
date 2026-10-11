package svcmgr

import (
	"errors"
	"fmt"
	"sync"

	"github.com/unxed/f4/vfs"
)

// panelProviderID is also the ID plughost derives the panel's command from
// ("panel.f4.svcmgr").
const panelProviderID = "f4.svcmgr"

// Plugin exposes the service list as an in-process f4 panel plugin.
type Plugin struct {
	mu           sync.Mutex
	registration vfs.Registration
	initialized  bool
}

// NewPlugin constructs the built-in Service Manager plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "SvcMgr" }

// Init registers the panel provider. Off Windows it does nothing and returns
// nil: an absent Service Control Manager is not a load failure.
func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("SvcMgr: nil host API")
	}
	if !Supported() {
		return nil
	}
	host, ok := api.(vfs.PanelContributionHost)
	if !ok {
		return errors.New("SvcMgr: host does not support panel contributions")
	}
	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		return errors.New("SvcMgr: plugin is already initialized")
	}
	p.mu.Unlock()

	registration, err := host.RegisterPanelProvider(vfs.PanelProvider{
		ID:          panelProviderID,
		Title:       "Services",
		Description: "Windows services: name, display name, state and process id",
		Open: func(ctx vfs.PanelContext) (vfs.PanelController, error) {
			return newServicesPanel(ctx, listServices)
		},
	})
	if err != nil {
		return fmt.Errorf("SvcMgr: register panel provider: %w", err)
	}
	p.mu.Lock()
	p.registration = registration
	p.initialized = true
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registration := p.registration
	p.registration = nil
	p.initialized = false
	p.mu.Unlock()
	if registration != nil {
		registration.Unregister()
	}
	return nil
}
