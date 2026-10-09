package git

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

type fakeRegistration struct{ unregistered bool }

func (r *fakeRegistration) Unregister() { r.unregistered = true }

// fakePanelHost implements vfs.PanelContributionHost only, the same minimal
// fake plugins/proclist/plugin_test.go uses for the same interface.
type fakePanelHost struct {
	vfs.HostAPI
	provider vfs.PanelProvider
	reg      *fakeRegistration
}

func (h *fakePanelHost) RegisterPanelProvider(p vfs.PanelProvider) (vfs.Registration, error) {
	h.provider = p
	h.reg = &fakeRegistration{}
	return h.reg, nil
}

func TestInitRegistersPanelProvider(t *testing.T) {
	p := NewPlugin()
	host := &fakePanelHost{}
	if err := p.Init(host); err != nil {
		t.Fatalf("Init returned an error: %v", err)
	}
	if host.provider.ID != panelProviderID {
		t.Fatalf("registered provider ID = %q, want %q", host.provider.ID, panelProviderID)
	}
	if host.provider.Open == nil {
		t.Fatal("registered provider has no Open callback")
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
	if !host.reg.unregistered {
		t.Fatal("Close did not unregister the panel provider")
	}
}

func TestInitRejectsNilHostAPI(t *testing.T) {
	if err := NewPlugin().Init(nil); err == nil {
		t.Fatal("Init(nil) should return an error")
	}
}

// hostWithoutPanels implements vfs.HostAPI but not vfs.PanelContributionHost,
// the shape a host that predates panel-only plugins (f4#658) would have.
type hostWithoutPanels struct{ vfs.HostAPI }

func TestInitRejectsHostWithoutPanelSupport(t *testing.T) {
	if err := NewPlugin().Init(hostWithoutPanels{}); err == nil {
		t.Fatal("Init should fail against a host without vfs.PanelContributionHost")
	}
}

func TestInitRejectsDoubleInitialization(t *testing.T) {
	p := NewPlugin()
	if err := p.Init(&fakePanelHost{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Init(&fakePanelHost{}); err == nil {
		t.Fatal("second Init on the same plugin should fail")
	}
}

func TestGetName(t *testing.T) {
	if got := NewPlugin().GetName(); got != "Git" {
		t.Fatalf("GetName() = %q, want %q", got, "Git")
	}
}

func TestCloseWithoutInitIsSafe(t *testing.T) {
	if err := NewPlugin().Close(); err != nil {
		t.Fatalf("Close on an uninitialized plugin returned an error: %v", err)
	}
}
