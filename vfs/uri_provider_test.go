package vfs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

type testURIProvider struct{ scheme string }

func (p *testURIProvider) Scheme() string { return p.scheme }
func (p *testURIProvider) OpenURI(context.Context, VFS, string) (VFS, error) {
	return nil, nil
}

func TestURIProviderRegistryNormalizesAndFindsScheme(t *testing.T) {
	provider := &testURIProvider{scheme: "Core-URI-Test"}
	if err := RegisterURIProvider(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { UnregisterURIProvider(provider.scheme) })
	if got := FindURIProvider("CORE-URI-TEST://profile/path"); got != provider {
		t.Fatalf("FindURIProvider returned %T, want registered provider", got)
	}
	if got := FindURIProvider("not-a-uri"); got != nil {
		t.Fatalf("plain path resolved to %T", got)
	}
	if scheme, ok := URIScheme("CoRe-UrI-TeSt://x"); !ok || scheme != "core-uri-test" {
		t.Fatalf("URIScheme = %q, %v", scheme, ok)
	}
}

func TestURIProviderRegistryRejectsInvalidAndDuplicateSchemes(t *testing.T) {
	if err := RegisterURIProvider(&testURIProvider{scheme: "1invalid"}); err == nil {
		t.Fatal("invalid scheme was accepted")
	}
	first := &testURIProvider{scheme: "core-uri-duplicate-test"}
	if err := RegisterURIProvider(first); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { UnregisterURIProvider(first.scheme) })
	if err := RegisterURIProvider(&testURIProvider{scheme: "CORE-URI-DUPLICATE-TEST"}); err == nil {
		t.Fatal("case-insensitive duplicate was accepted")
	}
}

func TestURIProviderRegistryConcurrentDuplicateRegistration(t *testing.T) {
	const workers = 32
	var successes atomic.Int32
	var wg sync.WaitGroup
	t.Cleanup(func() { UnregisterURIProvider("core-uri-race-test") })
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if RegisterURIProvider(&testURIProvider{scheme: "core-uri-race-test"}) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful registrations = %d, want 1", got)
	}
}
