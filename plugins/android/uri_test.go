package androidfs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestAndroidURIReopensPublicItemByStableSerial(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{
		"/sdcard/DCIM":            {Mode: remoteModeDir | 0755},
		"/sdcard/DCIM/a'b #?.txt": {Mode: 0100644},
	}}
	for _, name := range []string{"Pixel 3", "Phone's / USB #1?"} {
		t.Run(name, func(t *testing.T) {
			plugin := &Plugin{
				Source: &fakeDeviceSource{devices: []DeviceInfo{{Serial: "serial", Model: name, State: DeviceStateOnline}}},
				Opener: DeviceOpenerFunc(func(_ context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
					if device.Serial != "serial" {
						t.Fatalf("transport serial = %q", device.Serial)
					}
					return newSyncVFS(parent, device.Serial, deviceSessionTitle(device), client, nil), nil
				}),
			}
			paths := vfs.DevicePath{Scheme: "android", Device: name}
			raw := paths.Public("/sdcard/DCIM/a'b #?.txt")
			mounted, err := (&androidURIProvider{plugin: plugin}).OpenURI(context.Background(), nil, raw)
			if err != nil {
				t.Fatal(err)
			}
			defer mounted.Close()
			if mounted.GetPath() != paths.Public("/sdcard/DCIM") {
				t.Fatalf("directory = %q", mounted.GetPath())
			}
			if _, err := mounted.Stat(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAndroidURIDuplicateNamesAndUnavailableDevices(t *testing.T) {
	source := &fakeDeviceSource{devices: []DeviceInfo{
		{Serial: "b", Model: "Pixel 3", State: DeviceStateOnline},
		{Serial: "a", Model: "Pixel 3", State: DeviceStateOnline},
	}}
	openedSerial := ""
	plugin := &Plugin{Source: source, Opener: DeviceOpenerFunc(func(_ context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
		openedSerial = device.Serial
		return newSyncVFS(parent, device.Serial, deviceSessionTitle(device), &fakeSyncFS{
			entries: map[string]SyncEntry{"/": {Mode: remoteModeDir | 0755}},
		}, nil), nil
	})}
	provider := &androidURIProvider{plugin: plugin}
	if _, err := provider.OpenURI(context.Background(), nil, "android://Pixel 3/"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous name: %v", err)
	}
	for range 2 {
		mounted, err := provider.OpenURI(context.Background(), nil, "android://Pixel 3 (a)/")
		if err != nil {
			t.Fatal(err)
		}
		if openedSerial != "a" || mounted.GetPath() != (vfs.DevicePath{Scheme: "android", Device: "Pixel 3 (a)"}).Root() {
			t.Fatalf("resolved %q at %q", openedSerial, mounted.GetPath())
		}
		if err := mounted.Close(); err != nil {
			t.Fatal(err)
		}
		source.devices[0], source.devices[1] = source.devices[1], source.devices[0]
	}
	source.devices = []DeviceInfo{{Serial: "a", Model: "Pixel 3", State: DeviceStateOffline}}
	if _, err := provider.OpenURI(context.Background(), nil, "android://Pixel 3/"); !errors.Is(err, ErrDeviceUnavailable) {
		t.Fatalf("offline: %v", err)
	}
}

func TestAndroidURIRegistrationFailureDoesNotRemoveOwner(t *testing.T) {
	owner := &Plugin{}
	if err := owner.Init(&recordingHost{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	other := &Plugin{}
	if err := other.Init(&recordingHost{}); err == nil {
		t.Fatal("duplicate registration succeeded")
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if vfs.FindURIProvider(androidRoot) != owner.uri {
		t.Fatal("removed another plugin's registration")
	}
}
