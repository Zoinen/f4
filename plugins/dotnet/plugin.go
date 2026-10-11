// Package dotnet mounts a .NET assembly in a file panel (f4#1666): Ctrl+PgDn
// on a .dll or .exe that holds .NET metadata shows its identity, references,
// types by namespace and resource names as read-only folders and files. The
// file is only read, through internal/dotnet; nothing in it is loaded or run.
package dotnet

import (
	"errors"
	"sync"

	"github.com/unxed/f4/vfs"
)

// Plugin is the built-in assembly browser.
type Plugin struct {
	mu       sync.Mutex
	provider *provider
}

// NewPlugin constructs the plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return ".NET Assemblies" }

func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New(".NET Assemblies: nil host API")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return errors.New(".NET Assemblies: plugin is already initialized")
	}
	p.provider = &provider{}
	api.RegisterVFSProvider(p.provider)
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	provider := p.provider
	p.provider = nil
	p.mu.Unlock()
	if provider != nil {
		vfs.UnregisterProvider(provider)
	}
	return nil
}
