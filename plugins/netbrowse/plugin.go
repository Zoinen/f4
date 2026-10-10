package netbrowse

import (
	"errors"
	"fmt"
	"sync"

	"github.com/unxed/f4/vfs"
)

// panelProviderID is also the ID plughost derives the panel's command from
// ("panel.f4.netbrowse").
const panelProviderID = "f4.netbrowse"

// Plugin exposes the network browser as an in-process f4 panel plugin.
type Plugin struct {
	mu           sync.Mutex
	registration vfs.Registration
	initialized  bool
}

// NewPlugin constructs the built-in network browser plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "NetBrowse" }

// Init registers the panel provider. Off Windows it does nothing and returns
// nil: an absent WNet API is not a load failure.
func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("NetBrowse: nil host API")
	}
	if !Supported() {
		return nil
	}
	host, ok := api.(vfs.PanelContributionHost)
	if !ok {
		return errors.New("NetBrowse: host does not support panel contributions")
	}
	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		return errors.New("NetBrowse: plugin is already initialized")
	}
	p.mu.Unlock()

	registration, err := host.RegisterPanelProvider(vfs.PanelProvider{
		ID:          panelProviderID,
		Title:       "Network",
		Description: "The network: servers and their shares (on Windows also domains and workgroups)",
		Open: func(ctx vfs.PanelContext) (vfs.PanelController, error) {
			return newNetPanel(ctx, enumerateNetwork)
		},
	})
	if err != nil {
		return fmt.Errorf("NetBrowse: register panel provider: %w", err)
	}
	// network:// reaches the same file system by address.
	if err := api.RegisterURIProvider(&uriProvider{enum: enumerateNetwork, open: openShare, guess: guessResource}); err != nil {
		registration.Unregister()
		return fmt.Errorf("NetBrowse: register network:// provider: %w", err)
	}
	// The same network as a drive of the ordinary file panel (Alt+F1): shares
	// open as normal directories there.
	api.RegisterDrive("Network", func() vfs.VFS {
		v := newNetworkVFS(enumerateNetwork, openShare)
		v.guess = guessResource
		return v
	})

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
	closeSMBConns()
	return nil
}
