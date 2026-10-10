package plughost

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

var extractedDriveSchemes = map[string]string{"iOS": "ios", "Android": "android", "CloudFox": "cloud"}

type extractedURIHost struct {
	vfs.HostAPI
	mu            sync.Mutex
	providers     map[string]*deferredURIProvider
	cloudProvider *deferredCloudProvider
}

func (h *extractedURIHost) prepare(scheme string) *deferredURIProvider {
	h.mu.Lock()
	defer h.mu.Unlock()
	if provider := h.providers[scheme]; provider != nil {
		return provider
	}
	provider := newDeferredURIProvider(scheme)
	if err := h.HostAPI.RegisterURIProvider(provider); err != nil {
		return nil
	}
	h.providers[scheme] = provider
	if scheme == "cloud" {
		h.cloudProvider = &deferredCloudProvider{provider: provider}
		h.HostAPI.RegisterVFSProvider(h.cloudProvider)
	}
	return provider
}

// CloudFox historically persists its friendly Account:/Folder address. Keep
// those standalone addresses accepted before its subprocess is ready, too.
type deferredCloudProvider struct{ provider *deferredURIProvider }

func (*deferredCloudProvider) Name() string                  { return "CloudFox-RPC" }
func (*deferredCloudProvider) Priority() int                 { return 220 }
func (*deferredCloudProvider) OpensStandalonePaths() bool    { return true }
func (*deferredCloudProvider) OpensVirtualDirectories() bool { return true }
func (*deferredCloudProvider) CanOpen(_ context.Context, _ vfs.VFS, raw string) bool {
	colon := strings.Index(raw, ":")
	return colon > 1 && colon+1 < len(raw) && (raw[colon+1] == '/' || raw[colon+1] == '\\') && !vfs.IsURIPath(raw)
}
func (p *deferredCloudProvider) Open(ctx context.Context, current vfs.VFS, raw string) (vfs.VFS, error) {
	return p.provider.OpenURI(ctx, current, raw)
}

func (h *extractedURIHost) RegisterDrive(name string, factory func() vfs.VFS) {
	scheme := extractedDriveSchemes[name]
	if scheme == "" {
		h.HostAPI.RegisterDrive(name, factory)
		return
	}
	provider := h.prepare(scheme)
	proxy, ok := factory().(*RPCVFS)
	if !ok || provider == nil {
		h.HostAPI.RegisterDrive(name, factory)
		return
	}
	provider.complete(proxy.sess, nil)
	h.HostAPI.RegisterDrive(name, func() vfs.VFS {
		mounted, err := provider.OpenURI(context.Background(), nil, scheme+"://")
		if err != nil {
			return factory()
		} // Older RPC binaries retain their drive.
		return mounted
	})
}

func (h *extractedURIHost) prepareInstalled() {
	for id := range GetInstalledPlugRingItems() {
		h.prepareID(id)
	}
	for _, entrypoint := range config.App.RegisteredPlugins {
		id := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(entrypoint), ".exe"), "-plugin")
		h.prepareID(id)
	}
}

func (h *extractedURIHost) prepareID(id string) {
	switch id {
	case "ios", "android":
		h.prepare(id)
	case "cloudfox":
		h.prepare("cloud")
	}
}

func (h *extractedURIHost) finish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for scheme, provider := range h.providers {
		provider.complete(nil, fmt.Errorf("%s plugin did not register its RPC drive", scheme))
	}
}

func (h *extractedURIHost) close() {
	h.finish()
	h.mu.Lock()
	defer h.mu.Unlock()
	for scheme := range h.providers {
		vfs.UnregisterURIProvider(scheme)
	}
	if h.cloudProvider != nil {
		vfs.UnregisterProvider(h.cloudProvider)
	}
}
