package vfs

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type registryQuickViewProvider struct {
	name      string
	priority  int
	match     bool
	nameCalls int
}

func (p *registryQuickViewProvider) Name() string {
	p.nameCalls++
	return p.name
}
func (p *registryQuickViewProvider) Priority() int                    { return p.priority }
func (p *registryQuickViewProvider) CanPreview(QuickViewRequest) bool { return p.match }
func (p *registryQuickViewProvider) Preview(context.Context, QuickViewRequest) (QuickViewResult, error) {
	return QuickViewResult{}, ErrQuickViewUnsupported
}

func TestQuickViewProviderRegistryPriorityAndStableOrder(t *testing.T) {
	providers := []*registryQuickViewProvider{
		{name: "quick-view-registry-low", priority: 1, match: true},
		{name: "quick-view-registry-high-first", priority: 10, match: true},
		{name: "quick-view-registry-high-second", priority: 10, match: true},
		{name: "quick-view-registry-no-match", priority: 100, match: false},
	}
	for _, provider := range providers {
		registration, err := RegisterQuickViewProvider(provider)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(registration.Unregister)
	}

	got := QuickViewProvidersFor(QuickViewRequest{Path: "movie.mkv"})
	want := []QuickViewProvider{providers[1], providers[2], providers[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider order = %#v, want %#v", got, want)
	}
}

func TestQuickViewProviderRegistryRejectsInvalidAndDuplicateNames(t *testing.T) {
	if _, err := RegisterQuickViewProvider(nil); err == nil {
		t.Fatal("nil provider was accepted")
	}
	var typedNil *registryQuickViewProvider
	if _, err := RegisterQuickViewProvider(typedNil); err == nil {
		t.Fatal("typed nil provider was accepted")
	}
	if _, err := RegisterQuickViewProvider(&registryQuickViewProvider{name: "  "}); err == nil {
		t.Fatal("blank provider name was accepted")
	}

	first := &registryQuickViewProvider{name: "Quick-View-Registry-Duplicate"}
	registration, err := RegisterQuickViewProvider(first)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)
	duplicate := &registryQuickViewProvider{name: " quick-view-registry-duplicate "}
	if _, err := RegisterQuickViewProvider(duplicate); err == nil {
		t.Fatal("case-insensitive duplicate provider name was accepted")
	}
	if duplicate.nameCalls != 1 {
		t.Fatalf("duplicate provider Name called %d times; callbacks must happen before locking", duplicate.nameCalls)
	}
}

func TestQuickViewProviderRegistryUnregister(t *testing.T) {
	provider := &registryQuickViewProvider{name: "quick-view-registry-unregister", match: true}
	registration, err := RegisterQuickViewProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	registration.Unregister()
	registration.Unregister()
	if got := QuickViewProvidersFor(QuickViewRequest{}); containsQuickViewProvider(got, provider) {
		t.Fatal("unregistered provider remains in snapshot")
	}
	replacement, err := RegisterQuickViewProvider(&registryQuickViewProvider{name: provider.name, match: true})
	if err != nil {
		t.Fatalf("name could not be re-registered after unregister: %v", err)
	}
	registration.Unregister() // a stale handle must not remove the replacement
	if len(QuickViewProvidersFor(QuickViewRequest{})) == 0 {
		t.Fatal("stale registration handle removed a replacement provider")
	}
	replacement.Unregister()
	if !errors.Is(ErrQuickViewUnsupported, ErrQuickViewUnsupported) {
		t.Fatal("unsupported sentinel does not work with errors.Is")
	}
}

func containsQuickViewProvider(items []QuickViewProvider, target QuickViewProvider) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
