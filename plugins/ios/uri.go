package iosfs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/unxed/f4/vfs"
)

type uriProvider struct {
	source DeviceSource
	opener DeviceOpener
}

func (*uriProvider) Scheme() string { return "ios" }

func (p *uriProvider) OpenURI(ctx context.Context, _ vfs.VFS, raw string) (vfs.VFS, error) {
	manager := NewManagerVFS(p.source, p.opener)
	if strings.EqualFold(raw, iosRoot) {
		return manager, nil
	}
	scheme, name, remote, err := vfs.ParseDevicePath(raw)
	if err != nil || scheme != "ios" {
		return nil, fmt.Errorf("ios: invalid device address")
	}
	if err := manager.ReadDir(ctx, manager.GetPath(), nil); err != nil {
		return nil, err
	}
	device, err := manager.deviceForURI(name)
	if err != nil {
		return nil, err
	}
	if !deviceReady(device) {
		return nil, ErrDeviceUnavailable
	}
	if p.opener == nil {
		return nil, ErrNoDeviceOpener
	}
	mounted, err := p.opener.OpenDevice(ctx, manager, device)
	if err != nil {
		return nil, err
	}
	owned := []vfs.VFS{manager, mounted}
	defer func() {
		if err != nil {
			for i := len(owned) - 1; i >= 0; i-- {
				_ = owned[i].Close()
			}
		}
	}()
	for _, part := range strings.Split(strings.Trim(remote, "/"), "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			err = fmt.Errorf("ios: device URI cannot traverse export boundaries")
			return nil, err
		}
		var next vfs.VFS
		next, err = openURIChild(ctx, mounted, part)
		if err != nil {
			return nil, err
		}
		if next != mounted {
			owned = append(owned, next)
			mounted = next
		}
	}
	if os.Getenv("VTUI_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, "[FIX:device-path] reopened iOS device-qualified directory")
	}
	return mounted, nil
}

func (m *ManagerVFS) deviceForURI(name string) (DeviceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found DeviceInfo
	count := 0
	for _, device := range m.devices {
		if name == device.pathName || name == device.UDID {
			found = device
			count++
		}
	}
	if count != 1 {
		return DeviceInfo{}, fmt.Errorf("ios: device name is unavailable or ambiguous: %w", os.ErrNotExist)
	}
	return found, nil
}

func openURIChild(ctx context.Context, mounted vfs.VFS, name string) (vfs.VFS, error) {
	var provider vfs.VFSProvider
	switch mounted.(type) {
	case *AFCVFS, *DeviceRootVFS:
		provider = &SelectorProvider{}
	case *ApplicationsVFS:
		provider = &ApplicationProvider{}
	case *AppGroupsVFS:
		provider = &GroupProvider{}
	}
	if _, ok := mounted.(*ApplicationsVFS); ok {
		if err := mounted.ReadDir(ctx, mounted.GetPath(), nil); err != nil {
			return nil, err
		}
	}
	target := mounted.Join(mounted.GetPath(), name)
	if provider != nil && provider.CanOpen(ctx, mounted, target) {
		return provider.Open(ctx, mounted, target)
	}
	item, err := mounted.Stat(ctx, target)
	if err != nil {
		return nil, err
	}
	if !item.IsDir {
		return nil, fmt.Errorf("ios: URI target is not a directory")
	}
	if setter, ok := mounted.(vfs.OptimisticPathSetter); ok {
		err = setter.SetPathOptimistic(target)
	} else {
		err = mounted.SetPath(target)
	}
	return mounted, err
}
