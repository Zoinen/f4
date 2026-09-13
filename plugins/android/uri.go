package androidfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/unxed/f4/vfs"
)

// qualifyDevices assigns friendly authorities without using discovery order.
// Duplicate models include the serial; resolution still rejects collisions
// with a device whose literal model happens to equal a generated authority.
func qualifyDevices(devices []DeviceInfo) []DeviceInfo {
	counts := make(map[string]int, len(devices))
	for _, device := range devices {
		if strings.TrimSpace(device.Serial) != "" {
			counts[deviceSessionTitle(device)]++
		}
	}
	qualified := append([]DeviceInfo{}, devices...)
	for i := range qualified {
		device := &qualified[i]
		name := deviceSessionTitle(*device)
		if counts[name] > 1 {
			name += " (" + device.Serial + ")"
		}
		device.publicName = name
	}
	return qualified
}

type androidURIProvider struct{ plugin *Plugin }

func (*androidURIProvider) Scheme() string { return "android" }

func (p *androidURIProvider) OpenURI(ctx context.Context, _ vfs.VFS, raw string) (vfs.VFS, error) {
	manager := newManagerVFS(p.plugin.Source, p.plugin.Opener, p.plugin.info)
	if strings.EqualFold(raw, androidRoot) {
		return manager, nil
	}
	scheme, name, remote, err := vfs.ParseDevicePath(raw)
	if err != nil {
		return nil, fmt.Errorf("android: parse URI: %w", err)
	}
	if scheme != "android" {
		return nil, fmt.Errorf("android: foreign URI scheme %q", scheme)
	}
	if manager.source == nil {
		return nil, ErrNoDeviceSource
	}
	devices, err := manager.source.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]DeviceInfo, 0, 1)
	for _, device := range qualifyDevices(devices) {
		if strings.TrimSpace(device.Serial) == "" {
			continue
		}
		// Serial and old manager-row addresses remain usable after reconnects.
		base := device
		base.publicName = ""
		canonical := name == device.publicName
		alias := name == device.Serial || name == DeviceDisplayName(device) || name == deviceSessionTitle(base)
		if canonical || alias {
			matches = append(matches, device)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("android: device %q: %w", name, os.ErrNotExist)
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("android: ambiguous device name %q", name)
	}
	device := matches[0]
	if device.State != DeviceStateOnline {
		return nil, fmt.Errorf("android: device %q is %s: %w", device.Serial, device.State, ErrDeviceUnavailable)
	}
	if manager.opener == nil {
		return nil, ErrNoDeviceOpener
	}
	mounted, err := manager.opener.OpenDevice(ctx, manager, device)
	if err != nil {
		return nil, err
	}
	// Resolve files to their containing directory so the returned mount can
	// reopen the original item URI with Open/Stat as well as navigate folders.
	item, err := mounted.Stat(ctx, remote)
	if err == nil {
		if !item.IsDir {
			remote = mounted.Dir(remote)
		}
		err = mounted.SetPath(remote)
	}
	if err != nil {
		return nil, errors.Join(err, mounted.Close())
	}
	return mounted, nil
}

var _ vfs.URIProvider = (*androidURIProvider)(nil)
