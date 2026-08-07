package iosfs

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/Masterminds/semver"
)

var (
	ErrCoreDeviceUnavailable = errors.New("ios: CoreDevice file service is unavailable")
	ErrCoreDeviceConnection  = errors.New("ios: CoreDevice connection was lost")
	ErrReadOnlyDomain        = errors.New("ios: this device domain is read-only")
)

type coreDomain uint8

const (
	coreDomainAppData coreDomain = iota + 1
	coreDomainAppGroup
	coreDomainCrashReports
)

type coreEntry struct {
	Name    string
	Size    int64
	IsDir   bool
	IsLink  bool
	Hidden  bool
	Mode    uint32
	ModUnix int64
}

type coreFileService interface {
	List(context.Context, string) ([]coreEntry, error)
	Pull(context.Context, string, io.Writer) error
	Close() error
}

type coreAccess interface {
	Open(context.Context, DeviceInfo, coreDomain, string) (coreFileService, error)
	Close() error
}

var minimumCoreDeviceVersion = semver.MustParse("17.4.0")

func parsedIOSVersion(raw string) (*semver.Version, bool) {
	version, err := semver.NewVersion(strings.TrimSpace(raw))
	return version, err == nil
}

func coreDeviceVersionAvailable(raw string) bool {
	version, ok := parsedIOSVersion(raw)
	return ok && !version.LessThan(minimumCoreDeviceVersion) && !coreDevicePathRegressionVersion(version)
}

func coreDevicePathRegression(raw string) bool {
	version, ok := parsedIOSVersion(raw)
	return ok && coreDevicePathRegressionVersion(version)
}

func coreDevicePathRegressionVersion(version *semver.Version) bool {
	return version.Major() == 26 && version.Minor() == 5
}
