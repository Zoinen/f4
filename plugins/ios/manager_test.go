package iosfs

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
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

func TestManagerReadDirRefreshesAndLabelsAllDeviceStates(t *testing.T) {
	source := &fakeDeviceSource{devices: []DeviceInfo{
		{UDID: "0003", Name: "Work iPhone", State: DeviceStateReady, Paired: true, ConnectionType: "Network"},
		{UDID: "0001", Name: "Alexander's iPhone", State: DeviceStateReady, Paired: true, ConnectionType: "USB"},
		{UDID: "0002", Model: "iPad Pro", State: DeviceStateLocked, Paired: true},
		{UDID: "0004", ProductType: "iPhone15,2", State: DeviceStateReady, Paired: false},
		{Name: "missing UDID", State: DeviceStateReady, Paired: true},
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
			t.Errorf("device row %q may be split as a filename extension", item.Name)
		}
	}
	wantNames := []string{
		"Alexander's iPhone",
		"Work iPhone",
		"iPad Pro [locked]",
		"iPhone15,2 [unpaired]",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("names = %#v, want %#v", gotNames, wantNames)
	}
	if !reflect.DeepEqual(gotExecutable, []bool{true, true, false, false}) {
		t.Fatalf("executable flags = %#v", gotExecutable)
	}

	// Every panel refresh performs discovery and replaces stale rows.
	source.devices = []DeviceInfo{{UDID: "new", Name: "New iPhone", State: DeviceStateReady, Paired: true}}
	items, err = readManagerItems(t, manager)
	if err != nil {
		t.Fatalf("second ReadDir: %v", err)
	}
	if source.calls != 2 || len(items) != 1 || items[0].Name != "New iPhone" {
		t.Fatalf("refresh calls/items = %d, %#v", source.calls, items)
	}
	if _, err := manager.Stat(context.Background(), "Alexander's iPhone"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale device Stat error = %v, want os.ErrNotExist", err)
	}
}

func TestManagerDisambiguatesEqualVisibleNamesWithoutShowingUDID(t *testing.T) {
	source := &fakeDeviceSource{devices: []DeviceInfo{
		{UDID: "0002", Name: "iPhone", State: DeviceStateReady, Paired: true},
		{UDID: "0001", Name: "iPhone", State: DeviceStateReady, Paired: true},
	}}
	manager := NewManagerVFS(source, &fakeDeviceOpener{})
	items, err := readManagerItems(t, manager)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("duplicate rows = %#v, want 2", items)
	}
	if got, want := []string{items[0].Name, items[1].Name}, []string{"iPhone", "iPhone (2)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate labels = %q, want %q", got, want)
	}
	first, firstOK := manager.deviceForPath("iPhone")
	second, secondOK := manager.deviceForPath("iPhone (2)")
	if !firstOK || !secondOK || first.UDID != "0001" || second.UDID != "0002" {
		t.Fatalf("duplicate identities = %#v/%v %#v/%v", first, firstOK, second, secondOK)
	}
	secondItem, err := manager.Stat(context.Background(), "iPhone (2)")
	if err != nil || secondItem.Name != "iPhone (2)" {
		t.Fatalf("second duplicate Stat = %#v, %v", secondItem, err)
	}
	if !secondItem.IsDir {
		t.Fatalf("second duplicate Stat item is not a directory: %#v", secondItem)
	}
}

func TestManagerDeduplicatesUDIDAndPrefersReadyUSBDevice(t *testing.T) {
	source := &fakeDeviceSource{devices: []DeviceInfo{
		{UDID: "same", Name: "Phone", State: DeviceStateLocked, Paired: true, ConnectionType: "USB"},
		{UDID: "same", Name: "Phone", State: DeviceStateReady, Paired: true, ConnectionType: "Network"},
		{UDID: "same", Name: "Phone", State: DeviceStateReady, Paired: true, ConnectionType: "USB"},
	}}
	manager := NewManagerVFS(source, &fakeDeviceOpener{})
	items, err := readManagerItems(t, manager)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].IsExecutable {
		t.Fatalf("items = %#v, want one ready row", items)
	}
	device, ok := manager.deviceForPath(items[0].Name)
	if !ok || device.ConnectionType != "USB" {
		t.Fatalf("selected duplicate = %#v, present %v", device, ok)
	}
}

func TestManagerDiscoveryFailureAndCancellationSnapshots(t *testing.T) {
	device := DeviceInfo{UDID: "udid", Name: "Phone", State: DeviceStateReady, Paired: true}
	source := &fakeDeviceSource{devices: []DeviceInfo{device}}
	manager := NewManagerVFS(source, &fakeDeviceOpener{})
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatalf("initial ReadDir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source.err = context.Canceled
	if err := manager.ReadDir(ctx, manager.GetPath(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ReadDir error = %v", err)
	}
	provider := &deviceProvider{}
	if !provider.CanOpen(context.Background(), manager, DeviceDisplayName(device)) {
		t.Fatal("canceled refresh discarded last successful snapshot")
	}

	source.err = errors.New("usbmuxd unavailable")
	if err := manager.ReadDir(context.Background(), manager.GetPath(), nil); !errors.Is(err, source.err) {
		t.Fatalf("failed ReadDir error = %v", err)
	}
	if provider.CanOpen(context.Background(), manager, DeviceDisplayName(device)) {
		t.Fatal("discovery failure left a stale row openable")
	}
}

func TestDeviceProviderIsNarrowAndOpensOnlyReadyPairedDevice(t *testing.T) {
	ready := DeviceInfo{UDID: "ready", Name: "Phone", State: DeviceStateReady, Paired: true}
	locked := DeviceInfo{UDID: "locked", Name: "Phone", State: DeviceStateLocked, Paired: true}
	unpaired := DeviceInfo{UDID: "unpaired", Name: "Phone", State: DeviceStateReady, Paired: false}
	source := &fakeDeviceSource{devices: []DeviceInfo{ready, locked, unpaired}}
	target := vfs.NewNullVFS(0)
	opener := &fakeDeviceOpener{result: target}
	manager := NewManagerVFS(source, opener)
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatal(err)
	}
	provider := &deviceProvider{}
	ctx := context.Background()

	if provider.CanOpen(ctx, vfs.NewNullVFS(0), DeviceDisplayName(ready)) {
		t.Fatal("provider accepted a row outside ManagerVFS")
	}
	if !provider.CanOpen(ctx, manager, "ios://"+DeviceDisplayName(ready)) {
		t.Fatal("provider rejected ready manager row")
	}
	if provider.CanOpen(ctx, manager, "nested/"+DeviceDisplayName(ready)) ||
		provider.CanOpen(ctx, manager, "ios://nested/"+DeviceDisplayName(ready)) {
		t.Fatal("provider accepted a nested path by device basename")
	}
	if provider.CanOpen(ctx, manager, DeviceDisplayName(locked)) || provider.CanOpen(ctx, manager, DeviceDisplayName(unpaired)) {
		t.Fatal("provider accepted unavailable device")
	}

	opened, err := provider.Open(ctx, manager, DeviceDisplayName(ready))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened != target || opener.calls != 1 || opener.parent != manager || opener.device != ready {
		t.Fatalf("opener delegation mismatch: opened=%T calls=%d parent=%T device=%#v", opened, opener.calls, opener.parent, opener.device)
	}
	if _, err := provider.Open(ctx, manager, DeviceDisplayName(locked)); !errors.Is(err, ErrDeviceUnavailable) {
		t.Fatalf("locked Open error = %v, want ErrDeviceUnavailable", err)
	}
	if _, err := provider.Open(ctx, vfs.NewNullVFS(0), "anything"); err == nil {
		t.Fatal("Open accepted non-manager parent")
	}
}

func TestDeviceDisplayNameEscapesPathSeparators(t *testing.T) {
	device := DeviceInfo{UDID: "serial/with/slash", Name: "Alice/Bob", State: DeviceStateReady, Paired: true}
	row := DeviceDisplayName(device)
	if strings.Contains(row, "/") {
		t.Fatalf("device row contains path separator: %q", row)
	}
	manager := NewManagerVFS(DeviceSourceFunc(func(context.Context) ([]DeviceInfo, error) {
		return []DeviceInfo{device}, nil
	}), DeviceOpenerFunc(func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) {
		return vfs.NewNullVFS(0), nil
	}))
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatal(err)
	}
	if !(&deviceProvider{}).CanOpen(context.Background(), manager, row) {
		t.Fatalf("escaped device row %q is not openable", row)
	}
}

func TestManagerPanelInfoUsesSelectedRow(t *testing.T) {
	device := DeviceInfo{
		UDID: "00008110", Name: "Alexander's iPhone", Model: "iPhone 13", ProductType: "iPhone14,5",
		OSVersion: "26.5.2", BuildVersion: "23F101", ConnectionType: "USB", State: DeviceStateReady,
		Paired: true, DeveloperMode: true,
	}
	manager := NewManagerVFS(&fakeDeviceSource{devices: []DeviceInfo{device}}, &fakeDeviceOpener{})
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatal(err)
	}
	req := vfs.PanelInfoRequest{Path: iosRoot, SelectedName: DeviceDisplayName(device)}
	if got := manager.PanelInfoKey(req); got != "ios-manager:"+device.UDID {
		t.Fatalf("PanelInfoKey = %q", got)
	}
	snapshot, fresh := manager.CachedPanelInfo(req)
	if !fresh || !snapshot.Authoritative || len(snapshot.Sections) != 1 {
		t.Fatalf("snapshot = %#v, fresh %v", snapshot, fresh)
	}
	fields := make(map[string]string)
	for _, field := range snapshot.Sections[0].Fields {
		fields[field.ID] = field.Value
	}
	for id, want := range map[string]string{
		"name": "Alexander's iPhone", "model": "iPhone 13", "product_type": "iPhone14,5",
		"udid": "00008110", "ios": "26.5.2", "build": "23F101", "connection": "USB",
		"state": "ready", "paired": "Yes", "developer_mode": "Yes",
	} {
		if got := fields[id]; got != want {
			t.Errorf("field %s = %q, want %q", id, got, want)
		}
	}
}

func TestManagerVFSContract(t *testing.T) {
	source := &fakeDeviceSource{}
	opener := &fakeDeviceOpener{}
	manager := NewManagerVFS(source, opener)

	if !manager.IsAtRoot() || manager.GetPath() != iosRoot || manager.GetTitle() != "iOS" {
		t.Fatalf("root identity = %v %q %q", manager.IsAtRoot(), manager.GetPath(), manager.GetTitle())
	}
	if got := manager.PanelTitle(manager.GetPath()); got != "Apple mobile devices" {
		t.Fatalf("PanelTitle = %q", got)
	}
	if manager.ParentVFS() != nil {
		t.Fatal("manager unexpectedly has a parent")
	}
	if got := manager.Join(iosRoot, "Phone (udid)"); got != "ios://Phone (udid)" {
		t.Fatalf("Join = %q", got)
	}
	if got, err := manager.Abs("Phone (udid)"); err != nil || got != "ios://Phone (udid)" {
		t.Fatalf("Abs = %q, %v", got, err)
	}
	if got := manager.Base("ios://Phone (udid)"); got != "Phone (udid)" {
		t.Fatalf("Base = %q", got)
	}
	if err := manager.SetPath(iosRoot); err != nil {
		t.Fatalf("SetPath(root): %v", err)
	}
	if err := manager.SetPath("ios://child"); err == nil {
		t.Fatal("SetPath accepted a child")
	}
	if got := manager.GetCapabilities(); got != (vfs.VFSCapabilities{}) {
		t.Fatalf("capabilities = %#v", got)
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

func TestDeviceProviderOpensVirtualDirectories(t *testing.T) {
	provider := &deviceProvider{}
	if !provider.OpensVirtualDirectories() {
		t.Fatal("device provider does not open directory-rendered device rows")
	}
}
