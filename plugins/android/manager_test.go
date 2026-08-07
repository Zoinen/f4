package androidfs

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
)

type fakeDeviceSource struct {
	devices []DeviceInfo
	err     error
	calls   int
}

func (s *fakeDeviceSource) ListDevices(context.Context) ([]DeviceInfo, error) {
	s.calls++
	return append([]DeviceInfo(nil), s.devices...), s.err
}

type fakeDeviceOpener struct {
	result vfs.VFS
	err    error
	calls  int
	parent vfs.VFS
	device DeviceInfo
}

func (o *fakeDeviceOpener) OpenDevice(_ context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	o.calls++
	o.parent = parent
	o.device = device
	return o.result, o.err
}

func readManagerItems(t *testing.T, manager *ManagerVFS) ([]vfs.VFSItem, error) {
	t.Helper()
	var items []vfs.VFSItem
	err := manager.ReadDir(context.Background(), manager.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	})
	return items, err
}

func TestManagerReadDirDiscoversAndLabelsDevices(t *testing.T) {
	source := &fakeDeviceSource{devices: []DeviceInfo{
		{Serial: "serial-z", State: DeviceStateOnline, Model: "Pixel 9"},
		{Serial: "serial-a", State: DeviceStateUnauthorized, Model: "Tablet"},
		{Serial: "serial-m", State: DeviceStateOffline},
		{State: DeviceStateOnline, Model: "missing serial"},
	}}
	manager := NewManagerVFS(source, &fakeDeviceOpener{})

	items, err := readManagerItems(t, manager)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if source.calls != 1 {
		t.Fatalf("discovery calls = %d, want 1", source.calls)
	}

	gotNames := make([]string, len(items))
	gotExecutable := make([]bool, len(items))
	for i, item := range items {
		gotNames[i] = item.Name
		gotExecutable[i] = item.IsExecutable
		if !item.IsDir {
			t.Errorf("device row %q is not marked as directory", item.Name)
		}
		if !item.NoExtension {
			t.Errorf("device row %q may be mistaken for a filename with an extension", item.Name)
		}
	}
	wantNames := []string{
		"Pixel 9 (serial-z)",
		"Tablet (serial-a) [unauthorized]",
		"serial-m [offline]",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("names = %#v, want %#v", gotNames, wantNames)
	}
	if !reflect.DeepEqual(gotExecutable, []bool{true, false, false}) {
		t.Fatalf("executable flags = %#v", gotExecutable)
	}
	statItem, err := manager.Stat(context.Background(), "Pixel 9 (serial-z)")
	if err != nil {
		t.Fatalf("Stat online device: %v", err)
	}
	if !statItem.IsDir {
		t.Fatalf("Stat online device is not a directory: %#v", statItem)
	}

	// Refresh performs discovery again and atomically replaces stale rows.
	source.devices = []DeviceInfo{{Serial: "new", State: DeviceStateOnline, Model: "New phone"}}
	items, err = readManagerItems(t, manager)
	if err != nil {
		t.Fatalf("second ReadDir: %v", err)
	}
	if source.calls != 2 {
		t.Fatalf("discovery calls = %d, want 2", source.calls)
	}
	if len(items) != 1 || items[0].Name != "New phone (new)" {
		t.Fatalf("refreshed rows = %#v", items)
	}
	if _, err := manager.Stat(context.Background(), "Pixel 9 (serial-z)"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale device Stat error = %v, want os.ErrNotExist", err)
	}
}

func TestDeviceProviderOpensVirtualDirectories(t *testing.T) {
	provider := &deviceProvider{}
	if !provider.OpensVirtualDirectories() {
		t.Fatal("device provider does not open directory-rendered device rows")
	}
}

func TestManagerDiscoveryFailureClearsOpenableRows(t *testing.T) {
	source := &fakeDeviceSource{devices: []DeviceInfo{{Serial: "serial", State: DeviceStateOnline}}}
	manager := NewManagerVFS(source, &fakeDeviceOpener{})
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatalf("initial ReadDir: %v", err)
	}

	source.err = errors.New("adb unavailable")
	if _, err := readManagerItems(t, manager); !errors.Is(err, source.err) {
		t.Fatalf("ReadDir error = %v, want %v", err, source.err)
	}
	provider := &deviceProvider{}
	if provider.CanOpen(context.Background(), manager, "serial") {
		t.Fatal("provider accepted stale row after discovery failure")
	}
}

func TestManagerCanceledRefreshPreservesLastDiscoveredDevices(t *testing.T) {
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline, Model: "Phone"}
	source := &fakeDeviceSource{devices: []DeviceInfo{device}}
	manager := NewManagerVFS(source, &fakeDeviceOpener{})
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatalf("initial ReadDir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source.err = context.Canceled
	if err := manager.ReadDir(ctx, manager.GetPath(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ReadDir error = %v, want context.Canceled", err)
	}

	name := DeviceDisplayName(device)
	item, err := manager.Stat(context.Background(), name)
	if err != nil || item.Name != name {
		t.Fatalf("last discovered device after canceled refresh = %#v, %v", item, err)
	}
	provider := &deviceProvider{}
	if !provider.CanOpen(context.Background(), manager, name) {
		t.Fatal("canceled refresh made the selected device unopenable")
	}
}

func TestDeviceProviderIsNarrowAndDelegatesOnlineDevice(t *testing.T) {
	device := DeviceInfo{Serial: "abc123", State: DeviceStateOnline, Model: "Pixel"}
	source := &fakeDeviceSource{devices: []DeviceInfo{
		device,
		{Serial: "offline", State: DeviceStateOffline, Model: "Old phone"},
	}}
	target := vfs.NewNullVFS(0)
	opener := &fakeDeviceOpener{result: target}
	manager := NewManagerVFS(source, opener)
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	provider := &deviceProvider{}
	ctx := context.Background()

	if provider.CanOpen(ctx, vfs.NewNullVFS(0), "Pixel (abc123)") {
		t.Fatal("provider accepted a row outside ManagerVFS")
	}
	if !provider.CanOpen(ctx, manager, "android://Pixel (abc123)") {
		t.Fatal("provider rejected online manager row")
	}
	if provider.CanOpen(ctx, manager, "Old phone (offline) [offline]") {
		t.Fatal("provider accepted offline device")
	}
	if provider.CanOpen(ctx, manager, "unknown") {
		t.Fatal("provider accepted unknown manager row")
	}

	opened, err := provider.Open(ctx, manager, "Pixel (abc123)")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened != target || opener.calls != 1 || opener.parent != manager || opener.device != device {
		t.Fatalf("opener delegation mismatch: opened=%T calls=%d parent=%T device=%#v", opened, opener.calls, opener.parent, opener.device)
	}
	if _, err := provider.Open(ctx, manager, "Old phone (offline) [offline]"); !errors.Is(err, ErrDeviceUnavailable) {
		t.Fatalf("offline Open error = %v, want ErrDeviceUnavailable", err)
	}
	if _, err := provider.Open(ctx, vfs.NewNullVFS(0), "anything"); err == nil {
		t.Fatal("Open accepted non-manager parent")
	}
}

func TestManagerVFSContract(t *testing.T) {
	source := &fakeDeviceSource{}
	opener := &fakeDeviceOpener{}
	manager := NewManagerVFS(source, opener)

	if !manager.IsAtRoot() || manager.GetPath() != androidRoot || manager.GetTitle() != "Android" {
		t.Fatalf("root identity mismatch: atRoot=%v path=%q title=%q", manager.IsAtRoot(), manager.GetPath(), manager.GetTitle())
	}
	if got := manager.PanelTitle(manager.GetPath()); got != "Android devices" {
		t.Fatalf("panel title = %q, want %q", got, "Android devices")
	}
	if manager.ParentVFS() != nil {
		t.Fatal("manager unexpectedly has a parent")
	}
	if got := manager.Join(androidRoot, "Pixel (serial)"); got != "android://Pixel (serial)" {
		t.Fatalf("Join = %q", got)
	}
	if got, err := manager.Abs("Pixel (serial)"); err != nil || got != "android://Pixel (serial)" {
		t.Fatalf("Abs = %q, %v", got, err)
	}
	if got := manager.Base("android://Pixel (serial)"); got != "Pixel (serial)" {
		t.Fatalf("Base = %q", got)
	}
	if err := manager.SetPath(androidRoot); err != nil {
		t.Fatalf("SetPath(root): %v", err)
	}
	if err := manager.SetPath("android://child"); err == nil {
		t.Fatal("SetPath accepted child directory")
	}
	if capabilities := manager.GetCapabilities(); capabilities != (vfs.VFSCapabilities{}) {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	if err := manager.MkDir(context.Background(), "x"); !errors.Is(err, ErrManagerReadOnly) {
		t.Fatalf("MkDir error = %v", err)
	}

	clone, ok := manager.Clone().(*ManagerVFS)
	if !ok || clone == manager || clone.source != source || clone.opener != opener {
		t.Fatalf("invalid clone: %#v", clone)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

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

func TestPluginRegistersAndroidDriveAndProvider(t *testing.T) {
	source := &fakeDeviceSource{}
	opener := &fakeDeviceOpener{}
	plugin := &Plugin{Source: source, Opener: opener}
	host := &recordingHost{}

	if err := plugin.Init(host); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if plugin.GetName() != "Android" {
		t.Fatalf("GetName = %q", plugin.GetName())
	}
	factory := host.drives["Android"]
	if factory == nil {
		t.Fatal("Android drive was not registered")
	}
	manager, ok := factory().(*ManagerVFS)
	if !ok || manager.source != source || manager.opener != opener {
		t.Fatalf("registered factory returned %#v", manager)
	}
	if len(host.providers) != 1 || host.providers[0].Name() != "Android-device" {
		t.Fatalf("providers = %#v", host.providers)
	}
	if err := plugin.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
