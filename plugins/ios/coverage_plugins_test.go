package iosfs

import (
	"context"
	"errors"
	"testing"

	goios "github.com/danielpaulus/go-ios/ios"
)

func TestNativeEntrySelectionPrefersUSBAndStableID(t *testing.T) {
	usb := goios.DeviceEntry{DeviceID: 20, Properties: goios.DeviceProperties{ConnectionType: "USB"}}
	wifi := goios.DeviceEntry{DeviceID: 10, Properties: goios.DeviceProperties{ConnectionType: "Network"}}
	if !preferNativeEntry(usb, wifi) {
		t.Fatal("USB entry should be preferred over network entry")
	}
	if preferNativeEntry(wifi, usb) {
		t.Fatal("network entry should not replace USB entry")
	}

	lowerID := goios.DeviceEntry{DeviceID: 3, Properties: goios.DeviceProperties{ConnectionType: "USB"}}
	higherID := goios.DeviceEntry{DeviceID: 7, Properties: goios.DeviceProperties{ConnectionType: "USB"}}
	if !preferNativeEntry(lowerID, higherID) || preferNativeEntry(higherID, lowerID) {
		t.Fatal("same-transport entries should use the stable lower device ID")
	}
}

func TestNativeBackendGuardsUnavailableAndUnknownSelections(t *testing.T) {
	backend := &nativeBackend{}
	_, err := backend.OpenDevice(context.Background(), nil, DeviceInfo{UDID: "offline", Paired: true, State: DeviceStateOffline})
	if !errors.Is(err, ErrDeviceUnavailable) {
		t.Fatalf("OpenDevice error = %v, want ErrDeviceUnavailable", err)
	}

	applications, err := backend.OpenSelection(context.Background(), nil, DeviceInfo{}, CapabilityApplications)
	if err != nil {
		t.Fatalf("OpenSelection applications: %v", err)
	}
	if _, ok := applications.(*ApplicationsVFS); !ok {
		t.Fatalf("applications selection returned %T, want *ApplicationsVFS", applications)
	}

	if _, err := backend.OpenSelection(context.Background(), nil, DeviceInfo{}, Capability(255)); err == nil {
		t.Fatal("unknown capability unexpectedly succeeded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.OpenSelection(ctx, nil, DeviceInfo{}, CapabilityAppGroups); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled app-groups selection error = %v, want context.Canceled", err)
	}
	if _, err := backend.OpenApp(ctx, nil, DeviceInfo{}, AppInfo{BundleID: "com.example.app"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled app selection error = %v, want context.Canceled", err)
	}
}

func TestNativeSourcesAndServicesHonorCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var source nativeDeviceSource
	if _, err := source.ListDevices(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListDevices error = %v, want context.Canceled", err)
	}
	if _, err := resolveNativeDevice(ctx, DeviceInfo{UDID: "canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolveNativeDevice error = %v, want context.Canceled", err)
	}
	if _, err := resolveAppDevice(ctx, DeviceInfo{UDID: "canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolveAppDevice error = %v, want context.Canceled", err)
	}
	if _, err := (nativeAppSource{}).ListApps(ctx, DeviceInfo{UDID: "canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListApps error = %v, want context.Canceled", err)
	}
	if _, err := connectService(ctx, goios.DeviceEntry{}, "service"); !errors.Is(err, context.Canceled) {
		t.Fatalf("connectService error = %v, want context.Canceled", err)
	}
	if _, err := openHouseArrest(ctx, goios.DeviceEntry{}, "com.example.app", vendDocuments); !errors.Is(err, context.Canceled) {
		t.Fatalf("openHouseArrest error = %v, want context.Canceled", err)
	}
	if _, err := openCrashReportService(ctx, goios.DeviceEntry{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("openCrashReportService error = %v, want context.Canceled", err)
	}
}

func TestPluginNilBackendLifecycle(t *testing.T) {
	plugin := &Plugin{}
	if err := plugin.Init(&recordingHost{}); err == nil {
		t.Fatal("Init with nil backend unexpectedly succeeded")
	}
	if err := plugin.Close(); err != nil {
		t.Fatalf("Close with nil backend: %v", err)
	}
}
