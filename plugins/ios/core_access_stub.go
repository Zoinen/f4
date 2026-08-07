//go:build !darwin && !linux && !windows

package iosfs

import (
	"context"
	"fmt"
)

type unsupportedCoreAccess struct{}

func newCoreAccess() coreAccess { return unsupportedCoreAccess{} }
func coreAccessSupported() bool { return false }

func (unsupportedCoreAccess) Open(context.Context, DeviceInfo, coreDomain, string) (coreFileService, error) {
	return nil, fmt.Errorf("%w on this operating system", ErrCoreDeviceUnavailable)
}
func (unsupportedCoreAccess) Close() error { return nil }

var _ coreAccess = unsupportedCoreAccess{}
