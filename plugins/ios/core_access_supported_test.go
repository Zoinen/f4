//go:build darwin || linux || windows

package iosfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/unxed/f4/plugins/ios/internal/corefileservice"
)

type blockingNativeFileService struct {
	release chan struct{}
	closed  atomic.Int32
}

func (s *blockingNativeFileService) ListDirectory(string) ([]string, error) {
	<-s.release
	return nil, io.ErrUnexpectedEOF
}

func (s *blockingNativeFileService) PullFile(string, io.Writer) error {
	<-s.release
	return io.ErrUnexpectedEOF
}

func (s *blockingNativeFileService) Close() error {
	s.closed.Add(1)
	return nil
}

func TestRunCoreOperationBoundsAndPoisonsStalledConnection(t *testing.T) {
	connection := &blockingNativeFileService{release: make(chan struct{})}
	var aborted atomic.Int32
	service := &nativeCoreFileService{connection: connection, abort: func() {
		aborted.Add(1)
		close(connection.release)
	}}

	_, err := runCoreOperation(service, context.Background(), 20*time.Millisecond, func() ([]string, error) {
		return connection.ListDirectory(".")
	})
	if !errors.Is(err, ErrCoreDeviceConnection) {
		t.Fatalf("stalled operation error = %v", err)
	}
	if aborted.Load() != 1 || !service.broken.Load() {
		t.Fatalf("aborted=%d broken=%v", aborted.Load(), service.broken.Load())
	}
	if _, err := runCoreOperation(service, context.Background(), time.Second, func() ([]string, error) {
		return nil, nil
	}); !errors.Is(err, ErrCoreDeviceConnection) {
		t.Fatalf("operation on poisoned service = %v", err)
	}
}

func TestRunCoreOperationHonorsCallerCancellation(t *testing.T) {
	connection := &blockingNativeFileService{release: make(chan struct{})}
	service := &nativeCoreFileService{connection: connection, abort: func() { close(connection.release) }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runCoreOperation(service, ctx, time.Second, func() ([]string, error) {
		return connection.ListDirectory(".")
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled operation error = %v", err)
	}
}

func TestValidateDeveloperImagePath(t *testing.T) {
	version := semver.MustParse("17.4.0")
	directory := t.TempDir()
	if err := validateDeveloperImagePath(directory, version); err == nil {
		t.Fatal("Restore directory without BuildManifest.plist was accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "BuildManifest.plist"), []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateDeveloperImagePath(directory, version); err != nil {
		t.Fatalf("valid Restore directory rejected: %v", err)
	}
	if err := validateDeveloperImagePath(filepath.Join(directory, "missing"), version); err == nil {
		t.Fatal("missing Developer Disk Image path was accepted")
	}
}

func TestValidateCoreFileServices(t *testing.T) {
	services := map[string]goios.RsdServiceEntry{
		corefileservice.ControlServiceName: {Port: 1234},
		corefileservice.DataServiceName:    {Port: 1235},
	}
	if err := validateCoreFileServices(goios.RsdHandshakeResponse{Services: services}); err != nil {
		t.Fatalf("complete CoreDevice FileService rejected: %v", err)
	}

	delete(services, corefileservice.DataServiceName)
	err := validateCoreFileServices(goios.RsdHandshakeResponse{Services: services})
	if err == nil || !strings.Contains(err.Error(), corefileservice.DataServiceName) || !strings.Contains(err.Error(), "outdated") {
		t.Fatalf("missing CoreDevice FileService error = %v", err)
	}
}

func TestNativeCoreAccessCloseCancelsAndWaitsForBuild(t *testing.T) {
	access := newCoreAccess().(*nativeCoreAccess)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	access.build = func(ctx context.Context, _ DeviceInfo) (*coreTunnelSession, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return nil, ctx.Err()
	}

	sessionResult := make(chan error, 1)
	go func() {
		_, err := access.session(context.Background(), DeviceInfo{UDID: "test-device"})
		sessionResult <- err
	}()
	<-started

	closeResult := make(chan error, 1)
	go func() { closeResult <- access.Close() }()
	<-canceled
	secondCloseResult := make(chan error, 1)
	go func() { secondCloseResult <- access.Close() }()
	select {
	case err := <-closeResult:
		t.Fatalf("Close returned before in-flight build stopped: %v", err)
	default:
	}
	select {
	case err := <-secondCloseResult:
		t.Fatalf("concurrent Close returned before in-flight build stopped: %v", err)
	default:
	}
	close(release)
	if err := <-closeResult; err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if err := <-secondCloseResult; err != nil {
		t.Fatalf("concurrent Close error = %v", err)
	}
	if err := <-sessionResult; !errors.Is(err, ErrCoreDeviceUnavailable) {
		t.Fatalf("session error after Close = %v", err)
	}
}

func TestCoreServicePathUsesRelativeDomainPaths(t *testing.T) {
	for input, want := range map[string]string{
		"/": ".", "": ".", "/Documents": "Documents", "Library/Caches": "Library/Caches",
	} {
		if got := coreServicePath(input); got != want {
			t.Errorf("coreServicePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCoreDeviceVersionAvailability(t *testing.T) {
	for version, want := range map[string]bool{
		"17.3.1": false, "17.4": true, "26.4.1": true, "26.5": false, "26.5.2": false, "26.6": true, "invalid": false,
	} {
		if got := coreDeviceVersionAvailable(version); got != want {
			t.Errorf("coreDeviceVersionAvailable(%q) = %v, want %v", version, got, want)
		}
	}
}
