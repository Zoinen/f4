package iosfs

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

type recordingHost struct {
	vfs.HostAPI
	drives    map[string]func() vfs.VFS
	providers []vfs.VFSProvider
}

func (h *recordingHost) RegisterDrive(name string, factory func() vfs.VFS) {
	if h.drives == nil {
		h.drives = make(map[string]func() vfs.VFS)
	}
	h.drives[name] = factory
}

func (h *recordingHost) RegisterVFSProvider(provider vfs.VFSProvider) {
	h.providers = append(h.providers, provider)
}

func TestPluginRegistersIOSDriveAndProviders(t *testing.T) {
	plugin := NewPlugin()
	host := &recordingHost{}

	if err := plugin.Init(host); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := plugin.GetName(); got != "iOS" {
		t.Fatalf("GetName = %q, want iOS", got)
	}
	factory := host.drives["iOS"]
	if factory == nil {
		t.Fatal("iOS drive was not registered")
	}
	manager, ok := factory().(*ManagerVFS)
	if !ok {
		t.Fatalf("registered factory returned %T", factory())
	}
	if _, ok := manager.source.(nativeDeviceSource); !ok {
		t.Fatalf("manager source = %T, want nativeDeviceSource", manager.source)
	}
	if manager.opener != plugin.backend {
		t.Fatalf("manager opener = %T, want plugin backend", manager.opener)
	}

	wantProviders := []string{"iOS-device", "iOS-capability", "iOS-application", "iOS-app-group"}
	if len(host.providers) != len(wantProviders) {
		t.Fatalf("provider count = %d, want %d", len(host.providers), len(wantProviders))
	}
	for i, want := range wantProviders {
		if got := host.providers[i].Name(); got != want {
			t.Errorf("provider %d = %q, want %q", i, got, want)
		}
	}

	if err := plugin.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
