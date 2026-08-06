package androidfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/plugins/netfox/fishplus"
	"github.com/unxed/f4/vfs"
)

const (
	fishHandshakeTimeout       = 12 * time.Second
	base64BootstrapFastTimeout = 4 * time.Second
)

type serverDeviceSource struct{ server *Server }

func (s serverDeviceSource) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	devices, err := s.server.Devices(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]DeviceInfo, 0, len(devices))
	for _, device := range devices {
		result = append(result, DeviceInfo{
			Serial:      device.Serial,
			State:       device.State,
			Model:       device.Model,
			Product:     device.Product,
			Device:      device.Device,
			TransportID: device.TransportID,
		})
	}
	return result, nil
}

type hybridDeviceOpener struct {
	features func(context.Context, string) (map[string]bool, error)
	openFish func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error)
	openSync func(context.Context, vfs.VFS, DeviceInfo, map[string]bool) (vfs.VFS, error)
	pool     *fishSessionPool
	info     *deviceInfoService
}

func (o *hybridDeviceOpener) OpenDevice(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
	if device.State != DeviceStateOnline {
		return nil, fmt.Errorf("android: device %q is %s: %w", device.Serial, device.State, ErrDeviceUnavailable)
	}
	features, err := o.features(ctx, device.Serial)
	if err != nil {
		return nil, fmt.Errorf("android: query features for %q: %w", device.Serial, err)
	}

	var fishErr error
	if features["shell_v2"] && o.openFish != nil {
		var mounted vfs.VFS
		var err error
		if o.pool != nil {
			mounted, err = o.pool.Open(ctx, parent, device, o.openFish)
		} else {
			mounted, err = o.openFish(ctx, parent, device)
		}
		if err == nil {
			return attachAndroidPanelInfo(mounted, o.info.provider(device, "FISH+", fishBackendDetail(mounted))), nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		fishErr = err
	}

	if o.openSync == nil {
		if fishErr != nil {
			return nil, fishErr
		}
		return nil, errors.New("android: ADB Sync backend is unavailable")
	}
	mounted, syncErr := o.openSync(ctx, parent, device, features)
	if syncErr == nil {
		return attachAndroidPanelInfo(mounted, o.info.provider(device, "ADB Sync", syncBackendDetail(features))), nil
	}
	if fishErr == nil {
		return nil, syncErr
	}
	return nil, fmt.Errorf("android: neither backend could open %q (FISH+: %v; ADB Sync: %w)", device.Serial, fishErr, syncErr)
}

type panelInfoSetter interface {
	SetPanelInfoProvider(vfs.PanelInfoProvider)
}

func attachAndroidPanelInfo(mounted vfs.VFS, provider vfs.PanelInfoProvider) vfs.VFS {
	if provider != nil {
		if setter, ok := mounted.(panelInfoSetter); ok {
			setter.SetPanelInfoProvider(provider)
		}
	}
	return mounted
}

func fishBackendDetail(mounted vfs.VFS) string {
	fish, ok := mounted.(*netfox.FishVFS)
	if !ok || fish.Client() == nil {
		return ""
	}
	features := fish.Client().Session().Features()
	return fmt.Sprintf("list=%s, read=%s, write=%s", features.ListingMode(), features.ReadMode(), features.WriteMode())
}

func deviceSessionTitle(device DeviceInfo) string {
	if model := strings.TrimSpace(device.Model); model != "" {
		return model
	}
	return strings.TrimSpace(device.Serial)
}

func androidPanelTitle(title, canonicalPath string) string {
	displayPath := strings.TrimPrefix(strings.TrimSpace(canonicalPath), "/")
	displayPath = strings.ReplaceAll(displayPath, "/", `\`)
	return title + `:\` + displayPath
}

func openFishDevice(ctx context.Context, parent vfs.VFS, server *Server, device DeviceInfo) (vfs.VFS, error) {
	openCtx, cancel := context.WithTimeout(ctx, fishHandshakeTimeout)
	defer cancel()

	// Android's shell is dramatically faster when it receives the complete
	// helper as one base64 line. Give that path a bounded head start; a device
	// without a suitable decoder gets a fresh shell and the portable streaming
	// bootstrap still has most of the overall handshake budget available.
	fastCtx, fastCancel := context.WithTimeout(openCtx, base64BootstrapFastTimeout)
	fastFish, fastErr := openFishDeviceAttempt(fastCtx, parent, server, device, fishplus.HandshakeOptions{
		Bootstrap: fishplus.BootstrapBase64Line,
	})
	fastCancel()
	if fastErr == nil {
		return validateAndroidFish(fastFish)
	}
	if err := openCtx.Err(); err != nil {
		return nil, err
	}

	legacyFish, legacyErr := openFishDeviceAttempt(openCtx, parent, server, device, fishplus.HandshakeOptions{})
	if legacyErr == nil {
		return validateAndroidFish(legacyFish)
	}
	return nil, fmt.Errorf("android: FISH+ helper bootstrap failed (base64: %v; line: %w)", fastErr, legacyErr)
}

func openFishDeviceAttempt(ctx context.Context, parent vfs.VFS, server *Server, device DeviceInfo, opts fishplus.HandshakeOptions) (*netfox.FishVFS, error) {
	// API 24 and later advertises shell_v2. Its raw mode is a binary-clean,
	// full-duplex stream, which is exactly the transport-independent contract
	// FISH+ already uses over SSH.
	shell, err := server.OpenShellV2(ctx, device.Serial, "exec /system/bin/sh")
	if err != nil {
		return nil, err
	}
	stopInterrupt := shell.InterruptOnCancel(ctx)
	defer stopInterrupt()

	fish, err := netfox.NewFishVFSOnStreamWithOptions(ctx, parent, shell, shell, shell, deviceSessionTitle(device), opts)
	if err != nil {
		exitCode, hasExitCode := shell.ExitCode()
		stderr := shell.Stderr()
		if hasExitCode || len(stderr) != 0 {
			return nil, fmt.Errorf("android: FISH+ shell failed (exit=%d present=%t stderr=%q): %w", exitCode, hasExitCode, stderr, err)
		}
		return nil, err
	}
	fish.SetPanelTitleFormatter(androidPanelTitle)
	return fish, nil
}

func validateAndroidFish(fish *netfox.FishVFS) (vfs.VFS, error) {
	features := fish.Client().Session().Features()
	// The "cat" fallback cannot serve byte ranges, while vfs.Open promises
	// ReadAt. Sync can, by materializing once, so prefer it over a FISH+ session
	// that would open successfully but fail as soon as the viewer seeks.
	if features.ListingMode() == "" || features.ReadMode() == "" || features.ReadMode() == "cat" || features.WriteMode() == "" {
		_ = fish.Close()
		return nil, fmt.Errorf("android: FISH+ helper lacks a complete filesystem backend (list=%q read=%q write=%q)",
			features.ListingMode(), features.ReadMode(), features.WriteMode())
	}
	return fish, nil
}

func openSyncDevice(ctx context.Context, parent vfs.VFS, server *Server, device DeviceInfo, features map[string]bool) (vfs.VFS, error) {
	client := NewSyncClient(server, device.Serial, features)
	// Probe now, while provider.Open can still report a useful error. Otherwise
	// a dead Sync service would produce an apparently mounted but empty panel.
	root, err := client.Stat(ctx, "/")
	if err != nil {
		return nil, fmt.Errorf("android: probe ADB Sync on %q: %w", device.Serial, err)
	}
	if root.Mode&remoteModeType != remoteModeDir {
		return nil, fmt.Errorf("android: ADB Sync returned a non-directory device root")
	}
	run := func(ctx context.Context, serial, command string) (shellResult, error) {
		stdout, stderr, exitCode, err := server.RunShell(ctx, serial, command)
		return shellResult{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}, err
	}
	return newSyncVFS(parent, device.Serial, deviceSessionTitle(device), syncClientFS{client: client}, run), nil
}

// NewPlugin constructs the built-in Android drive around one shared ADB server
// client. Discovery is refreshed on demand; one validated FISH+ shell per
// serial is retained for repeat entry, while Sync opens one service per
// operation.
func NewPlugin() *Plugin {
	server := NewServer()
	pool := newFishSessionPool()
	info := newDeviceInfoService(server)
	opener := &hybridDeviceOpener{
		features: server.Features,
		pool:     pool,
		info:     info,
		openFish: func(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
			return openFishDevice(ctx, parent, server, device)
		},
		openSync: func(ctx context.Context, parent vfs.VFS, device DeviceInfo, features map[string]bool) (vfs.VFS, error) {
			return openSyncDevice(ctx, parent, server, device, features)
		},
	}
	return &Plugin{Source: serverDeviceSource{server: server}, Opener: opener, closer: pool, info: info}
}

var _ DeviceSource = serverDeviceSource{}
var _ DeviceOpener = (*hybridDeviceOpener)(nil)
var _ io.ReadWriteCloser = (*ShellStream)(nil)
