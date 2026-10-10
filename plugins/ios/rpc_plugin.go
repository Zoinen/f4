package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// RPCPlugin adapts the in-process iOS VFS hierarchy (ManagerVFS, AFCVFS,
// ApplicationsVFS, CoreVFS -- none of it changed by this file) to the
// subprocess RPC transport described in docs/PLUGINS.md and implemented by
// sdk/f4plugin, the same way plugins/cloudfox's RPCPlugin does (f4#1178,
// mirrored per
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645, which
// decided iOS and Android get the same downloadable-plugin treatment as
// cloud storage rather than just being cut from a lite build).
//
// It exists because AFCVFS/ApplicationsVFS/AppGroupsVFS/CoreVFS already
// speak the host-agnostic, path-addressed vfs.VFS interface
// (ReadDir/Stat/Open/... taking a plain path string); only the RPC edge
// needs translating. That edge is a single flat "drive + path" namespace
// (sdk/f4plugin.Plugin), while the in-process model is a fixed multi-level
// tree that deviceProvider/SelectorProvider/ApplicationProvider normally
// walk one vfs.VFS hop at a time (plugin.go, selectors.go): a device row,
// then (for the AFC/Media root) an optional capability row
// ("[Applications]" or "[Crash Reports]"), then (for Applications) an
// application row. resolve below re-implements that same walk against a
// flat path, keyed by the exact virtual row names and capability lookup
// AFCVFS/ApplicationsVFS already expose, and caches each opened mount by the
// flat path prefix that reaches it so a deep path does not reopen House
// Arrest or re-probe AFC on every call.
//
// Known gaps (part 1, mirroring cloudfox's own RPCPlugin doc comment):
//   - App Groups (AppGroupsVFS/GroupProvider) stays unreachable here for the
//     same reason it is unreachable through the in-process nativeBackend
//     today: openDeviceRoot's SetVirtualRoot call (plugin.go) only ever
//     lists Applications and Crash Reports as capability rows under a
//     mounted device (see availableRootCapabilities); DeviceRootVFS and
//     AppGroupsVFS exist but nothing currently wires either of them below a
//     real opened device, in-process or here. This file does not change
//     that.
//   - A lost AFC or CoreDevice session (device unplugged, usbmuxd
//     restarted, ...) has no host-driven reconnect path over this flat RPC
//     surface: unlike a native panel (vfs.SessionReconnector), the
//     f4plugin.Plugin interface exposes no session identity to the host. A
//     stale cached mount below simply starts failing until the plugin
//     process is restarted; this mirrors the transport's existing
//     limitation (the same one dummy_rpc and cloudfox's RPCPlugin have),
//     not something part 1 fixes.
type RPCPlugin struct {
	uriRPCState
	manager *ManagerVFS
	backend *nativeBackend

	mu      sync.Mutex
	mounts  map[string]vfs.VFS
	readers map[uint32]vfs.ReadAtCloser
	writers map[uint32]io.WriteCloser
	nextID  uint32
}

// NewRPCPlugin builds the RPC-facing adapter around a fresh native backend,
// the same one Plugin.Init would otherwise hand to a ManagerVFS in-process.
func NewRPCPlugin() *RPCPlugin {
	backend := &nativeBackend{apps: nativeAppSource{}, afc: newAFCRegistry(), core: newCoreAccess()}
	return &RPCPlugin{
		manager: NewManagerVFS(nativeDeviceSource{}, backend),
		backend: backend,
		mounts:  make(map[string]vfs.VFS),
		readers: make(map[uint32]vfs.ReadAtCloser),
		writers: make(map[uint32]io.WriteCloser),
	}
}

func (p *RPCPlugin) Init(*f4plugin.Host) ([]string, error) {
	return []string{"iOS"}, nil
}

// splitIOSPath breaks a flat RPC path into clean, non-empty segments. It
// accepts backslashes the same way plugins/cloudfox's splitConnPath does,
// since a Windows-built host may send either separator over the wire.
func splitIOSPath(raw string) []string {
	clean := strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if clean == "" || clean == "." {
		return nil
	}
	parts := strings.Split(clean, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func joinIOSRest(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	return "/" + strings.Join(segments, "/")
}

func (p *RPCPlugin) cachedMount(key string) (vfs.VFS, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	mounted, ok := p.mounts[key]
	return mounted, ok
}

// rememberMount records a newly opened mount under key, unless another call
// already won the race to open the same one first -- the same
// last-writer-loses rule plugins/cloudfox's RPCPlugin.resolve applies to a
// duplicate connection session.
func (p *RPCPlugin) rememberMount(key string, mounted vfs.VFS) vfs.VFS {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.mounts[key]; ok {
		return existing
	}
	p.mounts[key] = mounted
	return mounted
}

// resolve maps a flat RPC path onto the vfs.VFS that owns it, plus the
// remaining path relative to that VFS's own root. The walk has at most three
// fixed hops below the manager root: a device row, an optional capability
// row on the device's AFC root, and (for the Applications capability only)
// an application row.
func (p *RPCPlugin) resolve(ctx context.Context, raw string) (vfs.VFS, string, error) {
	segments := splitIOSPath(raw)
	if len(segments) == 0 {
		return p.manager, "", nil
	}

	deviceName := segments[0]
	// ManagerVFS keeps only its last completed snapshot. Refresh it before
	// every lookup so a newly connected or disconnected device is visible,
	// the same way a panel's own prior ReadDir keeps deviceProvider.CanOpen
	// current in the in-process build.
	if err := p.manager.ReadDir(ctx, "", nil); err != nil {
		return nil, "", err
	}
	device, found := p.manager.deviceForPath(deviceName)
	if !found {
		return nil, "", fmt.Errorf("ios: %q: %w", deviceName, os.ErrNotExist)
	}
	if !deviceReady(device) {
		return nil, "", fmt.Errorf("ios: device %q is %s: %w", device.UDID, device.State, ErrDeviceUnavailable)
	}

	key := deviceName
	current, ok := p.cachedMount(key)
	if !ok {
		opened, err := p.backend.OpenDevice(ctx, p.manager, device)
		if err != nil {
			return nil, "", err
		}
		current = p.rememberMount(key, opened)
	}
	rest := segments[1:]
	if len(rest) == 0 {
		return current, "", nil
	}

	afc, isAFC := current.(*AFCVFS)
	if !isAFC {
		return current, joinIOSRest(rest), nil
	}
	capability, isCapability := afc.rootCapabilities[rest[0]]
	if !isCapability {
		// A real Media path, not a virtual capability row.
		return current, joinIOSRest(rest), nil
	}
	key += "/" + rest[0]
	selection, selected := p.cachedMount(key)
	if !selected {
		opened, err := p.backend.OpenSelection(ctx, afc, device, capability)
		if err != nil {
			return nil, "", err
		}
		selection = p.rememberMount(key, opened)
	}
	current, rest = selection, rest[1:]

	if capability == CapabilityApplications && len(rest) > 0 {
		if apps, isApps := current.(*ApplicationsVFS); isApps {
			// Refresh the app snapshot the same way ApplicationProvider.CanOpen
			// relies on a prior ReadDir having populated apps.apps.
			if err := apps.ReadDir(ctx, "/", nil); err != nil {
				return nil, "", err
			}
			appRow := rest[0]
			app, appFound := apps.appForPath(appRow)
			if !appFound {
				return nil, "", fmt.Errorf("ios: application %q: %w", appRow, os.ErrNotExist)
			}
			key += "/" + appRow
			mounted, mountedFound := p.cachedMount(key)
			if !mountedFound {
				opened, err := p.backend.OpenApp(ctx, apps, device, app)
				if err != nil {
					return nil, "", err
				}
				mounted = p.rememberMount(key, opened)
			}
			current, rest = mounted, rest[1:]
		}
	}

	return current, joinIOSRest(rest), nil
}

func convertItem(item vfs.VFSItem) f4plugin.VFSItem {
	return f4plugin.VFSItem{
		KnownMetadata: uint32(item.KnownMetadata),
		SizeKnown:     item.SizeKnown,
		PhysicalSize:  item.PhysicalSize,
		ATime:         item.ATime,
		CTime:         item.CTime,
		UnixMode:      item.UnixMode,
		Uid:           item.Uid,
		Gid:           item.Gid,
		WinAttrs:      item.WinAttrs,
		NoExtension:   item.NoExtension,
		IsSymlink:     item.IsSymlink,
		ReparseTag:    item.ReparseTag,
		Name:          item.Name,
		Size:          item.Size,
		IsDir:         item.IsDir,
		MTime:         item.MTime,
		Mode:          item.Mode,
		IsExecutable:  item.IsExecutable,
		IsHidden:      item.IsHidden,
	}
}

func (p *RPCPlugin) ReadDir(drive, path string) ([]f4plugin.VFSItem, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return nil, err
	}
	var items []f4plugin.VFSItem
	err = target.ReadDir(ctx, rest, func(chunk []vfs.VFSItem) {
		for _, entry := range chunk {
			items = append(items, convertItem(entry))
		}
	})
	return items, err
}

func (p *RPCPlugin) Stat(drive, path string) (f4plugin.VFSItem, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return f4plugin.VFSItem{}, err
	}
	item, err := target.Stat(ctx, rest)
	if err != nil {
		return f4plugin.VFSItem{}, err
	}
	return convertItem(item), nil
}

func (p *RPCPlugin) Open(drive, path string) (uint32, int64, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return 0, 0, err
	}
	reader, err := target.Open(ctx, rest)
	if err != nil {
		return 0, 0, err
	}
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.readers[id] = reader
	p.mu.Unlock()
	return id, reader.Size(), nil
}

func (p *RPCPlugin) ReadAt(fileID uint32, length int, offset int64) ([]byte, error) {
	p.mu.Lock()
	reader, ok := p.readers[fileID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("ios: unknown file handle %d", fileID)
	}
	buf := make([]byte, length)
	n, err := reader.ReadAt(context.Background(), buf, offset)
	// RPC error responses cannot carry data. A short result is how the host
	// reconstructs EOF; returning EOF here would discard preview tail bytes.
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return buf[:n], err
}

func (p *RPCPlugin) Create(drive, path string) (uint32, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return 0, err
	}
	writer, err := target.Create(ctx, rest)
	if err != nil {
		return 0, err
	}
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.writers[id] = writer
	p.mu.Unlock()
	return id, nil
}

func (p *RPCPlugin) Write(fileID uint32, data []byte) error {
	p.mu.Lock()
	writer, ok := p.writers[fileID]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("ios: unknown file handle %d", fileID)
	}
	_, err := writer.Write(data)
	return err
}

func (p *RPCPlugin) CloseFile(fileID uint32) error {
	p.mu.Lock()
	reader, isReader := p.readers[fileID]
	delete(p.readers, fileID)
	writer, isWriter := p.writers[fileID]
	delete(p.writers, fileID)
	p.mu.Unlock()
	switch {
	case isReader:
		return reader.Close()
	case isWriter:
		return writer.Close()
	default:
		return fmt.Errorf("ios: unknown file handle %d", fileID)
	}
}

func (p *RPCPlugin) MkDir(drive, path string) error {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return err
	}
	return target.MkDir(ctx, rest)
}

func (p *RPCPlugin) Remove(drive, path string) error {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return err
	}
	return target.Remove(ctx, rest)
}

// Rename requires both paths to resolve into the same mounted VFS: iOS has
// no notion of moving a file between two different mounted domains (Media,
// an application container, ...), so this refuses that instead of silently
// operating on the wrong side.
func (p *RPCPlugin) Rename(drive, oldPath, newPath string) error {
	ctx := context.Background()
	target, oldRest, err := p.resolve(ctx, oldPath)
	if err != nil {
		return err
	}
	otherTarget, newRest, err := p.resolve(ctx, newPath)
	if err != nil {
		return err
	}
	if otherTarget != target {
		return fmt.Errorf("ios: cannot rename between different mounted domains: %q -> %q", oldPath, newPath)
	}
	return target.Rename(ctx, oldRest, newRest)
}

// Highlight, ProcessKey, OnHotkey and OnProgressTask complete the
// sdk/f4plugin.Plugin interface. iOS does not offer editor syntax
// highlighting, drive-scoped hotkeys or a progress-task callback over this
// transport in part 1; long-running transfers already report progress
// through f4's normal copy/move UI, which does not go through this plugin
// interface at all.
func (p *RPCPlugin) Highlight(line string, prev any, base uint64) ([]uint64, any, error) {
	return nil, nil, nil
}

func (p *RPCPlugin) ProcessKey(drive string, event vtinput.InputEvent) (bool, error) {
	return false, nil
}

func (p *RPCPlugin) OnHotkey(vk uint16, mods uint32) error { return nil }

func (p *RPCPlugin) OnProgressTask() error { return nil }

var _ f4plugin.Plugin = (*RPCPlugin)(nil)
