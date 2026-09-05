package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestIOSDeviceIntegration(t *testing.T) {
	udid := strings.TrimSpace(os.Getenv("F4_IOS_TEST_UDID"))
	if udid == "" {
		t.Skip("set F4_IOS_TEST_UDID to run the real-device test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	devices, err := (nativeDeviceSource{}).ListDevices(ctx)
	if err != nil {
		t.Fatalf("discover iOS devices: %v", err)
	}
	var selected DeviceInfo
	for _, device := range devices {
		if device.UDID == udid {
			selected = device
			break
		}
	}
	if selected.UDID == "" {
		t.Fatalf("device %q is not visible through usbmuxd", udid)
	}
	if !deviceReady(selected) {
		t.Fatalf("device %q is not ready: state=%q paired=%v", udid, selected.State, selected.Paired)
	}

	backend := &nativeBackend{apps: nativeAppSource{}, afc: newAFCRegistry(), core: newCoreAccess()}
	defer func() {
		if closeErr := backend.Close(); closeErr != nil {
			t.Errorf("close backend: %v", closeErr)
		}
	}()

	mounted, err := backend.openMedia(ctx, nil, selected)
	if err != nil {
		t.Fatalf("open Media AFC: %v", err)
	}
	media, ok := mounted.(*AFCVFS)
	if !ok {
		t.Fatalf("Media backend = %T, want *AFCVFS", mounted)
	}
	t.Cleanup(func() {
		if err := media.Close(); err != nil {
			t.Errorf("close Media filesystem: %v", err)
		}
	})

	repeated, err := backend.openMedia(ctx, nil, selected)
	if err != nil {
		t.Fatalf("open second Media view: %v", err)
	}
	repeatedSession, ok := repeated.(vfs.SessionIdentity)
	if !ok || repeatedSession.SessionKey() != media.SessionKey() {
		_ = repeated.Close()
		t.Fatal("second Media view did not reuse the AFC session")
	}
	if err := repeated.Close(); err != nil {
		t.Fatalf("close second Media view: %v", err)
	}

	simplified, err := backend.openDeviceRoot(ctx, nil, selected)
	if err != nil {
		t.Fatalf("open simplified device root: %v", err)
	}
	var rootItems []vfs.VFSItem
	if err := simplified.ReadDir(ctx, simplified.GetPath(), func(chunk []vfs.VFSItem) {
		rootItems = append(rootItems, chunk...)
	}); err != nil {
		_ = simplified.Close()
		t.Fatalf("list simplified device root: %v", err)
	}
	rootNames := make(map[string]bool, len(rootItems))
	for _, item := range rootItems {
		rootNames[item.Name] = item.IsDir
	}
	if !rootNames["DCIM"] || !rootNames[ApplicationsSelector] {
		_ = simplified.Close()
		t.Fatalf("simplified root entries = %#v, want real DCIM and virtual %s directories", rootNames, ApplicationsSelector)
	}
	if titleProvider, ok := simplified.(vfs.PanelTitleProvider); !ok || titleProvider.PanelTitle("/DCIM") != deviceLabel(selected)+":/DCIM" {
		_ = simplified.Close()
		t.Fatalf("simplified root title provider = %T", simplified)
	}
	if err := simplified.Close(); err != nil {
		t.Fatalf("close simplified device root: %v", err)
	}

	base := "/"
	if downloads, statErr := media.Stat(ctx, "/Downloads"); statErr == nil && downloads.IsDir {
		base = "/Downloads"
	}
	sandbox := path.Join(base, fmt.Sprintf(".f4-ios-test-%d-%d", os.Getpid(), time.Now().UnixNano()))
	cleaned := false
	defer func() {
		if cleaned {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if cleanupErr := media.Remove(cleanupCtx, sandbox); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			t.Logf("sandbox cleanup failed: %v", cleanupErr)
		}
	}()

	if err := media.MkDir(ctx, sandbox); err != nil {
		t.Fatalf("create sandbox %s: %v", sandbox, err)
	}
	source := path.Join(sandbox, "source file.txt")
	renamed := path.Join(sandbox, "renamed file.txt")
	payload := []byte("f4 iPhone integration\nsecond line\n")
	writer, err := media.Create(ctx, source)
	if err != nil {
		t.Fatalf("create test file: %v", err)
	}
	if n, writeErr := writer.Write(payload); writeErr != nil || n != len(payload) {
		_ = writer.Close()
		t.Fatalf("write test file: n=%d err=%v", n, writeErr)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test file: %v", err)
	}

	reader, err := media.Open(ctx, source)
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	if got := reader.Size(); got != int64(len(payload)) {
		_ = reader.Close()
		t.Fatalf("reader size = %d, want %d", got, len(payload))
	}
	buffer := make([]byte, len(payload)+7)
	n, readErr := reader.ReadAt(ctx, buffer, 0)
	if n != len(payload) || !errors.Is(readErr, io.EOF) || string(buffer[:n]) != string(payload) {
		_ = reader.Close()
		t.Fatalf("ReadAt = n=%d err=%v data=%q", n, readErr, buffer[:n])
	}
	prefix := make([]byte, 2)
	if n, readErr = reader.Read(ctx, prefix); readErr != nil || n != len(prefix) || string(prefix) != string(payload[:2]) {
		_ = reader.Close()
		t.Fatalf("sequential read after ReadAt = n=%d err=%v data=%q", n, readErr, prefix[:n])
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	wantMTime := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	if err := media.SetAttributes(ctx, source, vfs.VFSItem{MTime: wantMTime}); err != nil {
		t.Fatalf("set mtime: %v", err)
	}
	item, err := media.Stat(ctx, source)
	if err != nil {
		t.Fatalf("stat test file: %v", err)
	}
	if item.Size != int64(len(payload)) || item.IsDir {
		t.Fatalf("test file metadata = %#v", item)
	}
	if delta := item.MTime.Sub(wantMTime); delta < -time.Second || delta > time.Second {
		t.Fatalf("mtime = %v, want approximately %v", item.MTime, wantMTime)
	}

	if err := media.Rename(ctx, source, renamed); err != nil {
		t.Fatalf("rename test file: %v", err)
	}
	var entries []vfs.VFSItem
	if err := media.ReadDir(ctx, sandbox, func(chunk []vfs.VFSItem) {
		entries = append(entries, chunk...)
	}); err != nil {
		t.Fatalf("list sandbox: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != path.Base(renamed) || entries[0].Size != int64(len(payload)) {
		t.Fatalf("sandbox entries = %#v", entries)
	}

	if err := media.Remove(ctx, sandbox); err != nil {
		t.Fatalf("remove sandbox: %v", err)
	}
	cleaned = true
	if _, err := media.Stat(ctx, sandbox); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed sandbox stat error = %v, want not-exist", err)
	}

	apps, err := backend.apps.ListApps(ctx, selected)
	if err != nil {
		t.Fatalf("list user applications: %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("installation proxy returned no user applications")
	}
	for _, app := range apps {
		if strings.TrimSpace(app.BundleID) == "" {
			t.Fatalf("application without bundle ID: %#v", app)
		}
	}
	t.Logf("validated %s (%s, iOS %s) and listed %d user applications", selected.Name, selected.ProductType, selected.OSVersion, len(apps))

	testBundle := strings.TrimSpace(os.Getenv("F4_IOS_TEST_APP_BUNDLE"))
	if testBundle != "" {
		var selectedApp AppInfo
		for _, app := range apps {
			if app.BundleID == testBundle {
				selectedApp = app
				break
			}
		}
		if selectedApp.BundleID == "" {
			t.Fatalf("application %q is not installed", testBundle)
		}
		appVFS, err := backend.OpenApp(ctx, media, selected, selectedApp)
		if err != nil {
			t.Fatalf("open application %q: %v", testBundle, err)
		}
		t.Cleanup(func() {
			if err := appVFS.Close(); err != nil {
				t.Errorf("close application filesystem: %v", err)
			}
		})
		var appEntries []vfs.VFSItem
		if err := appVFS.ReadDir(ctx, appVFS.GetPath(), func(chunk []vfs.VFSItem) {
			appEntries = append(appEntries, chunk...)
		}); err != nil {
			t.Fatalf("list application %q root: %v", testBundle, err)
		}
		t.Logf("opened application %q through %T and listed %d root entries", testBundle, appVFS, len(appEntries))
	}

	coreBundle := strings.TrimSpace(os.Getenv("F4_IOS_TEST_CORE_BUNDLE"))
	coreCrash := os.Getenv("F4_IOS_TEST_CORE_CRASH") != ""
	if coreBundle == "" && !coreCrash {
		return
	}
	domain := coreDomainAppData
	identifier := coreBundle
	label := "application " + coreBundle
	if coreCrash {
		domain = coreDomainCrashReports
		identifier = ""
		label = "crash reports"
	} else {
		installed := false
		for _, app := range apps {
			if app.BundleID == coreBundle {
				installed = true
				break
			}
		}
		if !installed {
			t.Fatalf("CoreDevice application %q is not installed", coreBundle)
		}
	}
	service, err := backend.core.Open(ctx, selected, domain, identifier)
	if err != nil {
		t.Fatalf("open CoreDevice %s: %v", label, err)
	}
	coreVFS := newCoreVFS(media, selected, domain, identifier, label+" (CoreDevice)", service, backend.core)
	t.Cleanup(func() {
		if err := coreVFS.Close(); err != nil {
			t.Errorf("close CoreDevice filesystem: %v", err)
		}
	})
	corePath := strings.TrimSpace(os.Getenv("F4_IOS_TEST_CORE_PATH"))
	if corePath == "" {
		corePath = coreVFS.GetPath()
	}
	var coreEntries []vfs.VFSItem
	if err := coreVFS.ReadDir(ctx, corePath, func(chunk []vfs.VFSItem) {
		coreEntries = append(coreEntries, chunk...)
	}); err != nil {
		t.Fatalf("list CoreDevice %s path %q: %v", label, corePath, err)
	}
	t.Logf("opened CoreDevice %s and listed %d entries at %q", label, len(coreEntries), corePath)
}
