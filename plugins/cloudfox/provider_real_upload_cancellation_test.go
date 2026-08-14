package cloudfox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	realUploadCancellationEnv          = "F4_CLOUDFOX_REAL_UPLOAD_CANCELLATION"
	realUploadCancellationConfirmed    = "CONFIRMED"
	realUploadCancellationProviderEnv  = "F4_CLOUDFOX_REAL_UPLOAD_CANCEL_PROVIDER"
	realUploadCancellationSizeMiBEnv   = "F4_CLOUDFOX_REAL_UPLOAD_CANCEL_MIB"
	realUploadCancellationFolderMarker = "upload-cancel-"
)

// TestRealSavedCloudUploadCancellation deliberately interrupts one in-flight
// upload through a saved Google Drive or Yandex.Disk connection. It is
// independently and doubly gated because cancellation can leave a provider
// mutation in an uncertain state. Every object is confined to one generated
// folder, any visible uncertain result is permanently removed by exact remote
// identity, and the whole generated folder is permanently removed afterward.
//
// The provider must be selected explicitly with
// F4_CLOUDFOX_REAL_UPLOAD_CANCEL_PROVIDER (gdrive or yandex), and the staged
// object size must be supplied with F4_CLOUDFOX_REAL_UPLOAD_CANCEL_MIB. Yandex
// may use the existing harness-only DialContext retry setting; that retries
// only connection establishment before an HTTP request exists and never
// retries the upload or any other mutation.
func TestRealSavedCloudUploadCancellation(t *testing.T) {
	if os.Getenv(realMutationEnv) != realMutationConfirmed || os.Getenv(realUploadCancellationEnv) != realUploadCancellationConfirmed {
		t.Skip("real CloudFox upload cancellation requires both explicit confirmations")
	}

	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" || !filepath.IsAbs(configDir) {
		t.Fatal("real upload cancellation requires an absolute config directory")
	}
	if info, err := os.Stat(configDir); err != nil || !info.IsDir() {
		t.Fatal("real upload-cancellation config directory is unavailable")
	}

	selectedProvider := ProviderType(strings.TrimSpace(os.Getenv(realUploadCancellationProviderEnv)))
	if selectedProvider != ProviderGoogleDrive && selectedProvider != ProviderYandexDisk {
		t.Fatal("real upload-cancellation provider must be gdrive or yandex")
	}
	sizeMiB, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(realUploadCancellationSizeMiBEnv)), 10, 32)
	if err != nil || sizeMiB < 32 || sizeMiB > 1024 {
		t.Fatal("real upload-cancellation size must be an integer from 32 through 1024 MiB")
	}
	targetSize := sizeMiB << 20

	prompt := MasterPasswordPromptFunc(func(ctx context.Context, _ bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return os.Getenv(realVaultPasswordEnv), nil
	})
	plugin := NewPlugin(Options{ConfigDir: configDir, Keyring: NewKeyringStore(), PasswordPrompt: prompt})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close upload-cancellation CloudFox plugin: %s", realUploadCancellationErrorClass(err))
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	connections, err := plugin.Repository().List(ctx)
	if err != nil {
		t.Fatalf("list saved upload-cancellation connections: %s", realUploadCancellationErrorClass(err))
	}
	selectorEnv := realGoogleSelectorEnv
	if selectedProvider == ProviderYandexDisk {
		selectorEnv = realYandexSelectorEnv
	}
	connection := selectRealConnection(t, connections, selectedProvider, selectorEnv)
	factory, ok := plugin.Factory(selectedProvider)
	if !ok {
		t.Fatal("upload-cancellation provider factory is unavailable")
	}
	if selectedProvider == ProviderYandexDisk {
		factory = realYandexFactoryWithDialRetries(t, factory)
	}
	secrets, err := plugin.Repository().Credentials(ctx, connection)
	if err != nil {
		t.Fatalf("unlock upload-cancellation credentials: %s", realUploadCancellationErrorClass(err))
	}
	backend, err := factory.Open(ctx, connection.Clone(), secrets.Clone())
	clearSecrets(secrets)
	if err != nil {
		t.Fatalf("open upload-cancellation provider: %s", realUploadCancellationErrorClass(err))
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close upload-cancellation provider: %s", realUploadCancellationErrorClass(err))
		}
	})

	rootEntries := readRealDirectory(t, ctx, backend, backend.Root())
	writeRoot := realWritableRoot(t, ctx, backend, backend.Root(), rootEntries)
	uuid, err := newUUID()
	if err != nil {
		t.Fatal("generate upload-cancellation folder identity")
	}
	folderName := realFolderPrefix + realUploadCancellationFolderMarker + strings.ReplaceAll(uuid, "-", "")
	folderCandidate := backend.Join(writeRoot, folderName)
	workspace := ""
	creationTried := false
	t.Cleanup(func() {
		if creationTried {
			cleanupRealWorkspace(t, backend, writeRoot, workspace, folderName)
		}
	})
	creationTried = true
	if err := backend.MkDir(ctx, folderCandidate); err != nil {
		t.Fatalf("create upload-cancellation workspace: %s", realUploadCancellationErrorClass(err))
	}
	workspaceEntry, err := statRealReadOnly(ctx, backend, folderCandidate)
	if err != nil || !workspaceEntry.IsDir || workspaceEntry.Name != folderName || workspaceEntry.Location == "" {
		t.Fatalf("resolve upload-cancellation workspace: %s", realUploadCancellationErrorClass(err))
	}
	workspace = workspaceEntry.Location
	assertRealWorkspaceTarget(t, ctx, backend, writeRoot, workspace, folderName, workspaceEntry)

	candidateName := fmt.Sprintf("cancel-%d-mib.bin", sizeMiB)
	candidate := backend.Join(workspace, candidateName)
	t.Logf("phase: cancel an in-flight %d MiB %s upload", sizeMiB, selectedProvider)
	result, cancelLatency := runRealUploadCancellation(t, backend, selectedProvider, candidate, targetSize)
	resultErr := result.closeErr
	if resultErr == nil {
		resultErr = result.writeErr
	}
	if resultErr == nil {
		t.Fatal("canceled upload returned success")
	}
	if !errors.Is(resultErr, context.Canceled) && !errors.Is(resultErr, vfs.ErrOperationStateUnknown) {
		t.Fatalf("canceled upload returned an unsafe error classification: %s", realUploadCancellationErrorClass(resultErr))
	}
	if cancelLatency > 30*time.Second {
		t.Fatalf("canceled upload took %s to return; want at most 30s", cancelLatency.Round(time.Millisecond))
	}
	t.Logf("cancellation returned in %s with %s (writer accepted %d bytes)", cancelLatency.Round(time.Millisecond), realUploadCancellationErrorClass(resultErr), result.accepted)

	visible, err := observeRealUploadCancellationResult(ctx, backend, workspace, candidateName, 8*time.Second)
	if err != nil {
		t.Fatalf("inspect canceled upload result: %s", realUploadCancellationErrorClass(err))
	}
	if len(visible) == 0 {
		t.Log("canceled upload left no visible remote object")
	} else {
		if !errors.Is(resultErr, vfs.ErrOperationStateUnknown) {
			t.Fatal("canceled upload left a visible object without unknown-state classification")
		}
		t.Logf("provider safely classified unknown state; removing %d visible exact-name result(s)", len(visible))
		for _, entry := range visible {
			if entry.IsDir || entry.Location == "" || entry.Name != candidateName {
				t.Fatal("refusing to remove an unsafe canceled-upload result")
			}
			if err := backend.Remove(ctx, entry.Location); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("remove visible uncertain upload result: %s", realUploadCancellationErrorClass(err))
			}
		}
		remaining, err := listRealUploadCancellationMatches(ctx, backend, workspace, candidateName)
		if err != nil {
			t.Fatalf("verify uncertain upload removal: %s", realUploadCancellationErrorClass(err))
		}
		if len(remaining) != 0 {
			t.Fatal("visible uncertain upload result remains after exact permanent removal")
		}
	}

	t.Log("phase: verify the same provider session remains usable")
	healthName := "post-cancel-health.txt"
	healthCandidate := backend.Join(workspace, healthName)
	healthPayload := []byte("CloudFox upload cancellation health check\n")
	if err := writeRealUploadCancellationBytes(ctx, backend, healthCandidate, healthPayload); err != nil {
		t.Fatalf("write post-cancellation health object: %s", realUploadCancellationErrorClass(err))
	}
	healthMatches, err := listRealUploadCancellationMatches(ctx, backend, workspace, healthName)
	if err != nil || len(healthMatches) != 1 || healthMatches[0].IsDir || healthMatches[0].Size != int64(len(healthPayload)) {
		t.Fatalf("resolve post-cancellation health object: count=%d error=%s", len(healthMatches), realUploadCancellationErrorClass(err))
	}
	gotHash, gotSize, reportedSize, err := hashRealRemote(ctx, backend, healthMatches[0].Location)
	wantHash := sha256Bytes(healthPayload)
	if err != nil || gotHash != wantHash || gotSize != int64(len(healthPayload)) || reportedSize != int64(len(healthPayload)) {
		t.Fatalf("read post-cancellation health object: bytes=%d reported=%d hash-match=%t error=%s", gotSize, reportedSize, gotHash == wantHash, realUploadCancellationErrorClass(err))
	}
	if err := backend.Remove(ctx, healthMatches[0].Location); err != nil {
		t.Fatalf("remove post-cancellation health object: %s", realUploadCancellationErrorClass(err))
	}
	remaining, err := listRealUploadCancellationMatches(ctx, backend, workspace, healthName)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("post-cancellation health object remains after removal: count=%d error=%s", len(remaining), realUploadCancellationErrorClass(err))
	}
}

type realUploadCancellationResult struct {
	accepted int64
	writeErr error
	closeErr error
}

func runRealUploadCancellation(t *testing.T, backend Backend, provider ProviderType, location string, targetSize int64) (realUploadCancellationResult, time.Duration) {
	t.Helper()
	uploadCtx, cancelUpload := context.WithCancel(context.Background())
	defer cancelUpload()
	reporter := newRealUploadCancellationReporter()
	if provider == ProviderYandexDisk {
		uploadCtx = context.WithValue(uploadCtx, vfs.ReporterKey, vfs.TaskReporter(reporter))
	}
	w, err := backend.Create(uploadCtx, location)
	if err != nil {
		t.Fatalf("start cancellable upload: %s", realUploadCancellationErrorClass(err))
	}

	block := realPatternBytes(1<<20, 0xa7)
	if provider == ProviderYandexDisk {
		var accepted int64
		for accepted < targetSize {
			chunk := block
			if int64(len(chunk)) > targetSize-accepted {
				chunk = chunk[:targetSize-accepted]
			}
			n, writeErr := w.Write(chunk)
			accepted += int64(n)
			if writeErr != nil {
				_ = w.Close()
				t.Fatalf("stage cancellable upload: accepted=%d class=%s", accepted, realUploadCancellationErrorClass(writeErr))
			}
			if n != len(chunk) {
				_ = w.Close()
				t.Fatalf("stage cancellable upload: short write %d/%d", n, len(chunk))
			}
		}
		result := make(chan realUploadCancellationResult, 1)
		go func() {
			result <- realUploadCancellationResult{accepted: accepted, closeErr: w.Close()}
		}()
		select {
		case <-reporter.active:
		case early := <-result:
			t.Fatalf("Yandex upload completed before it could be canceled: %s", realUploadCancellationErrorClass(early.closeErr))
		case <-time.After(5 * time.Minute):
			cancelUpload()
			t.Fatal("Yandex upload body did not become active within five minutes")
		}
		canceledAt := time.Now()
		cancelUpload()
		select {
		case finished := <-result:
			return finished, time.Since(canceledAt)
		case <-time.After(30 * time.Second):
			t.Fatal("Yandex upload did not return within 30 seconds after cancellation")
		}
	}

	result := make(chan realUploadCancellationResult, 1)
	active := make(chan struct{})
	const googleActivationBytes = int64(16 << 20)
	go func() {
		var finished realUploadCancellationResult
		var activeOnce sync.Once
		for finished.accepted < targetSize {
			chunk := block
			if int64(len(chunk)) > targetSize-finished.accepted {
				chunk = chunk[:targetSize-finished.accepted]
			}
			n, writeErr := w.Write(chunk)
			finished.accepted += int64(n)
			if finished.accepted >= googleActivationBytes {
				activeOnce.Do(func() { close(active) })
			}
			if writeErr != nil {
				finished.writeErr = writeErr
				break
			}
			if n != len(chunk) {
				finished.writeErr = io.ErrShortWrite
				break
			}
		}
		finished.closeErr = w.Close()
		result <- finished
	}()
	select {
	case <-active:
	case early := <-result:
		t.Fatalf("Google upload completed before it could be canceled: %s", realUploadCancellationErrorClass(firstRealUploadCancellationError(early)))
	case <-time.After(5 * time.Minute):
		cancelUpload()
		t.Fatal("Google upload did not accept 16 MiB within five minutes")
	}
	canceledAt := time.Now()
	cancelUpload()
	select {
	case finished := <-result:
		return finished, time.Since(canceledAt)
	case <-time.After(30 * time.Second):
		t.Fatal("Google upload did not return within 30 seconds after cancellation")
	}
	return realUploadCancellationResult{}, 0
}

type realUploadCancellationReporter struct {
	active chan struct{}
	once   sync.Once
}

func newRealUploadCancellationReporter() *realUploadCancellationReporter {
	return &realUploadCancellationReporter{active: make(chan struct{})}
}

func (*realUploadCancellationReporter) UpdateScan(string, int64, int64) {}
func (r *realUploadCancellationReporter) UpdateTransfer(_ string, _ string, percent int, _ string, _ int, _ string) {
	if percent > 0 {
		r.once.Do(func() { close(r.active) })
	}
}
func (*realUploadCancellationReporter) IsCancelled() bool { return false }

func observeRealUploadCancellationResult(ctx context.Context, backend Backend, workspace, name string, duration time.Duration) ([]RemoteEntry, error) {
	deadline := time.Now().Add(duration)
	var visible []RemoteEntry
	for {
		matches, err := listRealUploadCancellationMatches(ctx, backend, workspace, name)
		if err != nil {
			return nil, err
		}
		if len(matches) != 0 {
			visible = matches
		}
		if !time.Now().Before(deadline) {
			return visible, nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func listRealUploadCancellationMatches(ctx context.Context, backend Backend, workspace, name string) ([]RemoteEntry, error) {
	var matches []RemoteEntry
	err := readDirRealReadOnly(ctx, backend, workspace, func(chunk []RemoteEntry) {
		for _, entry := range chunk {
			if entry.Name == name {
				matches = append(matches, entry)
			}
		}
	})
	return matches, err
}

func writeRealUploadCancellationBytes(ctx context.Context, backend Backend, location string, payload []byte) error {
	w, err := backend.Create(ctx, location)
	if err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func sha256Bytes(payload []byte) [32]byte {
	// Keep the real harness's comparison value concrete without logging any
	// remote content, location or provider URL.
	return sha256.Sum256(payload)
}

func firstRealUploadCancellationError(result realUploadCancellationResult) error {
	if result.closeErr != nil {
		return result.closeErr
	}
	return result.writeErr
}

// realUploadCancellationErrorClass deliberately avoids err.Error(): Yandex's
// transport error can contain a signed upload URL. Test output records only
// stable safety classifications and the concrete error type.
func realUploadCancellationErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	unknown := errors.Is(err, vfs.ErrOperationStateUnknown)
	canceled := errors.Is(err, context.Canceled)
	deadline := errors.Is(err, context.DeadlineExceeded)
	switch {
	case unknown && canceled:
		return "unknown-state+context-canceled"
	case unknown && deadline:
		return "unknown-state+deadline-exceeded"
	case unknown:
		return "unknown-state"
	case canceled:
		return "context-canceled"
	case deadline:
		return "deadline-exceeded"
	default:
		return fmt.Sprintf("unexpected(%T)", err)
	}
}
