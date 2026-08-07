package iosfs

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func collectSelectorItems(t *testing.T, filesystem vfs.VFS) []vfs.VFSItem {
	t.Helper()
	var items []vfs.VFSItem
	if err := filesystem.ReadDir(context.Background(), filesystem.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	return items
}

func itemNames(items []vfs.VFSItem) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return names
}

func TestDeviceRootVFSRowsProviderScopingAndParent(t *testing.T) {
	manager := NewManagerVFS(nil, nil)
	device := DeviceInfo{UDID: "00008110.device", Name: "Alexander's iPhone", OSVersion: "17.4"}
	target := vfs.NewNullVFS(0)
	var opened Capability
	var openedParent vfs.VFS
	opener := SelectorOpenerFunc(func(_ context.Context, parent vfs.VFS, got DeviceInfo, capability Capability) (vfs.VFS, error) {
		openedParent = parent
		opened = capability
		if got != device {
			t.Fatalf("device = %#v, want %#v", got, device)
		}
		return target, nil
	})
	root := NewDeviceRootVFS(manager, device, opener)

	items := collectSelectorItems(t, root)
	want := []string{MediaSelector, ApplicationsSelector, AppGroupsSelector, CrashReportsSelector}
	if got := itemNames(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	for _, item := range items {
		if !item.IsDir || !item.IsExecutable || !item.NoExtension {
			t.Fatalf("selector row = %#v, want openable directory", item)
		}
	}
	if root.ParentVFS() != manager {
		t.Fatal("device selector lost manager parent")
	}
	if got := root.PanelTitle("/"); got != "Alexander's iPhone:/" {
		t.Fatalf("PanelTitle = %q", got)
	}

	provider := &SelectorProvider{}
	if !provider.CanOpen(context.Background(), root, "/"+ApplicationsSelector) {
		t.Fatal("provider rejected exact capability row")
	}
	if provider.CanOpen(context.Background(), root, "/nested/"+ApplicationsSelector) {
		t.Fatal("provider accepted nested path by base name")
	}
	if provider.CanOpen(context.Background(), vfs.NewNullVFS(0), ApplicationsSelector) {
		t.Fatal("provider accepted foreign VFS")
	}
	got, err := provider.Open(context.Background(), root, ApplicationsSelector)
	if err != nil || got != target || opened != CapabilityApplications || openedParent != root {
		t.Fatalf("Open = %T, %v; capability=%v parent=%T", got, err, opened, openedParent)
	}
}

func TestIOSVirtualTitlesAndDeviceInfoSurviveSelectorTransitions(t *testing.T) {
	device := DeviceInfo{
		UDID: "00008110", Name: "Alexander's iPhone", Model: "iPhone 13",
		OSVersion: "26.5.2", State: DeviceStateReady, Paired: true,
	}
	root := NewDeviceRootVFS(nil, device, nil)
	apps := NewApplicationsVFS(root, device, nil, nil)
	groups := NewAppGroupsVFS(root, device, nil, nil)

	if got := root.PanelTitle("/"); got != "Alexander's iPhone:/" {
		t.Fatalf("device title = %q", got)
	}
	if got := apps.PanelTitle("/"); got != "Alexander's iPhone:/[Applications]/" {
		t.Fatalf("applications title = %q", got)
	}
	if got := groups.PanelTitle("/"); got != "Alexander's iPhone:/[App Groups]/" {
		t.Fatalf("app-groups title = %q", got)
	}
	if got := iosPanelTitle(iosDeviceTitle(device, MediaSelector), "/DCIM/100APPLE"); got != "Alexander's iPhone:/Media/DCIM/100APPLE" {
		t.Fatalf("media path title = %q", got)
	}

	for name, provider := range map[string]vfs.PanelInfoProvider{
		"device": root, "applications": apps, "app groups": groups,
	} {
		snapshot, fresh := provider.CachedPanelInfo(vfs.PanelInfoRequest{Path: "/"})
		if !fresh || !snapshot.Authoritative || len(snapshot.Sections) != 1 {
			t.Fatalf("%s panel info = %#v, fresh %v", name, snapshot, fresh)
		}
		fields := make(map[string]string)
		for _, field := range snapshot.Sections[0].Fields {
			fields[field.ID] = field.Value
		}
		if fields["udid"] != device.UDID || fields["model"] != device.Model || fields["ios"] != device.OSVersion {
			t.Fatalf("%s lost device fields: %#v", name, fields)
		}
	}
}

func TestAFCMediaRootExposesOnlyAvailableBracketSelectors(t *testing.T) {
	device := DeviceInfo{UDID: "phone", Name: "iPhone"}
	target := vfs.NewNullVFS(0)
	var opened Capability
	opener := SelectorOpenerFunc(func(_ context.Context, parent vfs.VFS, got DeviceInfo, capability Capability) (vfs.VFS, error) {
		if got != device {
			t.Fatalf("device = %#v, want %#v", got, device)
		}
		if _, ok := parent.(*AFCVFS); !ok {
			t.Fatalf("parent = %T, want AFC Media root", parent)
		}
		opened = capability
		return target, nil
	})
	media := &AFCVFS{device: device, path: "/"}
	media.SetVirtualRoot(opener, CapabilityApplications)

	items := media.virtualRootItems()
	if got, want := itemNames(items), []string{ApplicationsSelector}; !reflect.DeepEqual(got, want) {
		t.Fatalf("virtual root rows = %q, want %q", got, want)
	}
	if !items[0].IsDir || !items[0].NoExtension || strings.Contains(items[0].Name, "/") {
		t.Fatalf("virtual Applications row = %#v", items[0])
	}
	if _, err := media.Stat(context.Background(), "/"+ApplicationsSelector); err != nil {
		t.Fatalf("Stat(Applications): %v", err)
	}
	if err := media.Remove(context.Background(), "/"+ApplicationsSelector); !errors.Is(err, ErrSelectorReadOnly) {
		t.Fatalf("Remove(Applications) = %v, want read-only selector", err)
	}
	if _, ok := media.rootCapabilityForPath("/" + CrashReportsSelector); ok {
		t.Fatal("unavailable Crash Reports selector was exposed")
	}

	provider := &SelectorProvider{}
	if !provider.CanOpen(context.Background(), media, "/"+ApplicationsSelector) {
		t.Fatal("provider rejected bracket Applications directory in AFC root")
	}
	got, err := provider.Open(context.Background(), media, ApplicationsSelector)
	if err != nil || got != target || opened != CapabilityApplications {
		t.Fatalf("Open = %T, %v; capability=%v", got, err, opened)
	}
}

func TestDeviceRootDisablesAppGroupsWithoutCoreDevice(t *testing.T) {
	root := NewDeviceRootVFS(nil, DeviceInfo{UDID: "old-phone", OSVersion: "16.7"}, SelectorOpenerFunc(func(context.Context, vfs.VFS, DeviceInfo, Capability) (vfs.VFS, error) {
		return vfs.NewNullVFS(0), nil
	}))
	items := collectSelectorItems(t, root)
	for _, item := range items {
		if item.Name == AppGroupsSelector {
			t.Fatal("unavailable App Groups row is visible below iOS 17.4")
		}
	}
	if _, err := root.Stat(context.Background(), AppGroupsSelector); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(App Groups) = %v, want not-exist", err)
	}
	if (&SelectorProvider{}).CanOpen(context.Background(), root, AppGroupsSelector) {
		t.Fatal("provider accepted unavailable App Groups row")
	}
}

func TestDeviceRootDisablesAppGroupsOnIOS265(t *testing.T) {
	root := NewDeviceRootVFS(nil, DeviceInfo{UDID: "affected-phone", OSVersion: "26.5.2"}, SelectorOpenerFunc(func(context.Context, vfs.VFS, DeviceInfo, Capability) (vfs.VFS, error) {
		return vfs.NewNullVFS(0), nil
	}))
	for _, item := range collectSelectorItems(t, root) {
		if item.Name == AppGroupsSelector {
			t.Fatal("App Groups is visible on affected iOS 26.5.x")
		}
	}
	if _, err := root.Stat(context.Background(), AppGroupsSelector); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(App Groups) = %v, want not-exist", err)
	}
	if (&SelectorProvider{}).CanOpen(context.Background(), root, AppGroupsSelector) {
		t.Fatal("provider accepted App Groups on affected iOS 26.5.x")
	}
}

func TestApplicationsVFSDisambiguatesNamesAndKeepsDotsWhole(t *testing.T) {
	device := DeviceInfo{UDID: "phone.1", Name: "Phone"}
	apps := []AppInfo{
		{BundleID: "com.example.beta", DisplayName: "Chat"},
		{BundleID: "com.example.alpha", DisplayName: "Chat"},
		{BundleID: "org.example.editor", DisplayName: "Text.Editor"},
		{BundleID: "com.example.bare"},
		{DisplayName: "missing bundle"},
	}
	var opened AppInfo
	var openedParent vfs.VFS
	target := vfs.NewNullVFS(0)
	selector := NewApplicationsVFS(
		vfs.NewNullVFS(0),
		device,
		AppSourceFunc(func(context.Context, DeviceInfo) ([]AppInfo, error) { return apps, nil }),
		AppOpenerFunc(func(_ context.Context, parent vfs.VFS, _ DeviceInfo, app AppInfo) (vfs.VFS, error) {
			openedParent, opened = parent, app
			return target, nil
		}),
	)

	items := collectSelectorItems(t, selector)
	want := []string{
		"Chat (com.example.alpha)",
		"Chat (com.example.beta)",
		"Text.Editor (org.example.editor)",
		"com.example.bare",
	}
	if got := itemNames(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("application rows = %q, want %q", got, want)
	}
	for _, item := range items {
		if !item.IsDir || !item.NoExtension || !item.IsExecutable {
			t.Fatalf("application row is not an openable directory: %#v", item)
		}
	}

	provider := &ApplicationProvider{}
	row := "Chat (com.example.beta)"
	if !provider.CanOpen(context.Background(), selector, "/"+row) {
		t.Fatal("provider rejected exact app row")
	}
	if provider.CanOpen(context.Background(), selector, "/other/"+row) {
		t.Fatal("provider accepted app under nested path")
	}
	got, err := provider.Open(context.Background(), selector, row)
	if err != nil || got != target || opened.BundleID != "com.example.beta" || openedParent != selector {
		t.Fatalf("Open = %T, %v; app=%#v parent=%T", got, err, opened, openedParent)
	}

	clone, ok := selector.Clone().(*ApplicationsVFS)
	if !ok || clone == selector || clone.ParentVFS() != selector.ParentVFS() {
		t.Fatalf("clone = %#v", clone)
	}
	if !(&ApplicationProvider{}).CanOpen(context.Background(), clone, row) {
		t.Fatal("clone did not preserve completed exact-row snapshot")
	}
}

func TestApplicationAndGroupRowsEscapePathSeparators(t *testing.T) {
	app := AppInfo{BundleID: "com.example.app", DisplayName: "Files/Shared"}
	row := AppDisplayName(app)
	if strings.Contains(row, "/") {
		t.Fatalf("application row contains path separator: %q", row)
	}
	apps := NewApplicationsVFS(nil, DeviceInfo{UDID: "phone"}, AppSourceFunc(func(context.Context, DeviceInfo) ([]AppInfo, error) {
		return []AppInfo{app}, nil
	}), AppOpenerFunc(func(context.Context, vfs.VFS, DeviceInfo, AppInfo) (vfs.VFS, error) {
		return vfs.NewNullVFS(0), nil
	}))
	collectSelectorItems(t, apps)
	if !(&ApplicationProvider{}).CanOpen(context.Background(), apps, row) {
		t.Fatalf("escaped application row %q is not openable", row)
	}

	var openedGroup string
	groups := NewAppGroupsVFS(nil, DeviceInfo{UDID: "phone"}, []string{"group.example/shared"}, GroupOpenerFunc(func(_ context.Context, _ vfs.VFS, _ DeviceInfo, groupID string) (vfs.VFS, error) {
		openedGroup = groupID
		return vfs.NewNullVFS(0), nil
	}))
	items := collectSelectorItems(t, groups)
	if len(items) != 1 || strings.Contains(items[0].Name, "/") || !(&GroupProvider{}).CanOpen(context.Background(), groups, items[0].Name) {
		t.Fatalf("escaped group row is invalid: %#v", items)
	}
	if _, err := (&GroupProvider{}).Open(context.Background(), groups, items[0].Name); err != nil || openedGroup != "group.example/shared" {
		t.Fatalf("escaped group opened %q with error %v", openedGroup, err)
	}
}

func TestApplicationsVFSCanceledRefreshPreservesSnapshot(t *testing.T) {
	device := DeviceInfo{UDID: "phone"}
	first := true
	selector := NewApplicationsVFS(nil, device, AppSourceFunc(func(ctx context.Context, _ DeviceInfo) ([]AppInfo, error) {
		if first {
			first = false
			return []AppInfo{{BundleID: "com.example.app", DisplayName: "App"}}, nil
		}
		return nil, ctx.Err()
	}), AppOpenerFunc(func(context.Context, vfs.VFS, DeviceInfo, AppInfo) (vfs.VFS, error) {
		return vfs.NewNullVFS(0), nil
	}))
	collectSelectorItems(t, selector)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := selector.ReadDir(ctx, "/", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadDir = %v, want canceled", err)
	}
	if !(&ApplicationProvider{}).CanOpen(context.Background(), selector, "App (com.example.app)") {
		t.Fatal("canceled refresh discarded the last completed snapshot")
	}
}

func TestAppGroupsVFSDeduplicatesSortsAndScopesProvider(t *testing.T) {
	deviceRoot := NewDeviceRootVFS(nil, DeviceInfo{UDID: "phone", Name: "Phone"}, nil)
	device := DeviceInfo{UDID: "phone", Name: "Phone"}
	target := vfs.NewNullVFS(0)
	var opened string
	groups := NewAppGroupsVFS(deviceRoot, device, []string{
		" group.com.example.shared ",
		"group.com.example.alpha",
		"group.com.example.shared",
		"",
	}, GroupOpenerFunc(func(_ context.Context, parent vfs.VFS, _ DeviceInfo, groupID string) (vfs.VFS, error) {
		if parent == nil {
			t.Fatal("group opener did not receive selector parent")
		}
		opened = groupID
		return target, nil
	}))

	items := collectSelectorItems(t, groups)
	want := []string{"group.com.example.alpha", "group.com.example.shared"}
	if got := itemNames(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("group rows = %q, want %q", got, want)
	}
	for _, item := range items {
		if !item.IsDir || !item.NoExtension || !item.IsExecutable {
			t.Fatalf("group row = %#v, want openable directory", item)
		}
	}
	if groups.ParentVFS() != deviceRoot || deviceRoot.ParentVFS() != nil {
		t.Fatal("app-group parent chain is broken")
	}

	provider := &GroupProvider{}
	row := "group.com.example.shared"
	if !provider.CanOpen(context.Background(), groups, row) || provider.CanOpen(context.Background(), groups, "nested/"+row) {
		t.Fatal("group provider exact-row scoping failed")
	}
	got, err := provider.Open(context.Background(), groups, row)
	if err != nil || got != target || opened != row {
		t.Fatalf("Open = %T, %v; group=%q", got, err, opened)
	}
}

func TestSelectorsAreReadOnly(t *testing.T) {
	selectors := []vfs.VFS{
		NewDeviceRootVFS(nil, DeviceInfo{UDID: "phone"}, nil),
		NewApplicationsVFS(nil, DeviceInfo{UDID: "phone"}, nil, nil),
		NewAppGroupsVFS(nil, DeviceInfo{UDID: "phone"}, nil, nil),
	}
	ctx := context.Background()
	for _, selector := range selectors {
		if err := selector.MkDir(ctx, "x"); !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.MkDir = %v", selector, err)
		}
		if err := selector.Remove(ctx, "x"); !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.Remove = %v", selector, err)
		}
		if err := selector.Rename(ctx, "x", "y"); !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.Rename = %v", selector, err)
		}
		if err := selector.SetAttributes(ctx, "x", vfs.VFSItem{}); !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.SetAttributes = %v", selector, err)
		}
		if _, err := selector.Search(ctx, "/", "x"); !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.Search = %v", selector, err)
		}
		if _, err := selector.Open(ctx, "x"); !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.Open = %v", selector, err)
		}
		var writer io.WriteCloser
		var err error
		if writer, err = selector.Create(ctx, "x"); writer != nil || !errors.Is(err, ErrSelectorReadOnly) {
			t.Errorf("%T.Create = %T, %v", selector, writer, err)
		}
		if got := selector.GetCapabilities(); got != (vfs.VFSCapabilities{}) {
			t.Errorf("%T capabilities = %#v", selector, got)
		}
	}
}
