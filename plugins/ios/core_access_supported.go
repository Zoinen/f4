//go:build darwin || linux || windows

package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver"
	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/imagemounter"
	"github.com/danielpaulus/go-ios/ios/tunnel"
	"github.com/unxed/f4/plugins/ios/internal/corefileservice"
)

type coreTunnelSession struct {
	device goios.DeviceEntry
	tunnel tunnel.Tunnel
}

type coreTunnelDial struct {
	done    chan struct{}
	session *coreTunnelSession
	err     error
}

type nativeCoreAccess struct {
	mu        sync.Mutex
	sessions  map[string]*coreTunnelSession
	dials     map[string]*coreTunnelDial
	lifecycle context.Context
	cancel    context.CancelFunc
	build     func(context.Context, DeviceInfo) (*coreTunnelSession, error)
	builds    sync.WaitGroup
	closeDone chan struct{}
	closeErr  error
	closed    bool
}

func newCoreAccess() coreAccess {
	lifecycle, cancel := context.WithCancel(context.Background())
	return &nativeCoreAccess{
		sessions:  make(map[string]*coreTunnelSession),
		dials:     make(map[string]*coreTunnelDial),
		lifecycle: lifecycle,
		cancel:    cancel,
		build:     buildCoreTunnelSession,
		closeDone: make(chan struct{}),
	}
}

func coreAccessSupported() bool { return true }

func (a *nativeCoreAccess) Open(ctx context.Context, device DeviceInfo, domain coreDomain, identifier string) (coreFileService, error) {
	if ctx == nil {
		return nil, fmt.Errorf("ios: open CoreDevice service: %w", errors.ErrUnsupported)
	}
	if coreDevicePathRegression(device.OSVersion) && (domain == coreDomainAppData || domain == coreDomainAppGroup) {
		return nil, fmt.Errorf("%w: iOS %s rejects CoreDevice application-container paths", ErrCoreDeviceUnavailable, device.OSVersion)
	}
	session, err := a.session(ctx, device)
	if err != nil {
		return nil, err
	}
	fsDomain, err := nativeFileServiceDomain(domain)
	if err != nil {
		return nil, err
	}

	type openResult struct {
		connection *corefileservice.Connection
		err        error
	}
	result := make(chan openResult, 1)
	go func() {
		connection, openErr := corefileservice.New(session.device, fsDomain, identifier)
		result <- openResult{connection: connection, err: openErr}
	}()

	openCtx, cancel := context.WithTimeout(ctx, coreServiceOpenTimeout)
	defer cancel()
	select {
	case opened := <-result:
		if opened.err != nil {
			if coreTransportFailure(opened.err) {
				a.invalidateSession(device.UDID, session)
				return nil, fmt.Errorf("%w: open CoreDevice domain %q: %v", ErrCoreDeviceConnection, identifier, opened.err)
			}
			return nil, fmt.Errorf("ios: open CoreDevice domain %q: %w", identifier, opened.err)
		}
		service := &nativeCoreFileService{
			connection: opened.connection,
			abort:      func() { a.invalidateSession(device.UDID, session) },
		}
		return service, nil
	case <-openCtx.Done():
		a.invalidateSession(device.UDID, session)
		go func() {
			opened := <-result
			if opened.connection != nil {
				_ = opened.connection.Close()
			}
		}()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: opening CoreDevice domain %q timed out", ErrCoreDeviceConnection, identifier)
	}
}

func nativeFileServiceDomain(domain coreDomain) (corefileservice.Domain, error) {
	switch domain {
	case coreDomainAppData:
		return corefileservice.DomainAppDataContainer, nil
	case coreDomainAppGroup:
		return corefileservice.DomainAppGroupDataContainer, nil
	case coreDomainCrashReports:
		return corefileservice.DomainSystemCrashLogs, nil
	default:
		return 0, fmt.Errorf("ios: unknown CoreDevice domain %d", domain)
	}
}

func (a *nativeCoreAccess) session(ctx context.Context, info DeviceInfo) (*coreTunnelSession, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, ErrCoreDeviceUnavailable
	}
	if existing := a.sessions[info.UDID]; existing != nil {
		a.mu.Unlock()
		return existing, nil
	}
	dial := a.dials[info.UDID]
	if dial == nil {
		dial = &coreTunnelDial{done: make(chan struct{})}
		a.dials[info.UDID] = dial
		a.builds.Add(1)
		go a.buildSession(a.lifecycle, info, dial)
	}
	a.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-dial.done:
		if dial.err != nil {
			return nil, dial.err
		}
		if dial.session == nil {
			return nil, ErrCoreDeviceUnavailable
		}
		return dial.session, nil
	}
}

func (a *nativeCoreAccess) buildSession(ctx context.Context, info DeviceInfo, dial *coreTunnelDial) {
	defer a.builds.Done()
	session, err := a.build(ctx, info)

	var discard *coreTunnelSession
	a.mu.Lock()
	delete(a.dials, info.UDID)
	switch {
	case a.closed:
		dial.err = ErrCoreDeviceUnavailable
		discard = session
	case err != nil:
		dial.err = err
	case a.sessions[info.UDID] != nil:
		dial.session = a.sessions[info.UDID]
		discard = session
	default:
		a.sessions[info.UDID] = session
		dial.session = session
	}
	close(dial.done)
	a.mu.Unlock()
	if discard != nil {
		_ = discard.tunnel.Close()
	}
}

func buildCoreTunnelSession(ctx context.Context, info DeviceInfo) (*coreTunnelSession, error) {
	device, err := resolveNativeDevice(ctx, info)
	if err != nil {
		return nil, err
	}
	version, err := goios.GetProductVersion(device)
	if err != nil {
		return nil, fmt.Errorf("ios: determine version for CoreDevice: %w", err)
	}
	if version.LessThan(minimumCoreDeviceVersion) {
		return nil, fmt.Errorf("%w: userspace tunnel requires iOS 17.4 or newer (device has %s)", ErrCoreDeviceUnavailable, version)
	}
	if err := ensureDeveloperImage(device, version); err != nil {
		return nil, fmt.Errorf("%w: prepare Developer Disk Image: %v", ErrCoreDeviceUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	port, err := reserveLocalPort()
	if err != nil {
		return nil, fmt.Errorf("ios: reserve userspace tunnel port: %w", err)
	}
	tun, err := connectCoreUserspaceTunnel(ctx, device, port)
	if err != nil {
		return nil, fmt.Errorf("%w: establish userspace tunnel: %v", ErrCoreDeviceUnavailable, err)
	}
	tun.UserspaceTUN = true
	tun.UserspaceTUNPort = port

	device.UserspaceTUN = true
	device.UserspaceTUNHost = "127.0.0.1"
	device.UserspaceTUNPort = port
	device.Address = tun.Address
	rsd, err := goios.NewWithAddrPortDevice(tun.Address, tun.RsdPort, device)
	if err != nil {
		_ = tun.Close()
		return nil, fmt.Errorf("%w: connect RSD: %v", ErrCoreDeviceUnavailable, err)
	}
	provider, err := rsd.Handshake()
	_ = rsd.Close()
	if err != nil {
		_ = tun.Close()
		return nil, fmt.Errorf("%w: RSD handshake: %v", ErrCoreDeviceUnavailable, err)
	}
	if err := validateCoreFileServices(provider); err != nil {
		_ = tun.Close()
		return nil, fmt.Errorf("%w: %v", ErrCoreDeviceUnavailable, err)
	}
	device.Rsd = provider
	return &coreTunnelSession{device: device, tunnel: tun}, nil
}

func validateCoreFileServices(provider goios.RsdPortProvider) error {
	missing := make([]string, 0, 2)
	for _, service := range []string{
		corefileservice.ControlServiceName,
		corefileservice.DataServiceName,
	} {
		if provider.GetPort(service) == 0 {
			missing = append(missing, service)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf(
			"mounted Developer Disk Image does not provide required CoreDevice services %s; it may be outdated for this iOS version, so mount a current personalized DDI",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

// ensureDeveloperImage makes CoreDevice usable without an external Xcode,
// devicectl, or go-ios process. A mounted image is shared device state, so it
// is intentionally left mounted when f4 closes: unmounting it would disrupt
// debuggers and other developer tools using the same phone.
func ensureDeveloperImage(device goios.DeviceEntry, version *semver.Version) error {
	mounter, err := imagemounter.NewImageMounter(device)
	if err != nil {
		return fmt.Errorf("connect image mounter: %w", err)
	}
	images, listErr := mounter.ListImages()
	closeErr := mounter.Close()
	if listErr != nil {
		return fmt.Errorf("query mounted images: %w", listErr)
	}
	if len(images) != 0 {
		return closeErr
	}
	if closeErr != nil {
		return fmt.Errorf("close image mounter: %w", closeErr)
	}

	imagePath, err := developerImagePath(device, version)
	if err != nil {
		return err
	}
	if err := imagemounter.MountImage(device, imagePath); err != nil {
		return fmt.Errorf("mount %q: %w", imagePath, err)
	}
	return nil
}

func developerImagePath(device goios.DeviceEntry, version *semver.Version) (string, error) {
	if configured := strings.TrimSpace(os.Getenv("F4_IOS_DEVELOPER_IMAGE")); configured != "" {
		if err := validateDeveloperImagePath(configured, version); err != nil {
			return "", fmt.Errorf("F4_IOS_DEVELOPER_IMAGE: %w", err)
		}
		return filepath.Clean(configured), nil
	}

	// Current Xcode releases install the universal personalized iOS 17+ DDI
	// here. Prefer it over a download when it is present.
	if runtime.GOOS == "darwin" {
		const appleImage = "/Library/Developer/DeveloperDiskImages/iOS_DDI/Restore"
		if validateDeveloperImagePath(appleImage, version) == nil {
			return appleImage, nil
		}
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache for Developer Disk Image: %w", err)
	}
	baseDir := filepath.Join(cacheRoot, "f4", "ios", "developer-images")
	imagePath, err := imagemounter.DownloadImageFor(device, baseDir)
	if err != nil {
		return "", fmt.Errorf("download Developer Disk Image to %q: %w", baseDir, err)
	}
	if err := validateDeveloperImagePath(imagePath, version); err != nil {
		return "", err
	}
	return imagePath, nil
}

func validateDeveloperImagePath(imagePath string, version *semver.Version) error {
	// #nosec G703 -- imagePath is either an explicit user-configured local path or a fixed path returned by the bounded developer-image cache.
	info, err := os.Stat(imagePath)
	if err != nil {
		return fmt.Errorf("invalid Developer Disk Image path %q: %w", imagePath, err)
	}
	if version.Major() >= 17 {
		if !info.IsDir() {
			return fmt.Errorf("developer disk image path %q must be a Restore directory", imagePath)
		}
		// #nosec G703 -- the child name is fixed and imagePath was selected from the user's local configuration/cache above.
		if _, err := os.Stat(filepath.Join(imagePath, "BuildManifest.plist")); err != nil {
			return fmt.Errorf("developer disk image path %q has no BuildManifest.plist: %w", imagePath, err)
		}
		return nil
	}
	if info.IsDir() || !strings.EqualFold(filepath.Ext(imagePath), ".dmg") {
		return fmt.Errorf("developer disk image path %q must be a .dmg file", imagePath)
	}
	return nil
}

func reserveLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return port, listener.Close()
}

func (a *nativeCoreAccess) Close() error {
	a.mu.Lock()
	if a.closed {
		done := a.closeDone
		a.mu.Unlock()
		<-done
		a.mu.Lock()
		err := a.closeErr
		a.mu.Unlock()
		return err
	}
	a.closed = true
	a.cancel()
	sessions := make([]*coreTunnelSession, 0, len(a.sessions))
	for _, session := range a.sessions {
		sessions = append(sessions, session)
	}
	a.sessions = nil
	a.mu.Unlock()
	var result error
	for _, session := range sessions {
		result = joinError(result, session.tunnel.Close())
	}
	a.builds.Wait()
	a.mu.Lock()
	a.closeErr = result
	close(a.closeDone)
	a.mu.Unlock()
	return result
}

func (a *nativeCoreAccess) invalidateSession(udid string, target *coreTunnelSession) {
	a.mu.Lock()
	if a.sessions[udid] != target {
		a.mu.Unlock()
		return
	}
	delete(a.sessions, udid)
	a.mu.Unlock()
	_ = target.tunnel.Close()
}

const (
	coreServiceOpenTimeout  = 15 * time.Second
	coreServiceListTimeout  = 15 * time.Second
	coreServiceCloseTimeout = 2 * time.Second
)

type nativeFileServiceConnection interface {
	ListDirectory(string) ([]string, error)
	PullFile(string, io.Writer) error
	Close() error
}

type nativeCoreFileService struct {
	connection nativeFileServiceConnection
	abort      func()
	abortOnce  sync.Once
	broken     atomic.Bool
	closeOnce  sync.Once
	closeErr   error
}

func (s *nativeCoreFileService) List(ctx context.Context, p string) ([]coreEntry, error) {
	listCtx, cancel := context.WithTimeout(ctx, coreServiceListTimeout)
	defer cancel()
	remote := coreServicePath(p)
	names, err := runCoreOperation(s, listCtx, 0, func() ([]string, error) {
		return s.connection.ListDirectory(remote)
	})
	if err != nil {
		return nil, coreListError(ctx, listCtx, err)
	}
	result := make([]coreEntry, 0, len(names))
	for _, name := range names {
		if err := listCtx.Err(); err != nil {
			return nil, coreListError(ctx, listCtx, err)
		}
		if unsafeRemoteName(name) {
			continue
		}
		entry := coreEntry{Name: name, Hidden: strings.HasPrefix(name, ".")}
		// Current go-ios exposes names but not the metadata dictionaries that
		// devicectl renders. A bounded directory probe preserves correct panel
		// navigation without downloading file data; failures are treated as files.
		if _, probeErr := runCoreOperation(s, listCtx, 0, func() ([]string, error) {
			return s.connection.ListDirectory(coreServicePath(path.Join(p, name)))
		}); probeErr == nil {
			entry.IsDir = true
		} else if errors.Is(probeErr, context.Canceled) || errors.Is(probeErr, context.DeadlineExceeded) || errors.Is(probeErr, ErrCoreDeviceConnection) {
			return nil, coreListError(ctx, listCtx, probeErr)
		}
		result = append(result, entry)
	}
	return result, nil
}

func coreListError(parent, bounded context.Context, err error) error {
	if parentErr := parent.Err(); parentErr != nil {
		return parentErr
	}
	if bounded.Err() != nil {
		return fmt.Errorf("%w: directory listing timed out after %s", ErrCoreDeviceConnection, coreServiceListTimeout)
	}
	return err
}

func (s *nativeCoreFileService) Pull(ctx context.Context, p string, writer io.Writer) error {
	_, err := runCoreOperation(s, ctx, 0, func() (struct{}, error) {
		return struct{}{}, s.connection.PullFile(coreServicePath(p), writer)
	})
	return err
}

func (s *nativeCoreFileService) Close() error {
	s.closeOnce.Do(func() {
		closed := make(chan error, 1)
		go func() { closed <- s.connection.Close() }()
		select {
		case s.closeErr = <-closed:
		case <-time.After(coreServiceCloseTimeout):
			s.breakConnection()
			s.closeErr = fmt.Errorf("%w: closing CoreDevice service timed out", ErrCoreDeviceConnection)
		}
	})
	return s.closeErr
}

type coreOperationResult[T any] struct {
	value T
	err   error
}

func runCoreOperation[T any](s *nativeCoreFileService, ctx context.Context, timeout time.Duration, operation func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, errors.ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if s.broken.Load() {
		return zero, ErrCoreDeviceConnection
	}

	opCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		opCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	result := make(chan coreOperationResult[T], 1)
	go func() {
		value, err := operation()
		result <- coreOperationResult[T]{value: value, err: err}
	}()

	select {
	case completed := <-result:
		if completed.err != nil && coreTransportFailure(completed.err) {
			s.breakConnection()
			return zero, fmt.Errorf("%w: %v", ErrCoreDeviceConnection, completed.err)
		}
		return completed.value, completed.err
	case <-opCtx.Done():
		s.breakConnection()
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return zero, fmt.Errorf("%w: operation timed out after %s", ErrCoreDeviceConnection, timeout)
	}
}

func (s *nativeCoreFileService) breakConnection() {
	s.broken.Store(true)
	s.abortOnce.Do(func() {
		if s.abort != nil {
			s.abort()
		}
	})
}

func coreTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"broken pipe", "connection reset", "connection refused", "connection closed",
		"closed network connection", "stream closed", "transport is closing", "unexpected eof",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func coreServicePath(p string) string {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	if p == "/" {
		// RetrieveDirectoryList expects a relative path. The upstream
		// FileService client and Apple's service use "." for the domain root;
		// an empty string does not produce a response on affected devices.
		return "."
	}
	return strings.TrimPrefix(p, "/")
}

func joinError(base, next error) error {
	if base == nil {
		return next
	}
	if next == nil {
		return base
	}
	return fmt.Errorf("%v; %w", base, next)
}

var _ coreAccess = (*nativeCoreAccess)(nil)
var _ coreFileService = (*nativeCoreFileService)(nil)
