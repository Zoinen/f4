package androidfs

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

// RPCPlugin adapts the in-process Android VFS provider (Plugin, ManagerVFS,
// the hybrid FISH+/ADB Sync device opener -- none of it changed by this
// file) to the subprocess RPC transport described in docs/PLUGINS.md and
// implemented by sdk/f4plugin. It is plugins/android's half of f4#1178's
// Android part (plan: the owner's decision at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645 to give
// Android the same "download this plugin on demand" treatment as cloud
// storage/iOS), and mirrors plugins/cloudfox/rpc_plugin.go's shape as
// closely as the two plugins' different manager models allow.
//
// Unlike CloudFox, plugins/android carries no unique heavy dependency of its
// own -- it is a thin wrapper around the local `adb` server/executable (the
// same "wrap an external host tool" pattern as plugins/multiarc and
// netfox's FISH+), reusing plugins/netfox's FishVFS/fishplus machinery for
// the FISH+ backend rather than vendoring a copy. The one dependency worth
// flagging explicitly: because plugins/netfox is a shared, non-modularized
// package whose FTP/SFTP/SSH-dialer/Pageant files are only //go:build
// lite/!lite gated (not split into a separate module the way this file's
// own extraction splits plugins/android out), a plain `go build` of this
// module's binary would silently link golang.org/x/crypto/ssh,
// github.com/pkg/sftp, github.com/jlaffaye/ftp and github.com/kbolino/pageant
// even though nothing here ever references any of them. go.mod's own
// comment and the build-android-plugin CI job address that with -tags lite,
// which selects netfox's FISH+-only file set -- exactly what device.go and
// fish_pool.go actually use (netfox.FishVFS, netfox.fishplus; never
// netfox's FTP/SFTP VFS types or its SSH dialer). So the extraction here is
// primarily architectural, unifying the plugin install mechanism across
// cloud storage, iOS and Android per the owner's request, rather than a
// binary-size win the way CloudFox's ~30 MB of cloud SDKs was.
//
// ManagerVFS lists ADB devices (one row per serial, refreshed on every
// ReadDir from `adb devices`) and is not itself a two-tier "list of saved
// entries" the way CloudFox's manager is -- there is nothing here to
// add/edit/save, so unlike CloudFox this adapter has no missing-dialog gap
// to flag. Opening a device negotiates FISH+ or falls back to ADB Sync
// (device.go's hybridDeviceOpener, unchanged); this adapter caches the
// resulting session per device's display name, keyed the same way
// ManagerVFS's own map is, and forwards VFS calls into it exactly as
// CloudFox's RPCPlugin forwards into a cached CloudVFS session.
type RPCPlugin struct {
	uriRPCState
	plugin  *Plugin
	manager *ManagerVFS

	mu      sync.Mutex
	devices map[string]vfs.VFS
	readers map[uint32]vfs.ReadAtCloser
	writers map[uint32]io.WriteCloser
	nextID  uint32
}

// NewRPCPlugin builds the RPC-facing adapter around a fresh Android Plugin
// (device.go's NewPlugin: one shared ADB server client, FISH+ session pool
// and device-info cache).
func NewRPCPlugin() *RPCPlugin {
	plugin := NewPlugin()
	return &RPCPlugin{
		plugin:  plugin,
		manager: newManagerVFS(plugin.Source, plugin.Opener, plugin.info),
		devices: make(map[string]vfs.VFS),
		readers: make(map[uint32]vfs.ReadAtCloser),
		writers: make(map[uint32]io.WriteCloser),
	}
}

func (p *RPCPlugin) Init(*f4plugin.Host) ([]string, error) {
	return []string{"Android"}, nil
}

// splitDevicePath separates the leading path segment (the device's display
// name, exactly as ManagerVFS.ReadDir/DeviceDisplayName render it) from
// whatever follows it, without requiring that device to actually be online.
// Rename and Remove need this existence-free split so a bare device row
// never has its opened session's own root mutated (see those methods below);
// resolve needs the existence-checked, session-opening version for
// everything else.
func splitDevicePath(raw string) (name, rest string) {
	clean := strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if clean == "" || clean == "." {
		return "", ""
	}
	name, rest, _ = strings.Cut(clean, "/")
	return name, rest
}

// resolve maps a flat RPC path onto the vfs.VFS that owns it -- the device
// manager itself for the root, or a lazily opened and cached FISH+/ADB Sync
// session for a named, online device -- plus the remaining path to hand
// that VFS. It queries ADB's device list directly (plugin.Source), the same
// way CloudFox's resolve queries its repository directly, rather than
// through the shared ManagerVFS's own device-name cache: that cache only
// exists to serve ManagerVFS's in-process panel callers (Stat,
// PanelInfoKey, ...) and this adapter has its own, RPC-scoped session cache
// below.
func (p *RPCPlugin) resolve(ctx context.Context, raw string) (vfs.VFS, string, error) {
	name, rest := splitDevicePath(raw)
	if name == "" {
		return p.manager, "", nil
	}

	p.mu.Lock()
	if cached, ok := p.devices[name]; ok {
		p.mu.Unlock()
		return cached, rest, nil
	}
	p.mu.Unlock()

	if p.plugin.Source == nil {
		return nil, "", ErrNoDeviceSource
	}
	list, err := p.plugin.Source.ListDevices(ctx)
	if err != nil {
		return nil, "", err
	}
	var (
		device DeviceInfo
		found  bool
	)
	for _, candidate := range list {
		if DeviceDisplayName(candidate) == name {
			device, found = candidate, true
			break
		}
	}
	if !found {
		return nil, "", fmt.Errorf("android: %q: %w", name, os.ErrNotExist)
	}
	if device.State != DeviceStateOnline {
		return nil, "", fmt.Errorf("android: device %q is %s: %w", device.Serial, device.State, ErrDeviceUnavailable)
	}
	if p.plugin.Opener == nil {
		return nil, "", ErrNoDeviceOpener
	}

	opened, err := p.plugin.Opener.OpenDevice(ctx, p.manager, device)
	if err != nil {
		return nil, "", err
	}

	p.mu.Lock()
	if cached, ok := p.devices[name]; ok {
		// Lost a race with another RPC call opening the same device. Keep
		// the winner, close the redundant session.
		p.mu.Unlock()
		_ = opened.Close()
		return cached, rest, nil
	}
	p.devices[name] = opened
	p.mu.Unlock()
	return opened, rest, nil
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
		return nil, fmt.Errorf("android: unknown file handle %d", fileID)
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
		return fmt.Errorf("android: unknown file handle %d", fileID)
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
		return fmt.Errorf("android: unknown file handle %d", fileID)
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

// Remove deletes a file or folder on an opened device, or -- for a bare,
// one-segment path -- delegates to the device manager itself
// (ManagerVFS.Remove), exactly like selecting a device row in the full
// build and pressing F8: ManagerVFS unconditionally refuses (devices come
// from `adb devices`, not from anything f4 saves or owns), which matters
// here because routing a one-segment path into the opened FISH+/Sync
// session instead would attempt to remove the device's own filesystem
// root -- a materially different and far more dangerous operation that the
// full build never allows either.
func (p *RPCPlugin) Remove(drive, path string) error {
	ctx := context.Background()
	name, rest := splitDevicePath(path)
	if rest == "" {
		return p.manager.Remove(ctx, name)
	}
	target, deviceRest, err := p.resolve(ctx, path)
	if err != nil {
		return err
	}
	return target.Remove(ctx, deviceRest)
}

// Rename follows the same one-segment-means-the-manager rule as Remove: a
// bare source name asks the device manager (ManagerVFS.Rename), which
// always refuses, rather than renaming the device's own filesystem root.
func (p *RPCPlugin) Rename(drive, oldPath, newPath string) error {
	ctx := context.Background()
	oldName, oldRest := splitDevicePath(oldPath)
	if oldRest == "" {
		newName, _ := splitDevicePath(newPath)
		return p.manager.Rename(ctx, oldName, newName)
	}
	newName, newRest := splitDevicePath(newPath)
	if newName != oldName || newRest == "" {
		return fmt.Errorf("android: cannot rename outside the source device: %q -> %q", oldPath, newPath)
	}
	target, deviceOldRest, err := p.resolve(ctx, oldPath)
	if err != nil {
		return err
	}
	return target.Rename(ctx, deviceOldRest, newRest)
}

// Highlight, ProcessKey, OnHotkey and OnProgressTask complete the
// sdk/f4plugin.Plugin interface. The Android drive does not offer editor
// syntax highlighting, drive-scoped hotkeys or a progress-task callback
// over this transport in part 1; long-running transfers already report
// progress through f4's normal copy/move UI, which does not go through this
// plugin interface at all.
func (p *RPCPlugin) Highlight(line string, prev any, base uint64) ([]uint64, any, error) {
	return nil, nil, nil
}

func (p *RPCPlugin) ProcessKey(drive string, event vtinput.InputEvent) (bool, error) {
	return false, nil
}

func (p *RPCPlugin) OnHotkey(vk uint16, mods uint32) error { return nil }

func (p *RPCPlugin) OnProgressTask() error { return nil }

var _ f4plugin.Plugin = (*RPCPlugin)(nil)
