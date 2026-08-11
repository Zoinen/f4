package vfs

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// URIProvider restores a VFS from a persistent hierarchical URI. Init must
// register providers without doing network I/O; OpenURI runs on a cancellable
// background task owned by the destination panel.
type URIProvider interface {
	Scheme() string
	OpenURI(ctx context.Context, current VFS, uri string) (VFS, error)
}

var uriProviders = struct {
	sync.RWMutex
	byScheme map[string]URIProvider
}{byScheme: make(map[string]URIProvider)}

// RegisterURIProvider installs exactly one provider for a URI scheme. Scheme
// names are case-insensitive and are stored in lower case.
func RegisterURIProvider(provider URIProvider) error {
	if provider == nil {
		return fmt.Errorf("register URI provider: nil provider")
	}
	scheme := strings.ToLower(strings.TrimSpace(provider.Scheme()))
	if !validURIScheme(scheme) {
		return fmt.Errorf("register URI provider: invalid scheme %q", provider.Scheme())
	}

	uriProviders.Lock()
	defer uriProviders.Unlock()
	if _, exists := uriProviders.byScheme[scheme]; exists {
		return fmt.Errorf("register URI provider: scheme %q is already registered", scheme)
	}
	uriProviders.byScheme[scheme] = provider
	return nil
}

// UnregisterURIProvider removes the provider for scheme. It is intended for
// plugin unload/reload and tests; callers must only remove schemes they own.
func UnregisterURIProvider(scheme string) bool {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	uriProviders.Lock()
	defer uriProviders.Unlock()
	if _, ok := uriProviders.byScheme[scheme]; !ok {
		return false
	}
	delete(uriProviders.byScheme, scheme)
	return true
}

// FindURIProvider returns the provider registered for rawURI. Only
// hierarchical scheme:// paths are treated as persistent VFS URIs, keeping
// Windows volume paths and ordinary relative paths out of this registry.
func FindURIProvider(rawURI string) URIProvider {
	scheme, ok := URIScheme(rawURI)
	if !ok {
		return nil
	}
	uriProviders.RLock()
	provider := uriProviders.byScheme[scheme]
	uriProviders.RUnlock()
	return provider
}

// URIScheme extracts and normalizes the scheme from a hierarchical VFS URI.
func URIScheme(path string) (string, bool) {
	separator := strings.Index(path, "://")
	if separator <= 0 {
		return "", false
	}
	scheme := strings.ToLower(path[:separator])
	if !validURIScheme(scheme) {
		return "", false
	}
	return scheme, true
}

// IsURIPath reports whether path has the hierarchical URI shape used by
// persistent virtual file-system locations. Registration is intentionally not
// required so history maintenance does not discard temporarily unavailable
// plugin paths.
func IsURIPath(path string) bool {
	_, ok := URIScheme(path)
	return ok
}

func validURIScheme(scheme string) bool {
	if scheme == "" || !isASCIIAlpha(scheme[0]) {
		return false
	}
	for i := 1; i < len(scheme); i++ {
		c := scheme[i]
		if !isASCIIAlpha(c) && (c < '0' || c > '9') && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
