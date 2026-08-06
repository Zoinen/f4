package androidfs

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/vfs"
)

func TestServerDeviceSourceMapsTransportFields(t *testing.T) {
	// The mapping itself is kept explicit so manager rows cannot accidentally
	// start depending on wire-only fields. Its behaviour is covered through the
	// pure helper below because Server transport framing has separate tests.
	device := Device{Serial: "abc", State: "device", Product: "p", Model: "Pixel", Device: "d", TransportID: "7"}
	got := DeviceInfo{
		Serial: device.Serial, State: device.State, Product: device.Product,
		Model: device.Model, Device: device.Device, TransportID: device.TransportID,
	}
	want := DeviceInfo{Serial: "abc", State: "device", Product: "p", Model: "Pixel", Device: "d", TransportID: "7"}
	if got != want {
		t.Fatalf("mapped device = %#v, want %#v", got, want)
	}
}

func TestHybridOpenerPrefersFishForShellV2(t *testing.T) {
	fishVFS := vfs.NewNullVFS(0)
	syncVFS := vfs.NewNullVFS(0)
	fishCalls, syncCalls := 0, 0
	opener := &hybridDeviceOpener{
		features: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{"shell_v2": true}, nil
		},
		openFish: func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) {
			fishCalls++
			return fishVFS, nil
		},
		openSync: func(context.Context, vfs.VFS, DeviceInfo, map[string]bool) (vfs.VFS, error) {
			syncCalls++
			return syncVFS, nil
		},
	}
	got, err := opener.OpenDevice(context.Background(), nil, DeviceInfo{Serial: "serial", State: DeviceStateOnline})
	if err != nil || got != fishVFS || fishCalls != 1 || syncCalls != 0 {
		t.Fatalf("OpenDevice = %T, %v; calls fish=%d sync=%d", got, err, fishCalls, syncCalls)
	}
}

func TestHybridOpenerAttachesInfoWithoutWrappingBackend(t *testing.T) {
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline, Model: "phone"}
	info := newDeviceInfoService(nil)

	t.Run("Fish", func(t *testing.T) {
		fish := &netfox.FishVFS{}
		opener := &hybridDeviceOpener{
			info: info,
			features: func(context.Context, string) (map[string]bool, error) {
				return map[string]bool{"shell_v2": true}, nil
			},
			openFish: func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) { return fish, nil },
		}
		mounted, err := opener.OpenDevice(context.Background(), nil, device)
		if err != nil || mounted != fish {
			t.Fatalf("mounted = %T, %v; want the original *FishVFS", mounted, err)
		}
		provider, ok := mounted.(vfs.PanelInfoProvider)
		if !ok || provider.PanelInfoKey(vfs.PanelInfoRequest{Path: "/"}) == "" {
			t.Fatal("raw FishVFS has no attached Android panel-info provider")
		}
	})

	t.Run("Sync", func(t *testing.T) {
		syncVFS := newSyncVFS(nil, device.Serial, "phone", &fakeSyncFS{}, nil)
		opener := &hybridDeviceOpener{
			info:     info,
			features: func(context.Context, string) (map[string]bool, error) { return map[string]bool{"stat_v2": true}, nil },
			openSync: func(context.Context, vfs.VFS, DeviceInfo, map[string]bool) (vfs.VFS, error) { return syncVFS, nil },
		}
		mounted, err := opener.OpenDevice(context.Background(), nil, device)
		if err != nil || mounted != syncVFS {
			t.Fatalf("mounted = %T, %v; want the original *SyncVFS", mounted, err)
		}
		provider, ok := mounted.(vfs.PanelInfoProvider)
		if !ok || provider.PanelInfoKey(vfs.PanelInfoRequest{Path: "/"}) == "" {
			t.Fatal("raw SyncVFS has no attached Android panel-info provider")
		}
	})
}

func TestHybridOpenerLocksToSyncAfterFishSetupFailure(t *testing.T) {
	wantErr := errors.New("helper incompatible")
	syncVFS := vfs.NewNullVFS(0)
	fishCalls, syncCalls := 0, 0
	opener := &hybridDeviceOpener{
		features: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{"shell_v2": true, "stat_v2": true}, nil
		},
		openFish: func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) {
			fishCalls++
			return nil, wantErr
		},
		openSync: func(_ context.Context, _ vfs.VFS, _ DeviceInfo, features map[string]bool) (vfs.VFS, error) {
			syncCalls++
			if !features["stat_v2"] {
				t.Fatal("features were not retained for Sync")
			}
			return syncVFS, nil
		},
	}
	got, err := opener.OpenDevice(context.Background(), nil, DeviceInfo{Serial: "serial", State: DeviceStateOnline})
	if err != nil || got != syncVFS || fishCalls != 1 || syncCalls != 1 {
		t.Fatalf("OpenDevice = %T, %v; calls fish=%d sync=%d", got, err, fishCalls, syncCalls)
	}
}

func TestHybridOpenerSkipsFishWithoutShellV2(t *testing.T) {
	syncVFS := vfs.NewNullVFS(0)
	fishCalls := 0
	opener := &hybridDeviceOpener{
		features: func(context.Context, string) (map[string]bool, error) { return map[string]bool{}, nil },
		openFish: func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) {
			fishCalls++
			return nil, nil
		},
		openSync: func(context.Context, vfs.VFS, DeviceInfo, map[string]bool) (vfs.VFS, error) {
			return syncVFS, nil
		},
	}
	got, err := opener.OpenDevice(context.Background(), nil, DeviceInfo{Serial: "serial", State: DeviceStateOnline})
	if err != nil || got != syncVFS || fishCalls != 0 {
		t.Fatalf("OpenDevice = %T, %v; fish calls=%d", got, err, fishCalls)
	}
}

func TestHybridOpenerReportsFeatureAndBackendFailures(t *testing.T) {
	featureErr := errors.New("adb unavailable")
	opener := &hybridDeviceOpener{features: func(context.Context, string) (map[string]bool, error) {
		return nil, featureErr
	}}
	if _, err := opener.OpenDevice(context.Background(), nil, DeviceInfo{Serial: "serial", State: DeviceStateOnline}); !errors.Is(err, featureErr) {
		t.Fatalf("feature error = %v", err)
	}
	if _, err := opener.OpenDevice(context.Background(), nil, DeviceInfo{Serial: "serial", State: DeviceStateOffline}); !errors.Is(err, ErrDeviceUnavailable) {
		t.Fatalf("offline error = %v", err)
	}

	fishErr, syncErr := errors.New("fish failed"), errors.New("sync failed")
	opener = &hybridDeviceOpener{
		features: func(context.Context, string) (map[string]bool, error) { return map[string]bool{"shell_v2": true}, nil },
		openFish: func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) { return nil, fishErr },
		openSync: func(context.Context, vfs.VFS, DeviceInfo, map[string]bool) (vfs.VFS, error) { return nil, syncErr },
	}
	if _, err := opener.OpenDevice(context.Background(), nil, DeviceInfo{Serial: "serial", State: DeviceStateOnline}); !errors.Is(err, syncErr) {
		t.Fatalf("combined backend error = %v", err)
	}
}

func TestDeviceSessionTitleUsesOnlyDeviceName(t *testing.T) {
	device := DeviceInfo{Serial: "abc", Model: "Pixel", State: DeviceStateOnline}
	if got := deviceSessionTitle(device); got != "Pixel" {
		t.Fatalf("title = %q", got)
	}
	if got := deviceSessionTitle(DeviceInfo{Serial: "serial-only"}); got != "serial-only" {
		t.Fatalf("serial fallback title = %q", got)
	}
}

func TestAndroidPanelTitleUsesBackslashes(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/", want: `Pixel:\`},
		{path: "/sdcard", want: `Pixel:\sdcard`},
		{path: "/sdcard/Download", want: `Pixel:\sdcard\Download`},
	} {
		if got := androidPanelTitle("Pixel", tc.path); got != tc.want {
			t.Errorf("androidPanelTitle(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
