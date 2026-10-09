// Package pdfview mounts a PDF in a file panel (f4#1665): Ctrl+PgDn on a .pdf
// shows its text (whole, and page by page) and the pictures embedded in it as
// read-only files, so F3 on a picture opens f4's own image viewer, terminal
// graphics included. The file is only read, through internal/pdftext; nothing
// in it is run, and vector drawing on a page is not rendered.
package pdfview

import (
	"errors"
	"sync"

	"github.com/unxed/f4/vfs"
)

// Plugin is the built-in PDF browser.
type Plugin struct {
	mu       sync.Mutex
	provider *provider
}

// NewPlugin constructs the plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "PDF Documents" }

func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("PDF Documents: nil host API")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return errors.New("PDF Documents: plugin is already initialized")
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
