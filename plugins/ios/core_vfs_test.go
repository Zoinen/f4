package iosfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

type fakeCoreService struct {
	entries []coreEntry
	payload []byte
	pullErr error
	listErr error
	lists   int
	closes  int
}

func (s *fakeCoreService) List(context.Context, string) ([]coreEntry, error) {
	s.lists++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]coreEntry(nil), s.entries...), nil
}

func (s *fakeCoreService) Pull(_ context.Context, _ string, writer io.Writer) error {
	if s.pullErr != nil {
		return s.pullErr
	}
	_, err := writer.Write(s.payload)
	return err
}

func (s *fakeCoreService) Close() error {
	s.closes++
	return nil
}

type fakeCoreAccess struct {
	service coreFileService
	opens   int
}

func (a *fakeCoreAccess) Open(context.Context, DeviceInfo, coreDomain, string) (coreFileService, error) {
	a.opens++
	return a.service, nil
}

func (*fakeCoreAccess) Close() error { return nil }

func TestCleanIOSPathRejectsTraversalAndNUL(t *testing.T) {
	for _, candidate := range []string{"../private", "/safe/../private", "safe\x00name"} {
		if _, err := cleanIOSPath(candidate); err == nil {
			t.Fatalf("cleanIOSPath(%q) accepted an unsafe path", candidate)
		}
	}
	if got, err := cleanIOSPath("documents/report.txt"); err != nil || got != "/documents/report.txt" {
		t.Fatalf("clean path = %q, %v", got, err)
	}
}

func TestCoreVFSReadDirChunksAndFiltersUnsafeNames(t *testing.T) {
	entries := make([]coreEntry, 0, 260)
	for i := 0; i < 257; i++ {
		entries = append(entries, coreEntry{Name: strings.Repeat("x", i%4+1) + string(rune(0x100+i))})
	}
	entries = append(entries, coreEntry{Name: ".."}, coreEntry{Name: "nested/name"})
	fs := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppGroup, "group", "Group", &fakeCoreService{entries: entries})

	var chunks []int
	var total int
	err := fs.ReadDir(context.Background(), "/", func(items []vfs.VFSItem) {
		chunks = append(chunks, len(items))
		total += len(items)
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 257 || len(chunks) != 3 || chunks[0] != 128 || chunks[1] != 128 || chunks[2] != 1 {
		t.Fatalf("chunks = %v, total = %d", chunks, total)
	}
}

func TestCoreVFSKeepsDevicePanelInfoAndVirtualTitle(t *testing.T) {
	device := DeviceInfo{UDID: "phone", Name: "iPhone", Model: "iPhone 13", OSVersion: "17.4"}
	filesystem := newCoreVFS(nil, device, coreDomainAppData, "com.example.app",
		iosDeviceTitle(device, ApplicationsSelector, "Example"), &fakeCoreService{})
	if got := filesystem.PanelTitle("/Documents/cache"); got != "iPhone:/[Applications]/Example/Documents/cache" {
		t.Fatalf("PanelTitle = %q", got)
	}
	snapshot, fresh := filesystem.CachedPanelInfo(vfs.PanelInfoRequest{Path: "/Documents"})
	if !fresh || len(snapshot.Sections) != 1 {
		t.Fatalf("panel info = %#v, fresh %v", snapshot, fresh)
	}
	fields := make(map[string]string)
	for _, field := range snapshot.Sections[0].Fields {
		fields[field.ID] = field.Value
	}
	if fields["udid"] != device.UDID || fields["model"] != device.Model || fields["backend"] != "CoreDevice" {
		t.Fatalf("device fields = %#v", fields)
	}
}

func TestCoreVFSCloneHasIndependentPathAndSharedSession(t *testing.T) {
	fs := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app", "App", &fakeCoreService{})
	if err := fs.SetPathOptimistic("/first"); err != nil {
		t.Fatal(err)
	}
	clone := fs.Clone().(*CoreVFS)
	if err := clone.SetPathOptimistic("/second"); err != nil {
		t.Fatal(err)
	}
	if fs.GetPath() != "/first" || clone.GetPath() != "/second" {
		t.Fatalf("paths are not independent: original=%q clone=%q", fs.GetPath(), clone.GetPath())
	}
	if !vfs.SameSession(fs, clone) {
		t.Fatal("clone must retain the original session identity")
	}
}

func TestCoreVFSIndependentMountsHaveDistinctSessionKeys(t *testing.T) {
	first := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app.one", "One", &fakeCoreService{})
	second := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app.two", "Two", &fakeCoreService{})
	if vfs.SameSession(first, second) || first.SessionKey() == second.SessionKey() {
		t.Fatal("independent zero-allocation CoreDevice mounts share a session key")
	}
}

func TestCoreVFSClosesServiceAfterLastClone(t *testing.T) {
	service := &fakeCoreService{}
	filesystem := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app", "App", service)
	clone := filesystem.Clone().(*CoreVFS)
	if err := filesystem.Close(); err != nil {
		t.Fatal(err)
	}
	if service.closes != 0 {
		t.Fatalf("service closed with a live clone: %d", service.closes)
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	if service.closes != 1 {
		t.Fatalf("service close count = %d, want 1", service.closes)
	}
	if err := clone.Close(); err != nil || service.closes != 1 {
		t.Fatalf("second clone close = %v, count = %d", err, service.closes)
	}
}

func TestCoreVFSStatReusesRecentDirectoryMetadata(t *testing.T) {
	service := &fakeCoreService{entries: []coreEntry{{Name: "document.txt", Size: 9}}}
	filesystem := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app", "App", service)
	if err := filesystem.ReadDir(context.Background(), "/", nil); err != nil {
		t.Fatal(err)
	}
	item, err := filesystem.Stat(context.Background(), "/document.txt")
	if err != nil || item.Size != 9 {
		t.Fatalf("Stat = %#v, %v", item, err)
	}
	if service.lists != 1 {
		t.Fatalf("directory list calls = %d, want cached single call", service.lists)
	}
}

func TestCoreVFSReconnectReplacesSharedService(t *testing.T) {
	oldService := &fakeCoreService{listErr: ErrCoreDeviceConnection}
	newService := &fakeCoreService{entries: []coreEntry{{Name: "restored"}}}
	access := &fakeCoreAccess{service: newService}
	filesystem := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app", "App", oldService, access)
	clone := filesystem.Clone().(*CoreVFS)
	key := filesystem.SessionKey()
	if !filesystem.SessionLost(ErrCoreDeviceConnection) || !filesystem.CanReconnect() {
		t.Fatal("lost CoreDevice session is not reconnectable")
	}
	if err := clone.Reconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if access.opens != 1 || oldService.closes != 1 || filesystem.SessionKey() != key || clone.SessionKey() != key {
		t.Fatalf("reconnect state: opens=%d old closes=%d keys=%v/%v", access.opens, oldService.closes, filesystem.SessionKey(), clone.SessionKey())
	}
	var items []vfs.VFSItem
	if err := filesystem.ReadDir(context.Background(), "/", func(chunk []vfs.VFSItem) { items = append(items, chunk...) }); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "restored" {
		t.Fatalf("items after reconnect = %#v", items)
	}
	_ = filesystem.Close()
	_ = clone.Close()
	if newService.closes != 1 {
		t.Fatalf("replacement close count = %d", newService.closes)
	}
}

func TestCoreVFSOpenMaterializesOnceAndRemovesTempFile(t *testing.T) {
	payload := []byte("CoreDevice payload")
	fs := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainAppData, "app", "App", &fakeCoreService{payload: payload})
	handle, err := fs.Open(context.Background(), "/document.txt")
	if err != nil {
		t.Fatal(err)
	}
	tempPath := handle.(*coreTempFile).path
	buf := make([]byte, len(payload))
	if _, err := handle.ReadAt(context.Background(), buf, 0); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("payload = %q", buf)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file still exists: %v", err)
	}
}

func TestCoreVFSReadOnlyOperations(t *testing.T) {
	fs := newCoreVFS(nil, DeviceInfo{UDID: "udid"}, coreDomainCrashReports, "", "Crash Reports", &fakeCoreService{})
	if err := fs.MkDir(context.Background(), "/new"); !errors.Is(err, ErrReadOnlyDomain) {
		t.Fatalf("MkDir error = %v", err)
	}
	if _, err := fs.Create(context.Background(), "/new"); !errors.Is(err, ErrReadOnlyDomain) {
		t.Fatalf("Create error = %v", err)
	}
}
