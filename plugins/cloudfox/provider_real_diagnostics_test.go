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
	realDiagnosticsEnv         = "F4_CLOUDFOX_REAL_DIAGNOSTICS"
	realDiagnosticsConfirmed   = "CONFIRMED"
	realDiagnosticProviderEnv  = "F4_CLOUDFOX_REAL_DIAGNOSTIC_PROVIDER"
	realDiagnosticSizeMiBEnv   = "F4_CLOUDFOX_REAL_DIAGNOSTIC_MIB"
	realDiagnosticFolderPrefix = "f4-cloudfox-real-diagnostic-"
)

// TestRealSavedCloudConnectionDiagnostics measures behavior which a pure CRUD
// matrix cannot prove: progress delivery, reuse on a second Open, cancellation,
// and private-temp cleanup. It is independently opt-in because it deliberately
// transfers the same object more than once. All mutations remain confined to
// one generated folder and are never retried at the HTTP operation level.
func TestRealSavedCloudConnectionDiagnostics(t *testing.T) {
	if os.Getenv(realMutationEnv) != realMutationConfirmed || os.Getenv(realDiagnosticsEnv) != realDiagnosticsConfirmed {
		t.Skip("real CloudFox diagnostics require both explicit confirmations")
	}
	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" || !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox diagnostics require an absolute config directory")
	}
	mib, err := strconv.Atoi(strings.TrimSpace(os.Getenv(realDiagnosticSizeMiBEnv)))
	if err != nil || mib < 1 || mib > 128 {
		t.Fatal("real diagnostic size must be an integer from 1 through 128 MiB")
	}
	selectedProvider := ProviderType(strings.TrimSpace(os.Getenv(realDiagnosticProviderEnv)))
	if selectedProvider != ProviderGoogleDrive && selectedProvider != ProviderYandexDisk {
		t.Fatal("real diagnostic provider must be gdrive or yandex")
	}

	prompt := MasterPasswordPromptFunc(func(ctx context.Context, _ bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return os.Getenv(realVaultPasswordEnv), nil
	})
	plugin := NewPlugin(Options{ConfigDir: configDir, Keyring: NewKeyringStore(), PasswordPrompt: prompt})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close diagnostic CloudFox plugin: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	connections, err := plugin.Repository().List(ctx)
	if err != nil {
		t.Fatalf("list saved diagnostic connections: %v", err)
	}
	selectorEnv := realGoogleSelectorEnv
	if selectedProvider == ProviderYandexDisk {
		selectorEnv = realYandexSelectorEnv
	}
	connection := selectRealConnection(t, connections, selectedProvider, selectorEnv)
	factory, ok := plugin.Factory(selectedProvider)
	if !ok {
		t.Fatal("diagnostic provider factory is unavailable")
	}
	if selectedProvider == ProviderYandexDisk {
		factory = realYandexFactoryWithDialRetries(t, factory)
	}
	secrets, err := plugin.Repository().Credentials(ctx, connection)
	if err != nil {
		t.Fatalf("unlock diagnostic credentials: %v", err)
	}
	backend, err := factory.Open(ctx, connection.Clone(), secrets.Clone())
	clearSecrets(secrets)
	if err != nil {
		t.Fatalf("open diagnostic provider: %v", err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close diagnostic provider: %v", err)
		}
	})

	rootEntries := readRealDirectory(t, ctx, backend, backend.Root())
	writeRoot := realWritableRoot(t, ctx, backend, backend.Root(), rootEntries)
	uuid, err := newUUID()
	if err != nil {
		t.Fatalf("generate diagnostic folder identity: %v", err)
	}
	folderName := realDiagnosticFolderPrefix + strings.ReplaceAll(uuid, "-", "")
	folderCandidate := backend.Join(writeRoot, folderName)
	workspace := ""
	creationTried := false
	t.Cleanup(func() {
		if creationTried {
			cleanupRealDiagnosticWorkspace(t, backend, writeRoot, workspace, folderName)
		}
	})
	creationTried = true
	if err := backend.MkDir(ctx, folderCandidate); err != nil {
		t.Fatalf("create diagnostic workspace: %v", err)
	}
	entry, err := statRealReadOnly(ctx, backend, folderCandidate)
	if err != nil || !entry.IsDir || entry.Name != folderName || entry.Location == "" {
		t.Fatalf("resolve diagnostic workspace: %v", err)
	}
	workspace = entry.Location
	assertRealDiagnosticWorkspace(t, ctx, backend, writeRoot, workspace, folderName)

	baselineUploads := realDiagnosticTempPaths(t, "f4-cloudfox-upload-*")
	baselineDownloads := realDiagnosticTempPaths(t, "f4-cloudfox-yandex-download-*")
	size := int64(mib) << 20
	block := realPatternBytes(1<<20, 0x6d)
	uploadReporter := &realDiagnosticReporter{}
	uploadCtx := context.WithValue(ctx, vfs.ReporterKey, vfs.TaskReporter(uploadReporter))
	fileCandidate := backend.Join(workspace, fmt.Sprintf("diagnostic-%d-mib.bin", mib))
	uploadStarted := time.Now()
	wantHash := writeRealPattern(t, uploadCtx, backend, fileCandidate, size, block)
	t.Logf("upload: duration=%s progress=%s", time.Since(uploadStarted).Round(time.Millisecond), uploadReporter.summary())
	file := statRealFile(t, ctx, backend, fileCandidate, size)

	first := measureRealDiagnosticDownload(ctx, backend, file.Location)
	if first.err != nil || first.size != size || first.hash != wantHash {
		t.Fatalf("first diagnostic download: bytes=%d hash-match=%t: %s", first.size, first.hash == wantHash, redactRealProviderError(first.err))
	}
	t.Logf("first open/read: open=%s total=%s progress=%s", first.openDuration.Round(time.Millisecond), first.totalDuration.Round(time.Millisecond), first.progressSummary)

	second := measureRealDiagnosticDownload(ctx, backend, file.Location)
	if second.err != nil || second.size != size || second.hash != wantHash {
		t.Fatalf("second diagnostic download: bytes=%d hash-match=%t: %s", second.size, second.hash == wantHash, redactRealProviderError(second.err))
	}
	t.Logf("second open/read: open=%s total=%s progress=%s", second.openDuration.Round(time.Millisecond), second.totalDuration.Round(time.Millisecond), second.progressSummary)

	cancelCtx, cancelRead := context.WithCancel(context.Background())
	cancelResult := make(chan error, 1)
	go func() {
		reader, openErr := backend.Open(cancelCtx, file.Location)
		if openErr != nil {
			cancelResult <- openErr
			return
		}
		defer func() { _ = reader.Close() }()
		buffer := make([]byte, 1<<20)
		for {
			_, readErr := reader.Read(cancelCtx, buffer)
			if readErr != nil {
				cancelResult <- readErr
				return
			}
		}
	}()
	timer := time.NewTimer(25 * time.Millisecond)
	select {
	case err := <-cancelResult:
		timer.Stop()
		t.Logf("cancellation probe completed before cancellation: %s", redactRealProviderError(err))
	case <-timer.C:
		cancelRead()
		select {
		case err := <-cancelResult:
			if err == nil || (!errors.Is(err, context.Canceled) && !strings.Contains(strings.ToLower(err.Error()), "canceled")) {
				t.Fatalf("download cancellation returned %s, want context cancellation", redactRealProviderError(err))
			}
			t.Logf("cancellation: %s", redactRealProviderError(err))
		case <-time.After(30 * time.Second):
			t.Fatal("download did not stop within 30 seconds after cancellation")
		}
	}
	cancelRead()

	assertNoNewRealDiagnosticTemps(t, baselineDownloads, "f4-cloudfox-yandex-download-*")
	assertNoNewRealDiagnosticTemps(t, baselineUploads, "f4-cloudfox-upload-*")
	if current, err := statRealReadOnly(ctx, backend, file.Location); err != nil || current.IsDir || current.Size != size {
		t.Fatalf("remote object changed after canceled read: %v", err)
	}
}

type realDiagnosticReporter struct {
	mu      sync.Mutex
	samples []int
}

func (*realDiagnosticReporter) UpdateScan(string, int64, int64) {}
func (r *realDiagnosticReporter) UpdateTransfer(_ string, _ string, currentPct int, _ string, _ int, _ string) {
	r.mu.Lock()
	r.samples = append(r.samples, currentPct)
	r.mu.Unlock()
}
func (*realDiagnosticReporter) IsCancelled() bool { return false }

func (r *realDiagnosticReporter) summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.samples) == 0 {
		return "none"
	}
	minimum, maximum := r.samples[0], r.samples[0]
	for _, sample := range r.samples[1:] {
		if sample < minimum {
			minimum = sample
		}
		if sample > maximum {
			maximum = sample
		}
	}
	return fmt.Sprintf("count=%d min=%d max=%d", len(r.samples), minimum, maximum)
}

type realDiagnosticDownload struct {
	hash            [sha256.Size]byte
	size            int64
	openDuration    time.Duration
	totalDuration   time.Duration
	progressSummary string
	err             error
}

func measureRealDiagnosticDownload(ctx context.Context, backend Backend, location string) realDiagnosticDownload {
	started := time.Now()
	reporter := &realDiagnosticReporter{}
	openCtx := context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(func(_ string, percent int) {
		reporter.mu.Lock()
		reporter.samples = append(reporter.samples, percent)
		reporter.mu.Unlock()
	}))
	reader, err := backend.Open(openCtx, location)
	result := realDiagnosticDownload{openDuration: time.Since(started), progressSummary: reporter.summary(), err: err}
	if err != nil {
		result.totalDuration = time.Since(started)
		return result
	}
	defer func() { _ = reader.Close() }()
	h := sha256.New()
	buffer := make([]byte, 1<<20)
	for {
		n, readErr := reader.Read(ctx, buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
			result.size += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			result.err = readErr
			break
		}
		if n == 0 {
			result.err = io.ErrNoProgress
			break
		}
	}
	copy(result.hash[:], h.Sum(nil))
	result.totalDuration = time.Since(started)
	result.progressSummary = reporter.summary()
	return result
}

func realDiagnosticTempPaths(t *testing.T, pattern string) map[string]struct{} {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), pattern))
	if err != nil {
		t.Fatalf("enumerate diagnostic temp files: %v", err)
	}
	paths := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		paths[match] = struct{}{}
	}
	return paths
}

func assertNoNewRealDiagnosticTemps(t *testing.T, before map[string]struct{}, pattern string) {
	t.Helper()
	for path := range realDiagnosticTempPaths(t, pattern) {
		if _, existed := before[path]; !existed {
			t.Errorf("new diagnostic temporary file remains: %s", filepath.Base(path))
		}
	}
}

func assertRealDiagnosticWorkspace(t *testing.T, ctx context.Context, backend Backend, writeRoot, workspace, folderName string) {
	t.Helper()
	if folderName == "" || !strings.HasPrefix(folderName, realDiagnosticFolderPrefix) || workspace == "" || workspace == writeRoot || backend.IsRoot(workspace) {
		t.Fatal("refusing unsafe diagnostic workspace")
	}
	matches := 0
	for _, child := range readRealDirectory(t, ctx, backend, writeRoot) {
		if child.Name == folderName && child.Location == workspace && child.IsDir {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("diagnostic workspace identity is not unique in the writable root")
	}
}

func cleanupRealDiagnosticWorkspace(t *testing.T, backend Backend, writeRoot, workspace, folderName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var children []RemoteEntry
	if err := backend.ReadDir(ctx, writeRoot, func(chunk []RemoteEntry) {
		children = append(children, chunk...)
	}); err != nil {
		t.Errorf("list writable root during diagnostic cleanup: %v", err)
		return
	}
	matches := make([]RemoteEntry, 0, 1)
	for _, child := range children {
		if child.Name == folderName && child.IsDir {
			matches = append(matches, child)
		}
	}
	if len(matches) == 0 {
		return
	}
	if len(matches) != 1 {
		t.Errorf("refusing ambiguous diagnostic cleanup: %d exact folders", len(matches))
		return
	}
	if workspace != "" && matches[0].Location != workspace {
		t.Errorf("refusing diagnostic cleanup after canonical identity changed")
		return
	}
	workspace = matches[0].Location
	assertRealDiagnosticWorkspace(t, ctx, backend, writeRoot, workspace, folderName)
	if err := backend.Remove(ctx, workspace); err != nil {
		t.Errorf("remove diagnostic workspace: %v", err)
		return
	}
	for _, child := range readRealDirectory(t, ctx, backend, writeRoot) {
		if child.Name == folderName && child.Location == workspace {
			t.Errorf("diagnostic workspace remains after cleanup")
			return
		}
	}
}
