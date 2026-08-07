// Package iosfs exposes Apple mobile devices connected to the host as a
// top-level f4 drive.
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
	iosRoot = "ios://"

	DeviceStateReady    = "ready"
	DeviceStateLocked   = "locked"
	DeviceStateUnpaired = "unpaired"
	DeviceStateOffline  = "offline"
)

var (
	ErrNoDeviceSource    = errors.New("ios: device source is not configured")
	ErrNoDeviceOpener    = errors.New("ios: device opener is not configured")
	ErrDeviceUnavailable = errors.New("ios: device is not available")
	ErrManagerReadOnly   = errors.New("ios: device list is read-only")
)

// DeviceInfo is the transport-independent identity and status presented by
// the iOS drive. UDID is the stable key; the remaining fields may be refreshed
// whenever ReadDir discovers the device again.
type DeviceInfo struct {
	UDID           string
	Name           string
	Model          string
	ProductType    string
	OSVersion      string
	BuildVersion   string
	ConnectionType string
	State          string
	Paired         bool
	DeveloperMode  bool
}

// DeviceSource discovers the devices currently visible through the platform
// transport. ManagerVFS deliberately refreshes it for every ReadDir call.
type DeviceSource interface {
	ListDevices(context.Context) ([]DeviceInfo, error)
}

type DeviceSourceFunc func(context.Context) ([]DeviceInfo, error)

func (f DeviceSourceFunc) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	return f(ctx)
}

// DeviceOpener mounts the capability root for one ready device. Discovery and
// transport selection remain separate so the manager is independently
// testable and does not depend on usbmuxd or CoreDevice packages.
type DeviceOpener interface {
	OpenDevice(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error)
}

type DeviceOpenerFunc func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error)

func (f DeviceOpenerFunc) OpenDevice(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	return f(ctx, parent, device)
}

// ManagerVFS is the ios:// root. Its executable pseudo-files are opened by
// deviceProvider rather than treated as ordinary files.
type ManagerVFS struct {
	source DeviceSource
	opener DeviceOpener

	mu      sync.RWMutex
	devices map[string]DeviceInfo // keyed by the exact displayed row name
}

func NewManagerVFS(source DeviceSource, opener DeviceOpener) *ManagerVFS {
	return &ManagerVFS{
		source:  source,
		opener:  opener,
		devices: make(map[string]DeviceInfo),
	}
}

func (m *ManagerVFS) IsAtRoot() bool   { return true }
func (m *ManagerVFS) GetPath() string  { return iosRoot }
func (m *ManagerVFS) GetTitle() string { return "iOS" }
func (m *ManagerVFS) PanelTitle(string) string {
	return "Apple mobile devices"
}

func (m *ManagerVFS) IsAbs(p string) bool { return strings.HasPrefix(p, iosRoot) }

func (m *ManagerVFS) SetPath(p string) error {
	if p == "" || p == "." || p == "/" || p == iosRoot {
		return nil
	}
	return fmt.Errorf("ios: manager has no directory %q: %w", p, os.ErrNotExist)
}

func deviceReady(device DeviceInfo) bool {
	return device.Paired && strings.EqualFold(strings.TrimSpace(device.State), DeviceStateReady)
}

// DeviceDisplayName is the concise label shown to the user. The UDID remains
// the internal stable identity and is available in Ctrl+L; putting it in every
// row makes the common one-device case needlessly noisy.
func DeviceDisplayName(device DeviceInfo) string {
	label := firstNonEmpty(device.Name, device.Model, device.ProductType, "iOS device")
	name := virtualRowName(label)
	if deviceReady(device) {
		return name
	}
	state := strings.TrimSpace(device.State)
	if !device.Paired {
		state = DeviceStateUnpaired
	} else if state == "" {
		state = "unknown"
	}
	return name + " [" + state + "]"
}

// virtualRowName keeps untrusted labels representable as one direct VFS child.
// The original values remain available in DeviceInfo/AppInfo and are still
// passed to native services; only the panel-facing row is escaped.
func virtualRowName(value string) string {
	value = strings.ReplaceAll(value, "/", "⁄")
	return strings.ReplaceAll(value, "\x00", "�")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (m *ManagerVFS) ReadDir(ctx context.Context, _ string, onChunk func([]vfs.VFSItem)) error {
	if m.source == nil {
		m.replaceDevices(nil)
		return ErrNoDeviceSource
	}

	devices, err := m.source.ListDevices(ctx)
	if err != nil {
		// Preserve the last completed snapshot when a superseded panel refresh
		// is canceled: a provider open already launched from it may still be in
		// flight. Other discovery failures invalidate stale openable rows.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return err
		}
		m.replaceDevices(nil)
		return err
	}

	byUDID := make(map[string]DeviceInfo, len(devices))
	for _, device := range devices {
		udid := strings.TrimSpace(device.UDID)
		if udid == "" {
			continue
		}
		device.UDID = udid
		if previous, exists := byUDID[udid]; !exists || preferDevice(device, previous) {
			byUDID[udid] = device
		}
	}

	ordered := make([]DeviceInfo, 0, len(byUDID))
	for _, device := range byUDID {
		ordered = append(ordered, device)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := DeviceDisplayName(ordered[i]), DeviceDisplayName(ordered[j])
		if left != right {
			return left < right
		}
		return ordered[i].UDID < ordered[j].UDID
	})

	byName := make(map[string]DeviceInfo, len(ordered))
	items := make([]vfs.VFSItem, 0, len(ordered))
	for _, device := range ordered {
		base := DeviceDisplayName(device)
		name := base
		for suffix := 2; ; suffix++ {
			if _, exists := byName[name]; !exists {
				break
			}
			name = fmt.Sprintf("%s (%d)", base, suffix)
		}
		byName[name] = device
		items = append(items, vfs.VFSItem{
			Name:         name,
			IsDir:        true,
			IsExecutable: deviceReady(device),
			NoExtension:  true,
		})
	}
	m.mu.Lock()
	m.devices = byName
	m.mu.Unlock()

	if len(items) > 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

func preferDevice(candidate, current DeviceInfo) bool {
	if deviceReady(candidate) != deviceReady(current) {
		return deviceReady(candidate)
	}
	if candidate.Paired != current.Paired {
		return candidate.Paired
	}
	candidateUSB := strings.EqualFold(strings.TrimSpace(candidate.ConnectionType), "USB")
	currentUSB := strings.EqualFold(strings.TrimSpace(current.ConnectionType), "USB")
	if candidateUSB != currentUSB {
		return candidateUSB
	}
	// Make ties deterministic even when the source changes enumeration order.
	return fmt.Sprintf("%#v", candidate) < fmt.Sprintf("%#v", current)
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
	name, direct := directManagerRowName(p)
	if !direct {
		return DeviceInfo{}, false
	}
	m.mu.RLock()
	device, ok := m.devices[name]
	m.mu.RUnlock()
	return device, ok
}

func directManagerRowName(p string) (string, bool) {
	if strings.HasPrefix(p, iosRoot) {
		p = strings.TrimPrefix(p, iosRoot)
	} else {
		p = strings.TrimPrefix(p, "/")
	}
	if p == "" || strings.Contains(p, "/") || path.Clean(p) != p {
		return "", false
	}
	return p, true
}

func (m *ManagerVFS) selectedDevice(req vfs.PanelInfoRequest) (DeviceInfo, bool) {
	selected := strings.TrimSpace(req.SelectedName)
	if selected == "" {
		selected = strings.TrimSpace(req.Path)
	}
	if selected == "" || selected == iosRoot || selected == "/" || selected == "." {
		return DeviceInfo{}, false
	}
	return m.deviceForPath(selected)
}

func (m *ManagerVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if device, ok := m.selectedDevice(req); ok {
		return "ios-manager:" + device.UDID
	}
	return "ios-manager"
}

func (m *ManagerVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	if device, ok := m.selectedDevice(req); ok {
		return deviceBaselineSnapshot(device), true
	}
	return vfs.PanelInfoSnapshot{Authoritative: true}, true
}

func (m *ManagerVFS) RefreshPanelInfo(_ context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	snapshot, _ := m.CachedPanelInfo(req)
	return snapshot, nil
}

func deviceBaselineSnapshot(device DeviceInfo) vfs.PanelInfoSnapshot {
	fields := make([]vfs.PanelInfoField, 0, 10)
	fields = appendPanelText(fields, "name", "IOSInfo.Name", "Name", device.Name)
	fields = appendPanelText(fields, "model", "IOSInfo.Model", "Model", device.Model)
	fields = appendPanelText(fields, "product_type", "IOSInfo.ProductType", "Product type", device.ProductType)
	fields = appendPanelText(fields, "udid", "IOSInfo.UDID", "UDID", device.UDID)
	fields = appendPanelText(fields, "ios", "IOSInfo.OSVersion", "iOS", device.OSVersion)
	fields = appendPanelText(fields, "build", "IOSInfo.Build", "Build", device.BuildVersion)
	fields = appendPanelText(fields, "connection", "IOSInfo.Connection", "Connection", device.ConnectionType)
	fields = appendPanelText(fields, "state", "IOSInfo.State", "State", device.State)
	fields = append(fields,
		vfs.PanelInfoField{ID: "paired", LabelKey: "IOSInfo.Paired", Label: "Paired", Value: yesNo(device.Paired), Kind: vfs.PanelInfoText},
		vfs.PanelInfoField{ID: "developer_mode", LabelKey: "IOSInfo.DeveloperMode", Label: "Developer Mode", Value: yesNo(device.DeveloperMode), Kind: vfs.PanelInfoText},
	)
	return vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID: "ios.device", TitleKey: "IOSInfo.DeviceTitle", Title: "Apple mobile device", Fields: fields,
		}},
	}
}

// devicePanelInfoProvider keeps the physical-device facts attached while the
// panel moves through virtual selectors and mounted CoreDevice domains.
// Transport-specific filesystems may add their own fields on top of the same
// baseline (AFC also reports storage, for example).
type devicePanelInfoProvider struct {
	device  DeviceInfo
	backend string
}

func newDevicePanelInfoProvider(device DeviceInfo, backend string) *devicePanelInfoProvider {
	return &devicePanelInfoProvider{device: device, backend: backend}
}

func (p *devicePanelInfoProvider) PanelInfoKey(vfs.PanelInfoRequest) string {
	return "ios-device:" + p.device.UDID + ":" + p.backend
}

func (p *devicePanelInfoProvider) snapshot() vfs.PanelInfoSnapshot {
	snapshot := deviceBaselineSnapshot(p.device)
	if p.backend != "" && len(snapshot.Sections) != 0 {
		snapshot.Sections[0].Fields = append(snapshot.Sections[0].Fields,
			vfs.PanelInfoField{ID: "backend", Label: "Backend", Value: p.backend, Kind: vfs.PanelInfoText})
	}
	return snapshot
}

func (p *devicePanelInfoProvider) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	return p.snapshot(), true
}

func (p *devicePanelInfoProvider) RefreshPanelInfo(context.Context, vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	return p.snapshot(), nil
}

func appendPanelText(fields []vfs.PanelInfoField, id, key, label, value string) []vfs.PanelInfoField {
	if strings.TrimSpace(value) == "" {
		return fields
	}
	return append(fields, vfs.PanelInfoField{ID: id, LabelKey: key, Label: label, Value: value, Kind: vfs.PanelInfoText})
}

func yesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func (m *ManagerVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	if p == "" || p == "." || p == "/" || p == iosRoot {
		return vfs.VFSItem{Name: "iOS", IsDir: true}, nil
	}
	device, ok := m.deviceForPath(p)
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	name, _ := directManagerRowName(p)
	return vfs.VFSItem{Name: name, IsDir: true, IsExecutable: deviceReady(device), NoExtension: true}, nil
}

func (m *ManagerVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	if !m.IsAbs(elem[0]) {
		return path.Join(elem...)
	}
	parts := append([]string{strings.TrimPrefix(elem[0], iosRoot)}, elem[1:]...)
	joined := strings.TrimPrefix(path.Join(parts...), "/")
	if joined == "" || joined == "." {
		return iosRoot
	}
	return iosRoot + joined
}

func (m *ManagerVFS) Abs(p string) (string, error) {
	if m.IsAbs(p) {
		return p, nil
	}
	if p == "" || p == "." || p == "/" {
		return iosRoot, nil
	}
	return m.Join(iosRoot, p), nil
}

func (m *ManagerVFS) Base(p string) string {
	if m.IsAbs(p) {
		p = strings.TrimPrefix(p, iosRoot)
	}
	return path.Base(p)
}

func (*ManagerVFS) Dir(string) string { return iosRoot }

func (*ManagerVFS) MkDir(context.Context, string) error  { return ErrManagerReadOnly }
func (*ManagerVFS) Remove(context.Context, string) error { return ErrManagerReadOnly }
func (*ManagerVFS) Rename(context.Context, string, string) error {
	return ErrManagerReadOnly
}
func (*ManagerVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrManagerReadOnly
}
func (*ManagerVFS) GetCapabilities() vfs.VFSCapabilities { return vfs.VFSCapabilities{} }
func (*ManagerVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, ErrManagerReadOnly
}
func (*ManagerVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, ErrManagerReadOnly
}
func (*ManagerVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, ErrManagerReadOnly
}
func (*ManagerVFS) ParentVFS() vfs.VFS { return nil }

func (m *ManagerVFS) Clone() vfs.VFS { return NewManagerVFS(m.source, m.opener) }

func (m *ManagerVFS) Close() error {
	m.replaceDevices(nil)
	return nil
}

var _ vfs.PanelInfoProvider = (*ManagerVFS)(nil)
var _ vfs.PanelTitleProvider = (*ManagerVFS)(nil)

// deviceProvider intentionally recognizes only rows previously discovered by
// ManagerVFS. This prevents iOS probing from intercepting archives or ordinary
// executable files in other filesystems.
type deviceProvider struct{}

func (*deviceProvider) Name() string                  { return "iOS-device" }
func (*deviceProvider) Priority() int                 { return 200 }
func (*deviceProvider) OpensVirtualDirectories() bool { return true }

func (*deviceProvider) CanOpen(_ context.Context, parent vfs.VFS, p string) bool {
	manager, ok := parent.(*ManagerVFS)
	if !ok || manager.opener == nil {
		return false
	}
	device, ok := manager.deviceForPath(p)
	return ok && deviceReady(device)
}

func (*deviceProvider) Open(ctx context.Context, parent vfs.VFS, p string) (vfs.VFS, error) {
	manager, ok := parent.(*ManagerVFS)
	if !ok {
		return nil, fmt.Errorf("ios: unsupported parent %T", parent)
	}
	device, ok := manager.deviceForPath(p)
	if !ok {
		return nil, fmt.Errorf("ios: %q: %w", p, os.ErrNotExist)
	}
	if !deviceReady(device) {
		return nil, fmt.Errorf("ios: device %q is %s: %w", device.UDID, device.State, ErrDeviceUnavailable)
	}
	if manager.opener == nil {
		return nil, ErrNoDeviceOpener
	}
	return manager.opener.OpenDevice(ctx, manager, device)
}
