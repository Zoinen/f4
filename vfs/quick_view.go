package vfs

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ErrQuickViewUnsupported tells the host that a provider cannot describe this
// particular file after all. The next matching provider is tried; when every
// provider declines, Quick View falls back to its ordinary text/hex preview.
var ErrQuickViewUnsupported = errors.New("quick-view provider: unsupported")

// QuickViewRequest identifies one regular file selected in a file panel.
// Path is expressed in VFS' namespace and is not necessarily a local OS path.
// VFS is borrowed from the host: providers may open files through it but must
// neither mutate its current path nor close it. Item is an immutable snapshot.
type QuickViewRequest struct {
	VFS  VFS
	Path string
	Item VFSItem
}

// QuickViewResult is plain text rendered by the host. Label is an optional,
// localized one-line description shown in the file header; the host owns line
// wrapping, scrolling and clipping for Lines.
type QuickViewResult struct {
	Label string
	Lines []string
}

// QuickViewProvider contributes a specialized Ctrl+Q preview. Name is a
// stable registry identifier, not a display label. Higher priorities run
// first. CanPreview must be local, bounded and free of VFS I/O; Preview runs on
// a cancellable background task and must honor ctx.
type QuickViewProvider interface {
	Name() string
	Priority() int
	CanPreview(QuickViewRequest) bool
	Preview(context.Context, QuickViewRequest) (QuickViewResult, error)
}

type quickViewProviderEntry struct {
	id       uint64
	key      string
	provider QuickViewProvider
}

var quickViewProviders = struct {
	sync.RWMutex
	nextID uint64
	items  []quickViewProviderEntry
}{}

type quickViewRegistration struct {
	once sync.Once
	id   uint64
}

func (registration *quickViewRegistration) Unregister() {
	if registration == nil {
		return
	}
	registration.once.Do(func() {
		quickViewProviders.Lock()
		defer quickViewProviders.Unlock()
		for i, entry := range quickViewProviders.items {
			if entry.id == registration.id {
				quickViewProviders.items = append(quickViewProviders.items[:i], quickViewProviders.items[i+1:]...)
				return
			}
		}
	})
}

// RegisterQuickViewProvider installs one provider. Names are trimmed and
// compared case-insensitively so reloads cannot silently leave two providers
// with the same identity in the dispatch chain.
func RegisterQuickViewProvider(provider QuickViewProvider) (Registration, error) {
	if isNilQuickViewProvider(provider) {
		return nil, fmt.Errorf("register quick-view provider: nil provider")
	}
	name := provider.Name()
	key := quickViewProviderKey(name)
	if key == "" {
		return nil, fmt.Errorf("register quick-view provider: empty name")
	}

	quickViewProviders.Lock()
	defer quickViewProviders.Unlock()
	for _, entry := range quickViewProviders.items {
		if entry.key == key {
			return nil, fmt.Errorf("register quick-view provider: name %q is already registered", name)
		}
	}
	quickViewProviders.nextID++
	id := quickViewProviders.nextID
	quickViewProviders.items = append(quickViewProviders.items, quickViewProviderEntry{id: id, key: key, provider: provider})
	return &quickViewRegistration{id: id}, nil
}

// QuickViewProvidersFor returns a stable-priority snapshot of providers that
// cheaply claim req. Registry locks are released before any provider method is
// invoked, so registration and unload cannot deadlock with plugin code.
func QuickViewProvidersFor(req QuickViewRequest) []QuickViewProvider {
	quickViewProviders.RLock()
	entries := append([]quickViewProviderEntry(nil), quickViewProviders.items...)
	quickViewProviders.RUnlock()

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].provider.Priority() > entries[j].provider.Priority()
	})

	providers := make([]QuickViewProvider, 0, len(entries))
	for _, entry := range entries {
		if entry.provider.CanPreview(req) {
			providers = append(providers, entry.provider)
		}
	}
	return providers
}

func quickViewProviderKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isNilQuickViewProvider(provider QuickViewProvider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
