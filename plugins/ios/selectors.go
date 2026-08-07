package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

const (
	MediaSelector        = "Media"
	ApplicationsSelector = "[Applications]"
	AppGroupsSelector    = "[App Groups]"
	CrashReportsSelector = "[Crash Reports]"
)

var (
	ErrSelectorReadOnly = errors.New("ios: selector is read-only")
	ErrNoSelectorOpener = errors.New("ios: capability opener is not configured")
	ErrNoAppSource      = errors.New("ios: application source is not configured")
	ErrNoAppOpener      = errors.New("ios: application opener is not configured")
	ErrNoGroupOpener    = errors.New("ios: app group opener is not configured")
)

// Capability identifies one of the stable, transport-independent sections
// exposed below a connected iOS device.
type Capability uint8

const (
	CapabilityMedia Capability = iota + 1
	CapabilityApplications
	CapabilityAppGroups
	CapabilityCrashReports
)

// SelectorOpener mounts the selected capability. It is deliberately separate
// from device discovery and protocol negotiation: the selector only owns the
// virtual hierarchy and exact row-to-capability mapping.
type SelectorOpener interface {
	OpenSelection(context.Context, vfs.VFS, DeviceInfo, Capability) (vfs.VFS, error)
}

type SelectorOpenerFunc func(context.Context, vfs.VFS, DeviceInfo, Capability) (vfs.VFS, error)

func (f SelectorOpenerFunc) OpenSelection(ctx context.Context, parent vfs.VFS, device DeviceInfo, capability Capability) (vfs.VFS, error) {
	return f(ctx, parent, device, capability)
}

// AppInfo is the portion of installation-proxy metadata that is safe and
// useful to expose through the transport-independent application selector.
type AppInfo struct {
	BundleID        string
	DisplayName     string
	FileSharing     bool
	DeveloperSigned bool
	GroupIDs        []string
}

type AppSource interface {
	ListApps(context.Context, DeviceInfo) ([]AppInfo, error)
}

type AppSourceFunc func(context.Context, DeviceInfo) ([]AppInfo, error)

func (f AppSourceFunc) ListApps(ctx context.Context, device DeviceInfo) ([]AppInfo, error) {
	return f(ctx, device)
}

type AppOpener interface {
	OpenApp(context.Context, vfs.VFS, DeviceInfo, AppInfo) (vfs.VFS, error)
}

type AppOpenerFunc func(context.Context, vfs.VFS, DeviceInfo, AppInfo) (vfs.VFS, error)

func (f AppOpenerFunc) OpenApp(ctx context.Context, parent vfs.VFS, device DeviceInfo, app AppInfo) (vfs.VFS, error) {
	return f(ctx, parent, device, app)
}

type GroupOpener interface {
	OpenGroup(context.Context, vfs.VFS, DeviceInfo, string) (vfs.VFS, error)
}

type GroupOpenerFunc func(context.Context, vfs.VFS, DeviceInfo, string) (vfs.VFS, error)

func (f GroupOpenerFunc) OpenGroup(ctx context.Context, parent vfs.VFS, device DeviceInfo, groupID string) (vfs.VFS, error) {
	return f(ctx, parent, device, groupID)
}

type selectorBase struct {
	parent     vfs.VFS
	title      string
	panelTitle string
	panelInfo  vfs.PanelInfoProvider
}

func newSelectorBase(parent vfs.VFS, device DeviceInfo, title, panelTitle, backend string) selectorBase {
	return selectorBase{
		parent: parent, title: title, panelTitle: panelTitle,
		panelInfo: newDevicePanelInfoProvider(device, backend),
	}
}

func (s *selectorBase) IsAtRoot() bool  { return true }
func (s *selectorBase) GetPath() string { return "/" }
func (s *selectorBase) GetTitle() string {
	return s.title
}
func (s *selectorBase) PanelTitle(string) string { return s.panelTitle }
func (s *selectorBase) PanelInfoKey(req vfs.PanelInfoRequest) string {
	return s.panelInfo.PanelInfoKey(req)
}
func (s *selectorBase) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	return s.panelInfo.CachedPanelInfo(req)
}
func (s *selectorBase) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	return s.panelInfo.RefreshPanelInfo(ctx, req)
}
func (s *selectorBase) IsAbs(p string) bool { return path.IsAbs(p) }
func (s *selectorBase) SetPath(p string) error {
	if isSelectorRoot(p) {
		return nil
	}
	return fmt.Errorf("ios: selector has no directory %q: %w", p, os.ErrNotExist)
}
func (s *selectorBase) Join(elem ...string) string { return path.Join(elem...) }
func (s *selectorBase) Abs(p string) (string, error) {
	if isSelectorRoot(p) {
		return "/", nil
	}
	if path.IsAbs(p) {
		return path.Clean(p), nil
	}
	return path.Join("/", p), nil
}
func (s *selectorBase) Base(p string) string { return path.Base(p) }
func (s *selectorBase) Dir(string) string    { return "/" }
func (s *selectorBase) MkDir(context.Context, string) error {
	return ErrSelectorReadOnly
}
func (s *selectorBase) Remove(context.Context, string) error { return ErrSelectorReadOnly }
func (s *selectorBase) Rename(context.Context, string, string) error {
	return ErrSelectorReadOnly
}
func (s *selectorBase) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrSelectorReadOnly
}
func (s *selectorBase) GetCapabilities() vfs.VFSCapabilities { return vfs.VFSCapabilities{} }
func (s *selectorBase) Search(context.Context, string, string) (chan int64, error) {
	return nil, ErrSelectorReadOnly
}
func (s *selectorBase) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, ErrSelectorReadOnly
}
func (s *selectorBase) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, ErrSelectorReadOnly
}
func (s *selectorBase) ParentVFS() vfs.VFS { return s.parent }
func (s *selectorBase) Close() error       { return nil }

func isSelectorRoot(p string) bool {
	return p == "" || p == "." || p == "/"
}

// directSelectorName accepts one direct child of the virtual root and rejects
// nested paths even when their base name happens to equal a known selector.
func directSelectorName(p string) (string, bool) {
	if isSelectorRoot(p) {
		return "", false
	}
	trimmed := strings.TrimPrefix(p, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") || path.Clean(trimmed) != trimmed {
		return "", false
	}
	return trimmed, true
}

func deviceIdentity(device DeviceInfo) string {
	if id := strings.TrimSpace(device.UDID); id != "" {
		return id
	}
	return "unknown"
}

func deviceLabel(device DeviceInfo) string {
	if name := strings.TrimSpace(device.Name); name != "" {
		return name
	}
	if model := strings.TrimSpace(device.Model); model != "" {
		return model
	}
	return deviceIdentity(device)
}

// DeviceRootVFS is the fixed capability selector mounted below one device.
type DeviceRootVFS struct {
	selectorBase
	device DeviceInfo
	opener SelectorOpener
	rows   map[string]Capability
}

func NewDeviceRootVFS(parent vfs.VFS, device DeviceInfo, opener SelectorOpener) *DeviceRootVFS {
	id := deviceIdentity(device)
	displayRoot := iosDeviceTitle(device)
	return &DeviceRootVFS{
		selectorBase: newSelectorBase(parent, device, "iOS:"+id, iosPanelTitle(displayRoot, "/"), "Lockdown"),
		device:       device,
		opener:       opener,
		rows: map[string]Capability{
			MediaSelector:        CapabilityMedia,
			ApplicationsSelector: CapabilityApplications,
			AppGroupsSelector:    CapabilityAppGroups,
			CrashReportsSelector: CapabilityCrashReports,
		},
	}
}

func (d *DeviceRootVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isSelectorRoot(p) {
		return os.ErrNotExist
	}
	items := []vfs.VFSItem{
		{Name: MediaSelector, IsDir: true, IsExecutable: true, NoExtension: true},
		{Name: ApplicationsSelector, IsDir: true, IsExecutable: true, NoExtension: true},
	}
	if d.capabilityAvailable(CapabilityAppGroups) {
		// Keep unavailable capabilities out of the directory altogether. VFSItem
		// has no disabled-directory state, and a visible directory with no
		// provider would incorrectly fall through to selectorBase.SetPath.
		items = append(items, vfs.VFSItem{Name: AppGroupsSelector, IsDir: true, IsExecutable: true, NoExtension: true})
	}
	items = append(items, vfs.VFSItem{Name: CrashReportsSelector, IsDir: true, IsExecutable: true, NoExtension: true})
	if onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (d *DeviceRootVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	if isSelectorRoot(p) {
		return vfs.VFSItem{Name: deviceLabel(d.device), IsDir: true}, nil
	}
	name, ok := directSelectorName(p)
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	capability, ok := d.rows[name]
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	if !d.capabilityAvailable(capability) {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	return vfs.VFSItem{Name: name, IsDir: true, IsExecutable: true, NoExtension: true}, nil
}

func (d *DeviceRootVFS) Clone() vfs.VFS {
	return NewDeviceRootVFS(d.parent, d.device, d.opener)
}

func (d *DeviceRootVFS) capabilityForPath(p string) (Capability, bool) {
	name, ok := directSelectorName(p)
	if !ok {
		return 0, false
	}
	capability, ok := d.rows[name]
	return capability, ok && d.capabilityAvailable(capability)
}

func (d *DeviceRootVFS) capabilityAvailable(capability Capability) bool {
	if capability != CapabilityAppGroups {
		return true
	}
	if !coreAccessSupported() {
		return false
	}
	// The iOS 26.5 FileService daemon rejects container paths with
	// RemoteServices error 11007. Keep the row visible but non-enterable.
	return coreDeviceVersionAvailable(d.device.OSVersion)
}

// ApplicationsVFS discovers apps on refresh and keeps an exact snapshot for
// provider resolution. Including BundleID makes duplicate display names safe.
type ApplicationsVFS struct {
	selectorBase
	device DeviceInfo
	source AppSource
	opener AppOpener

	mu   sync.RWMutex
	apps map[string]AppInfo
}

func NewApplicationsVFS(parent vfs.VFS, device DeviceInfo, source AppSource, opener AppOpener) *ApplicationsVFS {
	id := deviceIdentity(device)
	displayRoot := iosDeviceTitle(device, ApplicationsSelector)
	return &ApplicationsVFS{
		selectorBase: newSelectorBase(parent, device, "iOS:"+id+":applications", iosPanelTitle(displayRoot, "/"), "Installation Proxy"),
		device:       device,
		source:       source,
		opener:       opener,
		apps:         make(map[string]AppInfo),
	}
}

func AppDisplayName(app AppInfo) string {
	bundleID := strings.TrimSpace(app.BundleID)
	displayName := strings.TrimSpace(app.DisplayName)
	if displayName == "" || displayName == bundleID {
		return virtualRowName(bundleID)
	}
	return virtualRowName(fmt.Sprintf("%s (%s)", displayName, bundleID))
}

func (a *ApplicationsVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	if !isSelectorRoot(p) {
		return os.ErrNotExist
	}
	if a.source == nil {
		a.replaceApps(nil)
		return ErrNoAppSource
	}
	apps, err := a.source.ListApps(ctx, a.device)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
			a.replaceApps(nil)
		}
		return err
	}

	byName := make(map[string]AppInfo, len(apps))
	for _, app := range apps {
		if strings.TrimSpace(app.BundleID) == "" {
			continue
		}
		byName[AppDisplayName(app)] = app
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]vfs.VFSItem, 0, len(names))
	for _, name := range names {
		items = append(items, vfs.VFSItem{Name: name, IsDir: true, IsExecutable: true, NoExtension: true})
	}
	a.replaceApps(byName)
	if len(items) != 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (a *ApplicationsVFS) replaceApps(apps map[string]AppInfo) {
	if apps == nil {
		apps = make(map[string]AppInfo)
	}
	a.mu.Lock()
	a.apps = apps
	a.mu.Unlock()
}

func (a *ApplicationsVFS) appForPath(p string) (AppInfo, bool) {
	name, ok := directSelectorName(p)
	if !ok {
		return AppInfo{}, false
	}
	a.mu.RLock()
	app, ok := a.apps[name]
	a.mu.RUnlock()
	return app, ok
}

func (a *ApplicationsVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	if isSelectorRoot(p) {
		return vfs.VFSItem{Name: ApplicationsSelector, IsDir: true}, nil
	}
	app, ok := a.appForPath(p)
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	return vfs.VFSItem{Name: AppDisplayName(app), IsDir: true, IsExecutable: true, NoExtension: true}, nil
}

func (a *ApplicationsVFS) Clone() vfs.VFS {
	clone := NewApplicationsVFS(a.parent, a.device, a.source, a.opener)
	a.mu.RLock()
	apps := make(map[string]AppInfo, len(a.apps))
	for name, app := range a.apps {
		apps[name] = app
	}
	a.mu.RUnlock()
	clone.apps = apps
	return clone
}

func (a *ApplicationsVFS) Close() error {
	a.replaceApps(nil)
	return nil
}

// AppGroupsVFS is a fixed selector created from the application metadata
// snapshot. IDs are trimmed, deduplicated and sorted before display.
type AppGroupsVFS struct {
	selectorBase
	device DeviceInfo
	opener GroupOpener
	groups map[string]string
}

func NewAppGroupsVFS(parent vfs.VFS, device DeviceInfo, groups []string, opener GroupOpener) *AppGroupsVFS {
	id := deviceIdentity(device)
	displayRoot := iosDeviceTitle(device, AppGroupsSelector)
	groupMap := make(map[string]string, len(groups))
	for _, groupID := range groups {
		groupID = strings.TrimSpace(groupID)
		if groupID != "" {
			groupMap[virtualRowName(groupID)] = groupID
		}
	}
	return &AppGroupsVFS{
		selectorBase: newSelectorBase(parent, device, "iOS:"+id+":app-groups", iosPanelTitle(displayRoot, "/"), "CoreDevice"),
		device:       device,
		opener:       opener,
		groups:       groupMap,
	}
}

func (g *AppGroupsVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isSelectorRoot(p) {
		return os.ErrNotExist
	}
	names := make([]string, 0, len(g.groups))
	for name := range g.groups {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]vfs.VFSItem, 0, len(names))
	for _, name := range names {
		items = append(items, vfs.VFSItem{Name: name, IsDir: true, IsExecutable: true, NoExtension: true})
	}
	if len(items) != 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (g *AppGroupsVFS) groupForPath(p string) (string, bool) {
	name, ok := directSelectorName(p)
	if !ok {
		return "", false
	}
	groupID, ok := g.groups[name]
	return groupID, ok
}

func (g *AppGroupsVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	if isSelectorRoot(p) {
		return vfs.VFSItem{Name: AppGroupsSelector, IsDir: true}, nil
	}
	groupID, ok := g.groupForPath(p)
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	return vfs.VFSItem{Name: virtualRowName(groupID), IsDir: true, IsExecutable: true, NoExtension: true}, nil
}

func (g *AppGroupsVFS) Clone() vfs.VFS {
	groups := make([]string, 0, len(g.groups))
	for _, groupID := range g.groups {
		groups = append(groups, groupID)
	}
	return NewAppGroupsVFS(g.parent, g.device, groups, g.opener)
}

// SelectorProvider only handles fixed capability rows inside DeviceRootVFS.
type SelectorProvider struct{}

func (*SelectorProvider) Name() string                  { return "iOS-capability" }
func (*SelectorProvider) Priority() int                 { return 210 }
func (*SelectorProvider) OpensVirtualDirectories() bool { return true }
func (*SelectorProvider) CanOpen(_ context.Context, parent vfs.VFS, p string) bool {
	switch selector := parent.(type) {
	case *DeviceRootVFS:
		if selector.opener == nil {
			return false
		}
		_, ok := selector.capabilityForPath(p)
		return ok
	case *AFCVFS:
		if selector.rootOpener == nil {
			return false
		}
		_, ok := selector.rootCapabilityForPath(p)
		return ok
	default:
		return false
	}
}
func (*SelectorProvider) Open(ctx context.Context, parent vfs.VFS, p string) (vfs.VFS, error) {
	switch selector := parent.(type) {
	case *DeviceRootVFS:
		capability, ok := selector.capabilityForPath(p)
		if !ok {
			return nil, fmt.Errorf("ios: capability %q: %w", p, os.ErrNotExist)
		}
		if selector.opener == nil {
			return nil, ErrNoSelectorOpener
		}
		return selector.opener.OpenSelection(ctx, selector, selector.device, capability)
	case *AFCVFS:
		capability, ok := selector.rootCapabilityForPath(p)
		if !ok {
			return nil, fmt.Errorf("ios: capability %q: %w", p, os.ErrNotExist)
		}
		if selector.rootOpener == nil {
			return nil, ErrNoSelectorOpener
		}
		return selector.rootOpener.OpenSelection(ctx, selector, selector.device, capability)
	default:
		return nil, fmt.Errorf("ios: unsupported capability parent %T", parent)
	}
}

// ApplicationProvider only handles rows from the last successful app listing.
type ApplicationProvider struct{}

func (*ApplicationProvider) Name() string                  { return "iOS-application" }
func (*ApplicationProvider) Priority() int                 { return 210 }
func (*ApplicationProvider) OpensVirtualDirectories() bool { return true }
func (*ApplicationProvider) CanOpen(_ context.Context, parent vfs.VFS, p string) bool {
	apps, ok := parent.(*ApplicationsVFS)
	if !ok || apps.opener == nil {
		return false
	}
	_, ok = apps.appForPath(p)
	return ok
}
func (*ApplicationProvider) Open(ctx context.Context, parent vfs.VFS, p string) (vfs.VFS, error) {
	apps, ok := parent.(*ApplicationsVFS)
	if !ok {
		return nil, fmt.Errorf("ios: unsupported application parent %T", parent)
	}
	app, ok := apps.appForPath(p)
	if !ok {
		return nil, fmt.Errorf("ios: application %q: %w", p, os.ErrNotExist)
	}
	if apps.opener == nil {
		return nil, ErrNoAppOpener
	}
	return apps.opener.OpenApp(ctx, apps, apps.device, app)
}

// GroupProvider only handles exact app-group IDs exposed by AppGroupsVFS.
type GroupProvider struct{}

func (*GroupProvider) Name() string                  { return "iOS-app-group" }
func (*GroupProvider) Priority() int                 { return 210 }
func (*GroupProvider) OpensVirtualDirectories() bool { return true }
func (*GroupProvider) CanOpen(_ context.Context, parent vfs.VFS, p string) bool {
	groups, ok := parent.(*AppGroupsVFS)
	if !ok || groups.opener == nil {
		return false
	}
	_, ok = groups.groupForPath(p)
	return ok
}
func (*GroupProvider) Open(ctx context.Context, parent vfs.VFS, p string) (vfs.VFS, error) {
	groups, ok := parent.(*AppGroupsVFS)
	if !ok {
		return nil, fmt.Errorf("ios: unsupported app-group parent %T", parent)
	}
	groupID, ok := groups.groupForPath(p)
	if !ok {
		return nil, fmt.Errorf("ios: app group %q: %w", p, os.ErrNotExist)
	}
	if groups.opener == nil {
		return nil, ErrNoGroupOpener
	}
	return groups.opener.OpenGroup(ctx, groups, groups.device, groupID)
}

var (
	_ vfs.VFS                      = (*DeviceRootVFS)(nil)
	_ vfs.VFS                      = (*ApplicationsVFS)(nil)
	_ vfs.VFS                      = (*AppGroupsVFS)(nil)
	_ vfs.PanelTitleProvider       = (*DeviceRootVFS)(nil)
	_ vfs.PanelTitleProvider       = (*ApplicationsVFS)(nil)
	_ vfs.PanelTitleProvider       = (*AppGroupsVFS)(nil)
	_ vfs.PanelInfoProvider        = (*DeviceRootVFS)(nil)
	_ vfs.PanelInfoProvider        = (*ApplicationsVFS)(nil)
	_ vfs.PanelInfoProvider        = (*AppGroupsVFS)(nil)
	_ vfs.VFSProvider              = (*SelectorProvider)(nil)
	_ vfs.VFSProvider              = (*ApplicationProvider)(nil)
	_ vfs.VFSProvider              = (*GroupProvider)(nil)
	_ vfs.VirtualDirectoryProvider = (*SelectorProvider)(nil)
	_ vfs.VirtualDirectoryProvider = (*ApplicationProvider)(nil)
	_ vfs.VirtualDirectoryProvider = (*GroupProvider)(nil)
)
