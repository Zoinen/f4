package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
)

// Plugin registers one self-contained iOS drive. The Apple platform service
// (usbmuxd / Apple Mobile Device Support) is the only runtime prerequisite;
// f4 never shells out to devicectl, go-ios, ifuse or idevice tools.
type Plugin struct {
	backend *nativeBackend
}

func NewPlugin() *Plugin {
	return &Plugin{backend: &nativeBackend{
		apps: nativeAppSource{}, afc: newAFCRegistry(), core: newCoreAccess(),
	}}
}

func (p *Plugin) Init(api vfs.HostAPI) error {
	if p.backend == nil {
		return errors.New("ios: plugin backend is not configured")
	}
	api.RegisterVFSProvider(&deviceProvider{})
	api.RegisterVFSProvider(&SelectorProvider{})
	api.RegisterVFSProvider(&ApplicationProvider{})
	api.RegisterVFSProvider(&GroupProvider{})
	api.RegisterDrive("iOS", func() vfs.VFS {
		return NewManagerVFS(nativeDeviceSource{}, p.backend)
	})
	return nil
}

func (p *Plugin) Close() error {
	if p.backend == nil {
		return nil
	}
	return p.backend.Close()
}

func (*Plugin) GetName() string { return "iOS" }

type nativeBackend struct {
	apps nativeAppSource
	afc  *afcRegistry
	core coreAccess
}

func (b *nativeBackend) Close() error {
	return errors.Join(b.afc.Close(), b.core.Close())
}

func (b *nativeBackend) OpenDevice(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	return b.openDeviceRoot(ctx, parent, device)
}

func (b *nativeBackend) openDeviceRoot(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	if !deviceReady(device) {
		return nil, fmt.Errorf("ios: device %q is not ready: %w", device.UDID, ErrDeviceUnavailable)
	}
	mounted, err := b.openMedia(ctx, parent, device)
	if err != nil {
		return nil, err
	}
	media, ok := mounted.(*AFCVFS)
	if !ok {
		_ = mounted.Close()
		return nil, fmt.Errorf("ios: Media root returned unsupported filesystem %T", mounted)
	}
	media.SetVirtualRoot(b, b.availableRootCapabilities(ctx, device)...)
	return media, nil
}

const iosCapabilityProbeTimeout = 3 * time.Second

type capabilityProbeResult struct {
	capability Capability
	available  bool
}

func (b *nativeBackend) availableRootCapabilities(ctx context.Context, device DeviceInfo) []Capability {
	probeCtx, cancel := context.WithTimeout(ctx, iosCapabilityProbeTimeout)
	defer cancel()
	results := make(chan capabilityProbeResult, 2)

	go func() {
		_, err := b.apps.ListApps(probeCtx, device)
		results <- capabilityProbeResult{capability: CapabilityApplications, available: err == nil}
	}()
	go func() {
		entry, err := resolveNativeDevice(probeCtx, device)
		if err == nil {
			var connection io.ReadWriteCloser
			connection, err = openCrashReportService(probeCtx, entry)
			if connection != nil {
				_ = connection.Close()
			}
		}
		results <- capabilityProbeResult{capability: CapabilityCrashReports, available: err == nil}
	}()

	available := make([]Capability, 0, 2)
	for range 2 {
		select {
		case result := <-results:
			if result.available {
				available = append(available, result.capability)
			}
		case <-probeCtx.Done():
			return available
		}
	}
	return available
}

func (b *nativeBackend) OpenSelection(ctx context.Context, parent vfs.VFS, device DeviceInfo, capability Capability) (vfs.VFS, error) {
	switch capability {
	case CapabilityMedia:
		return b.openMedia(ctx, parent, device)
	case CapabilityApplications:
		return NewApplicationsVFS(parent, device, b.apps, b), nil
	case CapabilityAppGroups:
		apps, err := b.apps.ListApps(ctx, device)
		if err != nil {
			return nil, err
		}
		groups := make([]string, 0)
		for _, app := range apps {
			groups = append(groups, app.GroupIDs...)
		}
		return NewAppGroupsVFS(parent, device, groups, b), nil
	case CapabilityCrashReports:
		return b.openCrashReports(ctx, parent, device)
	default:
		return nil, fmt.Errorf("ios: unknown capability %d", capability)
	}
}

func (b *nativeBackend) OpenApp(ctx context.Context, parent vfs.VFS, device DeviceInfo, app AppInfo) (vfs.VFS, error) {
	entry, err := resolveAppDevice(ctx, device)
	if err != nil {
		return nil, err
	}
	commands := make([]string, 0, 2)
	add := func(command string) {
		for _, current := range commands {
			if current == command {
				return
			}
		}
		commands = append(commands, command)
	}
	if app.DeveloperSigned {
		add(vendContainer)
	}
	if app.FileSharing {
		add(vendDocuments)
	}
	// Metadata is a hint, not an authorization decision. Probe both commands
	// on explicit user entry so a new iOS policy does not silently hide data.
	add(vendContainer)
	add(vendDocuments)

	var afcErr error
	for _, command := range commands {
		command := command
		key := strings.Join([]string{device.UDID, "app", app.BundleID, command}, ":")
		title := iosDeviceTitle(device, ApplicationsSelector, AppDisplayName(app))
		mounted, openErr := openAFCVFS(ctx, parent, device, b.afc, key, title, false, func(ctx context.Context) (io.ReadWriteCloser, error) {
			return openHouseArrest(ctx, entry, app.BundleID, command)
		})
		if openErr == nil {
			return mounted, nil
		}
		afcErr = errors.Join(afcErr, openErr)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	service, coreErr := b.core.Open(ctx, device, coreDomainAppData, app.BundleID)
	if coreErr == nil {
		return newCoreVFS(parent, device, coreDomainAppData, app.BundleID,
			iosDeviceTitle(device, ApplicationsSelector, AppDisplayName(app)), service, b.core), nil
	}
	return nil, fmt.Errorf("ios: application %s is not exported by iOS (House Arrest: %v; CoreDevice: %w)", app.BundleID, afcErr, coreErr)
}

func (b *nativeBackend) OpenGroup(ctx context.Context, parent vfs.VFS, device DeviceInfo, groupID string) (vfs.VFS, error) {
	service, err := b.core.Open(ctx, device, coreDomainAppGroup, groupID)
	if err != nil {
		return nil, fmt.Errorf("ios: open app group %s: %w", groupID, err)
	}
	return newCoreVFS(parent, device, coreDomainAppGroup, groupID,
		iosDeviceTitle(device, AppGroupsSelector, virtualRowName(groupID)), service, b.core), nil
}

func (b *nativeBackend) openMedia(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	entry, err := resolveNativeDevice(ctx, device)
	if err != nil {
		return nil, err
	}
	key := device.UDID + ":media"
	return openAFCVFS(ctx, parent, device, b.afc, key, iosDeviceTitle(device), false,
		func(ctx context.Context) (io.ReadWriteCloser, error) {
			return connectService(ctx, entry, afcServiceName)
		})
}

func (b *nativeBackend) openCrashReports(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	entry, err := resolveNativeDevice(ctx, device)
	if err != nil {
		return nil, err
	}
	key := device.UDID + ":crash-reports"
	mounted, afcErr := openAFCVFS(ctx, parent, device, b.afc, key,
		iosDeviceTitle(device, CrashReportsSelector), true,
		func(ctx context.Context) (io.ReadWriteCloser, error) {
			return openCrashReportService(ctx, entry)
		})
	if afcErr == nil {
		return mounted, nil
	}
	service, coreErr := b.core.Open(ctx, device, coreDomainCrashReports, "")
	if coreErr == nil {
		return newCoreVFS(parent, device, coreDomainCrashReports, "",
			iosDeviceTitle(device, CrashReportsSelector), service, b.core), nil
	}
	return nil, fmt.Errorf("ios: open crash reports (classic: %v; CoreDevice: %w)", afcErr, coreErr)
}

var (
	_ DeviceOpener   = (*nativeBackend)(nil)
	_ SelectorOpener = (*nativeBackend)(nil)
	_ AppOpener      = (*nativeBackend)(nil)
	_ GroupOpener    = (*nativeBackend)(nil)
)
