//go:build darwin || linux || windows

package iosfs

import (
	"context"
	"fmt"
	"io"
	_ "unsafe" // required by go:linkname

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/tunnel"
)

const coreDeviceProxyService = "com.apple.internal.devicecompute.CoreDeviceProxy"

// goIOSConnectUserspaceTunnel is the connection-owning implementation behind
// go-ios' public ConnectUserSpaceTunnelLockdown. The public wrapper does not
// close the usbmux connection when tunnel negotiation fails. f4 opens it
// explicitly so every failure path remains leak-free. go-ios is pinned to an
// exact commit, and cross-build tests guard this narrow ABI bridge.
//
//go:linkname goIOSConnectUserspaceTunnel github.com/danielpaulus/go-ios/ios/tunnel.connectToUserspaceTunnelLockdown
func goIOSConnectUserspaceTunnel(context.Context, goios.DeviceEntry, io.ReadWriteCloser, int) (tunnel.Tunnel, error)

func connectCoreUserspaceTunnel(ctx context.Context, device goios.DeviceEntry, port int) (tunnel.Tunnel, error) {
	connection, err := goios.ConnectToService(device, coreDeviceProxyService)
	if err != nil {
		return tunnel.Tunnel{}, err
	}
	tun, err := goIOSConnectUserspaceTunnel(ctx, device, connection, port)
	if err != nil {
		_ = connection.Close()
		return tunnel.Tunnel{}, fmt.Errorf("negotiate userspace tunnel: %w", err)
	}
	return tun, nil
}
