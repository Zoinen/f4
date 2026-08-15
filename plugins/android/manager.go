// Package androidfs exposes Android devices connected to the local ADB server
// as a top-level f4 drive.
package androidfs

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
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	androidRoot = "android://"

	// DeviceStateOnline is the state reported by ADB for a device that can
	// accept services. The wording is ADB's own: "device" means online.
	DeviceStateOnline       = "device"
	DeviceStateOffline      = "offline"
	DeviceStateUnauthorized = "unauthorized"
)

var (
	ErrNoDeviceSource       = errors.New("android: device source is not configured")
	ErrNoDeviceOpener       = errors.New("android: device opener is not configured")
	ErrDeviceUnavailable    = errors.New("android: device is not available")
	ErrAuthorizationPending = errors.New("android: device authorization is pending")
	ErrManagerReadOnly      = errors.New("android: device list is read-only")
)

const (
	authorizationWaitTimeout = 60 * time.Second
	authorizationPollDelay   = 250 * time.Millisecond
)

// DeviceInfo is the transport-independent description shown in the Android
// drive. It deliberately mirrors only stable fields from ADB's devices-l
// response so the manager does not depend on a particular ADB implementation.
type DeviceInfo struct {
	Serial      string
	State       string
	Model       string
	Product     string
	Device      string
	TransportID string
}

// DeviceSource discovers the devices currently known to ADB. ReadDir calls it
// every time, so reopening or refreshing the drive reflects current server
// state without a background hot-plug watcher.
type DeviceSource interface {
	ListDevices(ctx context.Context) ([]DeviceInfo, error)
}

// DeviceAuthorizationRestarter is implemented by an ADB-backed source that
// can restart the host daemon and recreate an unauthorized USB transport.
type DeviceAuthorizationRestarter interface {
	RestartForAuthorization(ctx context.Context) error
}

// DeviceSourceFunc adapts a function to DeviceSource.
type DeviceSourceFunc func(context.Context) ([]DeviceInfo, error)

func (f DeviceSourceFunc) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	return f(ctx)
}

// DeviceOpener chooses and creates the filesystem implementation for one
// online device. The manager owns discovery and selection; transport/FISH/Sync
// negotiation remains outside this file.
type DeviceOpener interface {
	OpenDevice(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error)
}

// DeviceOpenerFunc adapts a function to DeviceOpener.
type DeviceOpenerFunc func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error)

func (f DeviceOpenerFunc) OpenDevice(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	return f(ctx, parent, device)
}

// Plugin registers the separate Android drive and its deliberately narrow
// provider. Source and Opener are injectable to keep discovery and backend
// negotiation independently testable.
type Plugin struct {
	Source DeviceSource
	Opener DeviceOpener
	closer io.Closer
	info   *deviceInfoService
}

func (p *Plugin) Init(api vfs.HostAPI) error {
	api.RegisterVFSProvider(&deviceProvider{})
	api.RegisterDrive("Android", func() vfs.VFS {
		return newManagerVFS(p.Source, p.Opener, p.info)
	})
	return nil
}

func (p *Plugin) Close() error {
	if p.closer == nil {
		return nil
	}
	return p.closer.Close()
}
func (p *Plugin) GetName() string { return "Android" }

// ManagerVFS is the root of the Android drive. Device rows are executable
// pseudo-files so the normal VFS-provider mechanism can mount the selected
// device.
type ManagerVFS struct {
	source DeviceSource
	opener DeviceOpener
	info   *deviceInfoService

	mu      sync.RWMutex
	devices map[string]DeviceInfo // keyed by the exact displayed row name
}

func NewManagerVFS(source DeviceSource, opener DeviceOpener) *ManagerVFS {
	return newManagerVFS(source, opener, nil)
}

func newManagerVFS(source DeviceSource, opener DeviceOpener, info *deviceInfoService) *ManagerVFS {
	return &ManagerVFS{
		source:  source,
		opener:  opener,
		info:    info,
		devices: make(map[string]DeviceInfo),
	}
}

func (m *ManagerVFS) IsAtRoot() bool   { return true }
func (m *ManagerVFS) GetPath() string  { return androidRoot }
func (m *ManagerVFS) GetTitle() string { return "Android" }

// PanelTitle hides the manager's canonical android:// URI from the panel
// border. It is an implementation detail; at this level the user is simply
// looking at the list of connected Android devices.
func (m *ManagerVFS) PanelTitle(string) string { return "Android devices" }

func (m *ManagerVFS) IsAbs(p string) bool {
	return strings.HasPrefix(p, androidRoot)
}

func (m *ManagerVFS) SetPath(p string) error {
	if p == "" || p == "." || p == "/" || p == androidRoot {
		return nil
	}
	return fmt.Errorf("android: manager has no directory %q: %w", p, os.ErrNotExist)
}

// DeviceDisplayName is stable and unique because the ADB serial is always
// included when a model is available. Non-online states remain visible but are
// clearly labelled and cannot be opened.
func DeviceDisplayName(device DeviceInfo) string {
	serial := strings.TrimSpace(device.Serial)
	model := strings.TrimSpace(device.Model)
	name := serial
	if model != "" {
		name = fmt.Sprintf("%s (%s)", model, serial)
	}
	state := strings.TrimSpace(device.State)
	if state != DeviceStateOnline {
		if state == "" {
			state = "unknown"
		}
		name += " [" + state + "]"
	}
	return name
}

func (m *ManagerVFS) ReadDir(ctx context.Context, _ string, onChunk func([]vfs.VFSItem)) error {
	if m.source == nil {
		m.replaceDevices(nil)
		return ErrNoDeviceSource
	}

	devices, err := m.source.ListDevices(ctx)
	if err != nil {
		// Canceling a superseded panel refresh says nothing about whether the
		// last successfully discovered devices still exist. Keep that snapshot:
		// a provider Open already started from one of its rows may still need to
		// resolve the exact device after the listing context is canceled.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return err
		}
		// A stale row must not remain openable after ADB reports that discovery
		// itself is unavailable.
		m.replaceDevices(nil)
		return err
	}

	byName := make(map[string]DeviceInfo, len(devices))
	items := make([]vfs.VFSItem, 0, len(devices))
	for _, device := range devices {
		if strings.TrimSpace(device.Serial) == "" {
			continue
		}
		name := DeviceDisplayName(device)
		byName[name] = device
		items = append(items, vfs.VFSItem{
			Name:         name,
			IsDir:        true,
			IsExecutable: device.State == DeviceStateOnline || device.State == DeviceStateUnauthorized,
			NoExtension:  true,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	m.mu.Lock()
	m.devices = byName
	m.mu.Unlock()

	if len(items) != 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (m *ManagerVFS) replaceDevices(devices map[string]DeviceInfo) {
	if devices == nil {
		devices = make(map[string]DeviceInfo)
	}
	m.mu.Lock()
	m.devices = devices
	m.mu.Unlock()
}

func (m *ManagerVFS) deviceForPath(p string) (DeviceInfo, bool) {
	name := m.Base(p)
	m.mu.RLock()
	device, ok := m.devices[name]
	m.mu.RUnlock()
	return device, ok
}

func (m *ManagerVFS) panelInfoDevice(req vfs.PanelInfoRequest) (DeviceInfo, bool) {
	selected := strings.TrimSpace(req.SelectedName)
	if selected == "" {
		selected = strings.TrimSpace(req.Path)
	}
	if selected == "" || selected == androidRoot || selected == "/" || selected == "." {
		return DeviceInfo{}, false
	}
	return m.deviceForPath(selected)
}

// PanelInfoKey follows the selected manager row rather than the manager path:
// all rows live at android://, while moving the cursor must immediately replace
// the device facts shown by Ctrl+L.
func (m *ManagerVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if device, ok := m.panelInfoDevice(req); ok {
		if m.info != nil && device.State == DeviceStateOnline {
			return m.info.provider(device, "ADB", "host transport").PanelInfoKey(vfs.PanelInfoRequest{Path: "/"})
		}
		return "android-manager:" + device.Serial
	}
	return "android-manager"
}

func (m *ManagerVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	if device, ok := m.panelInfoDevice(req); ok {
		if m.info != nil && device.State == DeviceStateOnline {
			return m.info.provider(device, "ADB", "host transport").CachedPanelInfo(vfs.PanelInfoRequest{Path: "/"})
		}
		return deviceBaselineSnapshot(device), true
	}
	return vfs.PanelInfoSnapshot{Authoritative: true}, true
}

func (m *ManagerVFS) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if device, ok := m.panelInfoDevice(req); ok && m.info != nil && device.State == DeviceStateOnline {
		return m.info.provider(device, "ADB", "host transport").RefreshPanelInfo(ctx, vfs.PanelInfoRequest{Path: "/"})
	}
	snapshot, _ := m.CachedPanelInfo(req)
	return snapshot, nil
}

func (m *ManagerVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	if p == "" || p == "." || p == "/" || p == androidRoot {
		return vfs.VFSItem{Name: "Android", IsDir: true}, nil
	}
	device, ok := m.deviceForPath(p)
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	return vfs.VFSItem{
		Name:         DeviceDisplayName(device),
		IsDir:        true,
		IsExecutable: device.State == DeviceStateOnline || device.State == DeviceStateUnauthorized,
		NoExtension:  true,
	}, nil
}

func (m *ManagerVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	if !m.IsAbs(elem[0]) {
		return path.Join(elem...)
	}
	parts := append([]string{strings.TrimPrefix(elem[0], androidRoot)}, elem[1:]...)
	joined := strings.TrimPrefix(path.Join(parts...), "/")
	if joined == "" || joined == "." {
		return androidRoot
	}
	return androidRoot + joined
}

func (m *ManagerVFS) Abs(p string) (string, error) {
	if m.IsAbs(p) {
		return p, nil
	}
	if p == "" || p == "." || p == "/" {
		return androidRoot, nil
	}
	return m.Join(androidRoot, p), nil
}

func (m *ManagerVFS) Base(p string) string {
	if m.IsAbs(p) {
		p = strings.TrimPrefix(p, androidRoot)
	}
	return path.Base(p)
}

func (m *ManagerVFS) Dir(string) string { return androidRoot }

func (m *ManagerVFS) MkDir(context.Context, string) error  { return ErrManagerReadOnly }
func (m *ManagerVFS) Remove(context.Context, string) error { return ErrManagerReadOnly }
func (m *ManagerVFS) Rename(context.Context, string, string) error {
	return ErrManagerReadOnly
}
func (m *ManagerVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrManagerReadOnly
}

func (m *ManagerVFS) GetCapabilities() vfs.VFSCapabilities { return vfs.VFSCapabilities{} }
func (m *ManagerVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, ErrManagerReadOnly
}
func (m *ManagerVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, ErrManagerReadOnly
}
func (m *ManagerVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, ErrManagerReadOnly
}

func (m *ManagerVFS) ParentVFS() vfs.VFS { return nil }

func (m *ManagerVFS) Clone() vfs.VFS {
	return newManagerVFS(m.source, m.opener, m.info)
}

func (m *ManagerVFS) Close() error {
	m.replaceDevices(nil)
	return nil
}
func (m *ManagerVFS) IsReadOnly() bool { return true }

var _ vfs.PanelInfoProvider = (*ManagerVFS)(nil)
var _ vfs.PanelTitleProvider = (*ManagerVFS)(nil)

type deviceProvider struct{}

func (*deviceProvider) Name() string                  { return "Android-device" }
func (*deviceProvider) Priority() int                 { return 200 }
func (*deviceProvider) OpensVirtualDirectories() bool { return true }

func (*deviceProvider) CanOpen(_ context.Context, parent vfs.VFS, p string) bool {
	manager, ok := parent.(*ManagerVFS)
	if !ok || manager.opener == nil {
		return false
	}
	device, ok := manager.deviceForPath(p)
	return ok && (device.State == DeviceStateOnline || device.State == DeviceStateUnauthorized)
}

func (*deviceProvider) ProviderOpenStatus(parent vfs.VFS, p string) (vfs.ProviderOpenStatus, bool) {
	manager, ok := parent.(*ManagerVFS)
	if !ok {
		return vfs.ProviderOpenStatus{}, false
	}
	device, ok := manager.deviceForPath(p)
	if !ok || device.State != DeviceStateUnauthorized {
		return vfs.ProviderOpenStatus{}, false
	}
	return vfs.ProviderOpenStatus{
		Title: " Android authorization ",
		Message: fmt.Sprintf(
			"Requesting USB debugging authorization for %s.\n\nUnlock the Android device and accept the USB debugging prompt.\n\nf4 will open the device automatically after authorization.",
			DeviceDisplayName(device),
		),
	}, true
}

func (m *ManagerVFS) authorizeDevice(ctx context.Context, device DeviceInfo) (DeviceInfo, error) {
	restarter, ok := m.source.(DeviceAuthorizationRestarter)
	if !ok {
		return DeviceInfo{}, fmt.Errorf("android: cannot retry authorization for %q: ADB reconnect is unavailable", device.Serial)
	}
	if err := restarter.RestartForAuthorization(ctx); err != nil {
		return DeviceInfo{}, fmt.Errorf("android: request authorization for %q: %w", device.Serial, err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, authorizationWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(authorizationPollDelay)
	defer ticker.Stop()

	for {
		devices, err := m.source.ListDevices(waitCtx)
		if err == nil {
			for _, current := range devices {
				if current.Serial == device.Serial && current.State == DeviceStateOnline {
					return current, nil
				}
			}
		} else if waitCtx.Err() == nil {
			return DeviceInfo{}, fmt.Errorf("android: refresh authorization state for %q: %w", device.Serial, err)
		}

		select {
		case <-ctx.Done():
			return DeviceInfo{}, ctx.Err()
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return DeviceInfo{}, ctx.Err()
			}
			return DeviceInfo{}, fmt.Errorf("%w for %q; unlock the device, accept the USB debugging prompt, then try again", ErrAuthorizationPending, device.Serial)
		case <-ticker.C:
		}
	}
}

func (*deviceProvider) Open(ctx context.Context, parent vfs.VFS, p string) (vfs.VFS, error) {
	manager, ok := parent.(*ManagerVFS)
	if !ok {
		return nil, fmt.Errorf("android: unsupported parent %T", parent)
	}
	device, ok := manager.deviceForPath(p)
	if !ok {
		return nil, fmt.Errorf("android: %q: %w", p, os.ErrNotExist)
	}
	if device.State == DeviceStateUnauthorized {
		var err error
		device, err = manager.authorizeDevice(ctx, device)
		if err != nil {
			return nil, err
		}
	}
	if device.State != DeviceStateOnline {
		return nil, fmt.Errorf("android: device %q is %s: %w", device.Serial, device.State, ErrDeviceUnavailable)
	}
	if manager.opener == nil {
		return nil, ErrNoDeviceOpener
	}
	return manager.opener.OpenDevice(ctx, manager, device)
}
