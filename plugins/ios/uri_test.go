package iosfs

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestIOSURIReopensDirectoryAndKeepsTransportPaths(t *testing.T) {
	device := DeviceInfo{UDID: "stable", Name: "Alexander's iPhone", Paired: true, State: DeviceStateReady}
	service := &fakeCoreService{entries: []coreEntry{{Name: "DCIM", IsDir: true}, {Name: "100APPLE", IsDir: true}}}
	p := &uriProvider{
		source: &fakeDeviceSource{devices: []DeviceInfo{device}},
		opener: DeviceOpenerFunc(func(_ context.Context, parent vfs.VFS, got DeviceInfo) (vfs.VFS, error) {
			if got.UDID != device.UDID {
				t.Fatal("device transport identity changed")
			}
			return newCoreVFS(parent, got, coreDomainAppData, "", iosDeviceTitle(got), service), nil
		}),
	}
	want := "ios://Alexander's iPhone/DCIM/100APPLE"
	mounted, err := p.OpenURI(t.Context(), vfs.NewNullVFS(0), want)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mounted.Close() })
	if mounted.GetPath() != want {
		t.Fatalf("path = %q", mounted.GetPath())
	}
	core := mounted.(*CoreVFS)
	if got, err := core.resolve(core.Join(core.GetPath(), "IMG_0007.JPG")); err != nil || got != "/DCIM/100APPLE/IMG_0007.JPG" {
		t.Fatalf("transport path = %q, %v", got, err)
	}
	if _, ok := mounted.ParentVFS().(*ManagerVFS); !ok {
		t.Fatal("manager parent lost")
	}
}

func TestIOSURIRejectsAmbiguousDeviceNames(t *testing.T) {
	devices := []DeviceInfo{
		{UDID: "first", Name: "Phone", Paired: true, State: DeviceStateReady},
		{UDID: "second", Name: "Phone", Paired: true, State: DeviceStateReady},
	}
	m := NewManagerVFS(&fakeDeviceSource{devices: devices}, nil)
	if err := m.ReadDir(t.Context(), m.GetPath(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.deviceForURI("Phone"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ambiguous name = %v", err)
	}
	for _, device := range devices {
		resolved, err := m.deviceForURI("Phone [" + device.UDID + "]")
		if err != nil || resolved.UDID != device.UDID {
			t.Fatalf("disambiguation = %#v, %v", resolved, err)
		}
	}
}

func TestIOSURIFailureClosesOpenedMount(t *testing.T) {
	device := DeviceInfo{UDID: "stable", Name: "Phone", Paired: true, State: DeviceStateReady}
	service := &fakeCoreService{}
	p := &uriProvider{
		source: &fakeDeviceSource{devices: []DeviceInfo{device}},
		opener: DeviceOpenerFunc(func(_ context.Context, parent vfs.VFS, got DeviceInfo) (vfs.VFS, error) {
			return newCoreVFS(parent, got, coreDomainAppData, "", iosDeviceTitle(got), service), nil
		}),
	}
	if _, err := p.OpenURI(t.Context(), nil, "ios://Phone/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory = %v", err)
	}
	if service.closes != 1 {
		t.Fatalf("failed mount closed %d times", service.closes)
	}
}
