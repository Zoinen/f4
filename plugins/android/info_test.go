package androidfs

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func deviceInfoFixture() []byte {
	return []byte(strings.Join([]string{
		deviceInfoPropMarker,
		"[ro.product.manufacturer]: [Samsung]",
		"[ro.product.model]: [SM-G930F]",
		"[ro.build.version.release]: [8.0.0]",
		"[ro.build.version.sdk]: [26]",
		"[ro.build.version.security_patch]: [2020-09-01]",
		"[ro.build.display.id]: [R16NW.G930FXXU8ETI2]",
		"[ro.product.cpu.abilist]: [arm64-v8a,armeabi-v7a]",
		deviceInfoDFMarker,
		"Filesystem 1K-blocks Used Available Use% Mounted on",
		"/dev/block/dm-0 100000 40000 60000 40% /storage/emulated",
		deviceInfoMemMarker,
		"MemTotal:        2000000 kB",
		"MemAvailable:     750000 kB",
		deviceInfoUpMarker,
		"90061.25 123.00",
		deviceInfoBatMarker,
		"AC powered: false",
		"USB powered: true",
		"level: 75",
		"temperature: 302",
		deviceInfoKernelMark,
		"Linux localhost 3.18.140 #1 SMP PREEMPT",
	}, "\n"))
}

func TestParseDeviceInfoOutput(t *testing.T) {
	facts, err := parseDeviceInfoOutput(deviceInfoFixture())
	if err != nil {
		t.Fatalf("parseDeviceInfoOutput: %v", err)
	}
	if facts.manufacturer != "Samsung" || facts.model != "SM-G930F" {
		t.Fatalf("product facts = manufacturer %q model %q", facts.manufacturer, facts.model)
	}
	if facts.android != "8.0.0" || facts.api != "26" || facts.securityPatch != "2020-09-01" {
		t.Fatalf("Android facts = version %q API %q patch %q", facts.android, facts.api, facts.securityPatch)
	}
	if facts.build != "R16NW.G930FXXU8ETI2" || facts.abis != "arm64-v8a,armeabi-v7a" {
		t.Fatalf("build facts = build %q ABI %q", facts.build, facts.abis)
	}
	if facts.memoryTotal != 2000000*1024 || facts.memoryAvail != 750000*1024 {
		t.Fatalf("memory = total %d available %d", facts.memoryTotal, facts.memoryAvail)
	}
	if facts.uptime != "1d 01h 01m" {
		t.Fatalf("uptime = %q", facts.uptime)
	}
	if facts.battery != "75% · USB · 30.2 °C" {
		t.Fatalf("battery = %q", facts.battery)
	}
	if facts.storageTotal != 100000*1024 || facts.storageAvail != 60000*1024 || facts.storageMount != "/storage/emulated" {
		t.Fatalf("storage = total %d available %d mount %q", facts.storageTotal, facts.storageAvail, facts.storageMount)
	}
	if facts.kernel != "Linux localhost 3.18.140 #1 SMP PREEMPT" {
		t.Fatalf("kernel = %q", facts.kernel)
	}
}

func panelField(snapshot vfs.PanelInfoSnapshot, id string) (vfs.PanelInfoField, bool) {
	for _, section := range snapshot.Sections {
		for _, field := range section.Fields {
			if field.ID == id {
				return field, true
			}
		}
	}
	return vfs.PanelInfoField{}, false
}

func TestDeviceInfoProviderCachesBySerialAndTTL(t *testing.T) {
	var calls atomic.Int32
	now := time.Unix(1700000000, 0)
	runErr := error(nil)
	service := &deviceInfoService{
		caches: make(map[string]*deviceInfoCache),
		run: func(_ context.Context, serial, command string, limit int64) (shellResult, error) {
			calls.Add(1)
			if serial != "9886" {
				t.Fatalf("serial = %q", serial)
			}
			for _, probe := range []string{"df -k '/sdcard'", "df -k '/storage/emulated/0'", "df -k '/data'"} {
				if !strings.Contains(command, probe) {
					t.Fatalf("probe command does not contain primary-storage fallback %q: %q", probe, command)
				}
			}
			if limit != deviceInfoMaxOutput {
				t.Fatalf("output limit = %d", limit)
			}
			if runErr != nil {
				return shellResult{}, runErr
			}
			return shellResult{Stdout: deviceInfoFixture()}, nil
		},
	}
	device := DeviceInfo{Serial: "9886", State: DeviceStateOnline, Model: "SM_G930F"}
	provider := service.provider(device, "FISH+", "list=find, read=dd, write=base64")
	provider.cache.mu.Lock()
	provider.cache.now = func() time.Time { return now }
	provider.cache.ttl = 10 * time.Second
	provider.cache.timeout = 0
	provider.cache.mu.Unlock()

	// The manager and a newly mounted device both start at `/`; that path must
	// still report primary user storage rather than Android's small root volume.
	req := vfs.PanelInfoRequest{Path: "/"}
	cached, fresh := provider.CachedPanelInfo(req)
	if fresh || !cached.Authoritative || calls.Load() != 0 {
		t.Fatalf("initial cache = fresh %v authoritative %v calls %d", fresh, cached.Authoritative, calls.Load())
	}
	if field, ok := panelField(cached, "backend"); !ok || field.Value != "FISH+" {
		t.Fatalf("initial backend = %#v, present %v", field, ok)
	}
	if len(cached.Sections) == 0 || len(cached.Sections[0].Fields) < 4 {
		t.Fatalf("initial device fields are incomplete: %#v", cached.Sections)
	}
	gotLeadingIDs := []string{
		cached.Sections[0].Fields[0].ID,
		cached.Sections[0].Fields[1].ID,
		cached.Sections[0].Fields[2].ID,
		cached.Sections[0].Fields[3].ID,
	}
	wantLeadingIDs := []string{"model", "serial", "backend", "protocol"}
	if !reflect.DeepEqual(gotLeadingIDs, wantLeadingIDs) {
		t.Fatalf("leading device fields = %v, want %v", gotLeadingIDs, wantLeadingIDs)
	}

	refreshed, err := provider.RefreshPanelInfo(context.Background(), req)
	if err != nil || calls.Load() != 1 {
		t.Fatalf("first refresh = %v, calls %d", err, calls.Load())
	}
	if field, ok := panelField(refreshed, "android"); !ok || field.Value != "8.0.0 (API 26)" {
		t.Fatalf("Android field = %#v, present %v", field, ok)
	}
	if _, ok := panelField(refreshed, "current_directory"); ok {
		t.Fatal("provider duplicated the core current-directory field")
	}
	if field, ok := panelField(refreshed, "storage"); !ok ||
		field.TotalBytes != 100000*1024 || field.AvailableBytes != 60000*1024 || field.Kind != vfs.PanelInfoUsage {
		t.Fatalf("storage field = %#v, present %v", field, ok)
	}
	if field, ok := panelField(refreshed, "memory"); !ok ||
		field.TotalBytes != 2000000*1024 || field.AvailableBytes != 750000*1024 || field.Kind != vfs.PanelInfoUsage {
		t.Fatalf("memory field = %#v, present %v", field, ok)
	}

	if _, err := provider.RefreshPanelInfo(context.Background(), req); err != nil || calls.Load() != 1 {
		t.Fatalf("fresh refresh = %v, calls %d", err, calls.Load())
	}
	if _, err := provider.RefreshPanelInfo(context.Background(), vfs.PanelInfoRequest{Path: "/data"}); err != nil || calls.Load() != 1 {
		t.Fatalf("path change should reuse primary-storage cache: %v, calls %d", err, calls.Load())
	}

	now = now.Add(11 * time.Second)
	runErr = errors.New("device disappeared")
	stale, err := provider.RefreshPanelInfo(context.Background(), vfs.PanelInfoRequest{Path: "/data"})
	if !errors.Is(err, runErr) || calls.Load() != 2 {
		t.Fatalf("failed refresh = %v, calls %d", err, calls.Load())
	}
	if field, ok := panelField(stale, "android"); !ok || field.Value != "8.0.0 (API 26)" {
		t.Fatalf("failed refresh discarded cached facts: %#v, present %v", field, ok)
	}
	if again := service.provider(device, "ADB Sync", "stat_v2"); again.cache != provider.cache {
		t.Fatal("providers for one serial did not share the device cache")
	}
}

func TestDeviceInfoProviderCoalescesConcurrentRefreshAndCancelsWaiter(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	service := &deviceInfoService{
		caches: make(map[string]*deviceInfoCache),
		run: func(ctx context.Context, _, _ string, _ int64) (shellResult, error) {
			calls.Add(1)
			close(started)
			select {
			case <-ctx.Done():
				return shellResult{}, ctx.Err()
			case <-release:
				return shellResult{Stdout: deviceInfoFixture()}, nil
			}
		},
	}
	provider := service.provider(DeviceInfo{Serial: "serial", Model: "phone"}, "ADB", "host transport")
	provider.cache.mu.Lock()
	provider.cache.timeout = 0
	provider.cache.mu.Unlock()
	req := vfs.PanelInfoRequest{Path: "/"}
	firstDone := make(chan error, 1)
	go func() {
		_, err := provider.RefreshPanelInfo(context.Background(), req)
		firstDone <- err
	}()
	<-started

	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.RefreshPanelInfo(waitCtx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("runner calls = %d, want one", calls.Load())
	}
}

func TestManagerPanelInfoTracksSelectedDeviceAndRefreshes(t *testing.T) {
	service := &deviceInfoService{
		caches: make(map[string]*deviceInfoCache),
		run: func(_ context.Context, _, _ string, _ int64) (shellResult, error) {
			return shellResult{Stdout: deviceInfoFixture()}, nil
		},
	}
	devices := []DeviceInfo{
		{Serial: "one", State: DeviceStateOnline, Model: "First"},
		{Serial: "two", State: DeviceStateOnline, Model: "Second"},
	}
	manager := newManagerVFS(&fakeDeviceSource{devices: devices}, nil, service)
	if _, err := readManagerItems(t, manager); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	firstReq := vfs.PanelInfoRequest{Path: androidRoot, SelectedName: DeviceDisplayName(devices[0])}
	secondReq := vfs.PanelInfoRequest{Path: androidRoot, SelectedName: DeviceDisplayName(devices[1])}
	if firstKey, secondKey := manager.PanelInfoKey(firstReq), manager.PanelInfoKey(secondReq); firstKey == secondKey || !strings.Contains(firstKey, "one") || !strings.Contains(secondKey, "two") {
		t.Fatalf("selection keys = %q and %q", firstKey, secondKey)
	}
	if _, fresh := manager.CachedPanelInfo(firstReq); fresh {
		t.Fatal("uncached selected device unexpectedly fresh")
	}
	snapshot, err := manager.RefreshPanelInfo(context.Background(), firstReq)
	if err != nil {
		t.Fatalf("RefreshPanelInfo: %v", err)
	}
	if field, ok := panelField(snapshot, "serial"); !ok || field.Value != "one" {
		t.Fatalf("selected serial = %#v, present %v", field, ok)
	}
	if _, fresh := manager.CachedPanelInfo(firstReq); !fresh {
		t.Fatal("refreshed selected device cache is stale")
	}
}

func TestDeviceInfoProviderInvalidatesReusedSerialIdentity(t *testing.T) {
	service := &deviceInfoService{
		caches: make(map[string]*deviceInfoCache),
		run: func(_ context.Context, _, _ string, _ int64) (shellResult, error) {
			return shellResult{Stdout: deviceInfoFixture()}, nil
		},
	}
	oldDevice := DeviceInfo{
		Serial: "emulator-5554", State: DeviceStateOnline, Model: "Old phone", TransportID: "1",
	}
	oldProvider := service.provider(oldDevice, "ADB", "host transport")
	oldProvider.cache.mu.Lock()
	oldProvider.cache.timeout = 0
	oldProvider.cache.mu.Unlock()
	if _, err := oldProvider.RefreshPanelInfo(context.Background(), vfs.PanelInfoRequest{Path: "/"}); err != nil {
		t.Fatal(err)
	}
	if _, fresh := oldProvider.CachedPanelInfo(vfs.PanelInfoRequest{Path: "/"}); !fresh {
		t.Fatal("old identity did not become fresh")
	}
	oldKey := oldProvider.PanelInfoKey(vfs.PanelInfoRequest{Path: "/"})

	newDevice := DeviceInfo{
		Serial: "emulator-5554", State: DeviceStateOnline, Model: "New phone", TransportID: "2",
	}
	newProvider := service.provider(newDevice, "ADB", "host transport")
	if newProvider.cache != oldProvider.cache {
		t.Fatal("serial cache object should be reused after identity invalidation")
	}
	if oldKey == newProvider.PanelInfoKey(vfs.PanelInfoRequest{Path: "/"}) {
		t.Fatal("connection identity change did not change the async generation key")
	}
	snapshot, fresh := newProvider.CachedPanelInfo(vfs.PanelInfoRequest{Path: "/"})
	if fresh {
		t.Fatal("reused serial retained a fresh snapshot from the previous connection")
	}
	if field, ok := panelField(snapshot, "model"); !ok || field.Value != "New phone" {
		t.Fatalf("new discovery model = %#v, present %v", field, ok)
	}
	if _, ok := panelField(snapshot, "android"); ok {
		t.Fatal("old getprop facts leaked into the reused serial baseline")
	}
}

func TestDeviceInfoProviderDiscardsInflightPreviousIdentity(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := &deviceInfoService{
		caches: make(map[string]*deviceInfoCache),
		run: func(_ context.Context, _, _ string, _ int64) (shellResult, error) {
			close(started)
			<-release
			return shellResult{Stdout: deviceInfoFixture()}, nil
		},
	}
	oldProvider := service.provider(DeviceInfo{
		Serial: "reused", State: DeviceStateOnline, Model: "Old", TransportID: "1",
	}, "ADB", "host transport")
	oldProvider.cache.mu.Lock()
	oldProvider.cache.timeout = 0
	oldProvider.cache.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := oldProvider.RefreshPanelInfo(context.Background(), vfs.PanelInfoRequest{Path: "/"})
		done <- err
	}()
	<-started
	newProvider := service.provider(DeviceInfo{
		Serial: "reused", State: DeviceStateOnline, Model: "New", TransportID: "2",
	}, "ADB", "host transport")
	close(release)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("old in-flight refresh error = %v", err)
	}
	snapshot, fresh := newProvider.CachedPanelInfo(vfs.PanelInfoRequest{Path: "/"})
	if fresh {
		t.Fatal("old in-flight refresh marked the new identity fresh")
	}
	if field, ok := panelField(snapshot, "model"); !ok || field.Value != "New" {
		t.Fatalf("new baseline model = %#v, present %v", field, ok)
	}
	if _, ok := panelField(snapshot, "android"); ok {
		t.Fatal("old in-flight facts were stored after identity change")
	}
}
