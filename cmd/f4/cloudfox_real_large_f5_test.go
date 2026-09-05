package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/unxed/f4/plugins/cloudfox"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const (
	realCloudFoxLargeF5Env       = "F4_CLOUDFOX_REAL_LARGE_F5"
	realCloudFoxLargeF5Confirmed = "CONFIRMED"
	realCloudFoxLargeF5PathEnv   = "F4_CLOUDFOX_REAL_ARCHIVE_PATH"

	realCloudFoxLargeF5LocalPrefix = "f4-cloudfox-real-large-f5-"
)

// TestRealSavedCloudLargeF5RoundTrip copies one caller-supplied large file
// local -> cloud -> a new local directory through the production F5 action,
// ExecuteFileOpAt and FileOpProgressDialog route. It hashes both local copies,
// records the visible progress percentages and permanently removes only its
// UUID-named cloud workspace.
//
// The test is inert unless F4_CLOUDFOX_REAL_LARGE_F5 is exactly CONFIRMED. It
// also requires an absolute saved-profile directory and an absolute existing
// file of at least 300 MiB in F4_CLOUDFOX_REAL_ARCHIVE_PATH before it opens a
// profile or performs network I/O. The ordinary CloudFox selector, vault and
// harness-only Yandex TCP dial-retry variables documented by
// TestRealSavedCloudConnectionsUI are reused. A dial retry can happen only
// before a connection exists; HTTP requests and mutations are never retried.
// All frames are hosted by SilentScreenBuf, so the test cannot open a native
// window. It never records credentials, provider URLs or HTTP payloads.
func TestRealSavedCloudLargeF5RoundTrip(t *testing.T) {
	if os.Getenv(realCloudFoxLargeF5Env) != realCloudFoxLargeF5Confirmed {
		t.Skip("real CloudFox large F5 mutations require explicit confirmation")
	}

	fixturePath, fixtureSize, fixtureHash := loadRealCloudFoxLargeF5Fixture(t)
	configDir := strings.TrimSpace(os.Getenv(realCloudFoxConfigEnv))
	if configDir == "" || !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox large F5 config directory must be absolute")
	}
	info, err := os.Stat(configDir) // #nosec G703 -- this opt-in integration config must be an operator-supplied absolute directory.
	if err != nil || !info.IsDir() {
		t.Fatal("real CloudFox large F5 config directory is unavailable")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	originalConfig := AppConfig
	t.Cleanup(func() { AppConfig = originalConfig })
	AppConfig.ConfirmCopy = false
	AppConfig.DefaultFileOpMode = 2

	prompt := cloudfox.MasterPasswordPromptFunc(func(ctx context.Context, _ bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return os.Getenv(realCloudFoxVaultEnv), nil
	})
	plugin := cloudfox.NewPlugin(cloudfox.Options{
		ConfigDir:      configDir,
		Portable:       true,
		Keyring:        cloudfox.NewKeyringStore(),
		PasswordPrompt: prompt,
		Factories:      realCloudFoxUIFactories(t),
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close real CloudFox large F5 plugin: %v", err)
		}
	})

	host := newRealCloudFoxUIHost()
	if err := plugin.Init(host); err != nil {
		t.Fatalf("initialize production CloudFox registrations for large F5 test: %v", err)
	}
	if host.driveFactory == nil || len(host.vfsProviders) == 0 {
		t.Fatal("CloudFox did not register its production drive providers")
	}

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved CloudFox profiles for large F5 test: %v", err)
	}

	targets := []struct {
		name        string
		provider    cloudfox.ProviderType
		selectorEnv string
	}{
		{name: "google-drive", provider: cloudfox.ProviderGoogleDrive, selectorEnv: realCloudFoxGoogleEnv},
		{name: "yandex-disk", provider: cloudfox.ProviderYandexDisk, selectorEnv: realCloudFoxYandexEnv},
	}
	for _, target := range targets {
		target := target
		t.Run(target.name, func(t *testing.T) {
			connection := selectRealCloudFoxUIConnection(t, connections, target.provider, target.selectorEnv)
			root := mountRealCrossCloudWriteRoot(t, host, connection)
			runID := strings.ReplaceAll(uuid.NewString(), "-", "")
			workspace := createRealCrossCloudWorkspace(t, root, target.provider, "large-f5-"+string(target.provider)+"-"+runID)
			runRealCloudFoxLargeF5Provider(t, workspace, fixturePath, fixtureSize, fixtureHash)
		})
	}
}

func loadRealCloudFoxLargeF5Fixture(t *testing.T) (string, int64, string) {
	t.Helper()
	fixturePath := strings.TrimSpace(os.Getenv(realCloudFoxLargeF5PathEnv))
	if fixturePath == "" || !filepath.IsAbs(fixturePath) {
		t.Fatal("real CloudFox large F5 fixture path must be absolute")
	}
	info, err := os.Stat(fixturePath) // #nosec G703 -- this opt-in integration fixture must be an operator-supplied absolute path.
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("real CloudFox large F5 fixture is unavailable or is not a regular file")
	}
	if info.Size() < 300*1024*1024 {
		t.Fatalf("real CloudFox large F5 fixture is only %d bytes; want at least 300 MiB", info.Size())
	}
	hash, err := hashRealCloudFoxLargeF5File(fixturePath)
	if err != nil {
		t.Fatalf("hash real CloudFox large F5 fixture: %v", err)
	}
	return fixturePath, info.Size(), hash
}

func runRealCloudFoxLargeF5Provider(t *testing.T, endpoint *realCrossCloudEndpoint, fixturePath string, fixtureSize int64, fixtureHash string) {
	t.Helper()
	name := filepath.Base(fixturePath)
	if name == "" || name == "." || name == string(filepath.Separator) {
		t.Fatal("real CloudFox large F5 fixture has an invalid base name")
	}
	operationTimeout := 30 * time.Minute
	if endpoint.provider == cloudfox.ProviderYandexDisk {
		// The real Yandex data plane has taken more than 30 minutes to commit
		// this exact 320 MiB fixture. A test deadline must not manufacture a
		// cancellation while the mutation is still legitimately in flight.
		operationTimeout = 75 * time.Minute
	}

	uploadSource := vfs.NewOSVFS(filepath.Dir(fixturePath))
	uploadDestination := endpoint.workspace.Clone()
	if uploadDestination == nil || uploadDestination == endpoint.workspace {
		t.Fatal("CloudVFS did not clone for the large F5 upload panel")
	}
	uploadStarted := time.Now()
	uploadTrace, err := runRealCloudFoxLargeF5Action(t, uploadSource, uploadDestination, name, operationTimeout)
	_ = uploadSource.Close()
	_ = uploadDestination.Close()
	if err != nil {
		t.Fatalf("production large F5 local-to-cloud copy failed: %v", err)
	}
	uploadDuration := time.Since(uploadStarted)
	assertRealCloudFoxLargeF5Progress(t, "upload", uploadTrace)

	verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 3*time.Minute)
	remotePath := requireRealCloudFoxUIFile(t, verifyCtx, endpoint.workspace, endpoint.provider, name, fixtureSize)
	cancelVerify()
	if remotePath == "" {
		t.Fatal("production large F5 upload has no canonical remote identity")
	}

	downloadDir, cleanupLocal := newRealCloudFoxLargeF5LocalDir(t, endpoint.provider)
	defer cleanupLocal()
	downloadSource := endpoint.workspace.Clone()
	if downloadSource == nil || downloadSource == endpoint.workspace {
		t.Fatal("CloudVFS did not clone for the large F5 download panel")
	}
	downloadDestination := vfs.NewOSVFS(downloadDir)
	downloadStarted := time.Now()
	downloadTrace, err := runRealCloudFoxLargeF5Action(t, downloadSource, downloadDestination, name, operationTimeout)
	_ = downloadSource.Close()
	_ = downloadDestination.Close()
	if err != nil {
		t.Fatalf("production large F5 cloud-to-local copy failed: %v", err)
	}
	downloadDuration := time.Since(downloadStarted)
	assertRealCloudFoxLargeF5Progress(t, "download", downloadTrace)

	downloadPath := filepath.Join(downloadDir, name)
	downloadInfo, err := os.Stat(downloadPath)
	if err != nil || !downloadInfo.Mode().IsRegular() || downloadInfo.Size() != fixtureSize {
		t.Fatalf("large F5 downloaded file metadata mismatch: size=%d want=%d err=%v", fileSizeOrMinusOne(downloadInfo), fixtureSize, err)
	}
	downloadHash, err := hashRealCloudFoxLargeF5File(downloadPath)
	if err != nil {
		t.Fatalf("hash large F5 downloaded file: %v", err)
	}
	if downloadHash != fixtureHash {
		t.Fatalf("large F5 roundtrip SHA-256 mismatch: got %s want %s", downloadHash, fixtureHash)
	}
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		t.Fatalf("list large F5 local destination: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != name || entries[0].IsDir() {
		t.Fatalf("large F5 local destination contains %d rows instead of the one exact file", len(entries))
	}

	t.Logf("large F5 roundtrip size=%d sha256=%s upload=%s progress={%s} download=%s progress={%s}",
		fixtureSize, fixtureHash, uploadDuration.Round(time.Millisecond), uploadTrace.summary(), downloadDuration.Round(time.Millisecond), downloadTrace.summary())

	// Prove exact local cleanup now rather than leaving a successful 320 MiB
	// download in the process temp directory until testing.T cleanup runs.
	if err := os.Remove(downloadPath); err != nil {
		t.Fatalf("remove exact large F5 local result: %v", err)
	}
	if _, err := os.Stat(downloadPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("large F5 local result remains after exact removal: %v", err)
	}
	if err := os.Remove(downloadDir); err != nil {
		t.Fatalf("remove empty large F5 local destination: %v", err)
	}
	if _, err := os.Stat(downloadDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("large F5 local destination remains after cleanup: %v", err)
	}
}

func fileSizeOrMinusOne(info os.FileInfo) int64 {
	if info == nil {
		return -1
	}
	return info.Size()
}

func hashRealCloudFoxLargeF5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func newRealCloudFoxLargeF5LocalDir(t *testing.T, provider cloudfox.ProviderType) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", realCloudFoxLargeF5LocalPrefix+string(provider)+"-")
	if err != nil {
		t.Fatalf("create isolated large F5 local destination: %v", err)
	}
	cleaned := false
	cleanup := func() {
		t.Helper()
		if cleaned {
			return
		}
		cleaned = true
		cleanDir := filepath.Clean(dir)
		if filepath.Clean(filepath.Dir(cleanDir)) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(cleanDir), realCloudFoxLargeF5LocalPrefix) {
			t.Errorf("refusing unsafe large F5 local cleanup")
			return
		}
		if err := os.RemoveAll(cleanDir); err != nil {
			t.Errorf("remove isolated large F5 local destination: %v", err)
			return
		}
		if _, err := os.Stat(cleanDir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("isolated large F5 local destination remains after fallback cleanup: %v", err)
		}
	}
	t.Cleanup(cleanup)
	return dir, cleanup
}

type realCloudFoxLargeF5PercentSample struct {
	at      time.Duration
	percent int
}

type realCloudFoxLargeF5ProgressTrace struct {
	startedAt time.Time
	finished  time.Duration
	sawDialog bool
	current   []realCloudFoxLargeF5PercentSample
	total     []realCloudFoxLargeF5PercentSample
}

func (trace *realCloudFoxLargeF5ProgressTrace) record(samples *[]realCloudFoxLargeF5PercentSample, value int) {
	if value < 0 || value > 100 {
		return
	}
	if len(*samples) != 0 && (*samples)[len(*samples)-1].percent == value {
		return
	}
	*samples = append(*samples, realCloudFoxLargeF5PercentSample{at: time.Since(trace.startedAt), percent: value})
}

func (trace realCloudFoxLargeF5ProgressTrace) summary() string {
	return fmt.Sprintf("elapsed=%s current=%s total=%s", trace.finished.Round(time.Millisecond), summarizeRealCloudFoxLargeF5Samples(trace.current), summarizeRealCloudFoxLargeF5Samples(trace.total))
}

func summarizeRealCloudFoxLargeF5Samples(samples []realCloudFoxLargeF5PercentSample) string {
	if len(samples) == 0 {
		return "none"
	}
	minPercent, maxPercent := samples[0].percent, samples[0].percent
	firstAdvance := time.Duration(-1)
	firstComplete := time.Duration(-1)
	resetAfterComplete := false
	for _, sample := range samples {
		if sample.percent < minPercent {
			minPercent = sample.percent
		}
		if sample.percent > maxPercent {
			maxPercent = sample.percent
		}
		if firstAdvance < 0 && sample.percent > 0 {
			firstAdvance = sample.at
		}
		if firstComplete < 0 && sample.percent == 100 {
			firstComplete = sample.at
		} else if firstComplete >= 0 && sample.percent < 100 {
			resetAfterComplete = true
		}
	}
	return fmt.Sprintf("changes=%d range=%d..%d first=%d@%s last=%d@%s first>0=%s first100=%s reset-after-100=%t",
		len(samples), minPercent, maxPercent,
		samples[0].percent, samples[0].at.Round(time.Millisecond),
		samples[len(samples)-1].percent, samples[len(samples)-1].at.Round(time.Millisecond),
		formatRealCloudFoxLargeF5SampleTime(firstAdvance), formatRealCloudFoxLargeF5SampleTime(firstComplete), resetAfterComplete)
}

func formatRealCloudFoxLargeF5SampleTime(value time.Duration) string {
	if value < 0 {
		return "never"
	}
	return value.Round(time.Millisecond).String()
}

func runRealCloudFoxLargeF5Action(t *testing.T, source, destination vfs.VFS, name string, timeout time.Duration) (realCloudFoxLargeF5ProgressTrace, error) {
	t.Helper()
	realCloudFoxUIResetScreen()
	pf := realCloudFoxUIPanels(t, source, destination)
	defer pf.Close()
	left := pf.panels[0].(*FileSystemPanel)
	right := pf.panels[1].(*FileSystemPanel)
	realCloudFoxUIWaitPanelName(t, left, name, 3*time.Minute)
	realCloudFoxUIWait(t, 3*time.Minute, "large F5 destination panel to load", func() bool { return !right.isLoading })
	pf.activeIdx = 0
	left.SetFocus(true)
	right.SetFocus(false)

	trace := realCloudFoxLargeF5ProgressTrace{startedAt: time.Now()}
	// This is the registered production F5 handler. ConfirmCopy=false makes it
	// dispatch the same accepted operation without synthesizing a test-only
	// call to ExecuteFileOpAt or to a provider API.
	actionCopyMove(pf, false)
	err := awaitRealCloudFoxLargeF5Progress(&trace, timeout)
	trace.finished = time.Since(trace.startedAt)
	return trace, err
}

func awaitRealCloudFoxLargeF5Progress(trace *realCloudFoxLargeF5ProgressTrace, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var operationErr error

	for {
		activeProgress := false
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				if frame == nil || frame.IsDone() {
					continue
				}
				if dialog, ok := frame.(*FileOpProgressDialog); ok {
					activeProgress = true
					trace.sawDialog = true
					if dialog.pbCurrent.IsVisible() {
						trace.record(&trace.current, dialog.pbCurrent.Percent)
					}
					if dialog.pbTotal.IsVisible() {
						trace.record(&trace.total, dialog.pbTotal.Percent)
					}
					continue
				}
				title := strings.ToLower(strings.TrimSpace(frame.GetTitle()))
				if strings.Contains(title, "error") || strings.Contains(title, "ошиб") {
					// Do not retain or print the dialog body: a provider error may
					// contain a request URL. Closing Cancel/Ok cannot retry work.
					operationErr = errors.New("production file operation surfaced an error dialog (details intentionally omitted)")
					closeRealCrossCloudDialog(frame)
					continue
				}
				if frame.GetTitle() == " Warning " || frame.GetTitle() == " Rename " {
					operationErr = errors.New("production file operation surfaced an unexpected conflict dialog")
					closeRealCrossCloudDialog(frame)
				}
			}
		}

		if operationErr != nil && !activeProgress {
			return operationErr
		}
		if trace.sawDialog && !activeProgress {
			return operationErr
		}

		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-ticker.C:
		case <-deadline.C:
			cancelRealCrossCloudProgressDialogs()
			_ = quiesceRealCrossCloudFileOps(time.Minute)
			return errors.New("production file operation timed out and was cancelled without retry")
		}
	}
}

func assertRealCloudFoxLargeF5Progress(t *testing.T, direction string, trace realCloudFoxLargeF5ProgressTrace) {
	t.Helper()
	if !trace.sawDialog {
		t.Errorf("large F5 %s did not present the production progress dialog", direction)
		return
	}
	for label, samples := range map[string][]realCloudFoxLargeF5PercentSample{"current": trace.current, "total": trace.total} {
		if len(samples) == 0 {
			t.Errorf("large F5 %s exposed no visible %s progress samples", direction, label)
			continue
		}
		maxPercent := 0
		sawIntermediate := false
		for _, sample := range samples {
			if sample.percent > maxPercent {
				maxPercent = sample.percent
			}
			if sample.percent > 0 && sample.percent < 100 {
				sawIntermediate = true
			}
		}
		// FileDone intentionally clears the per-file tracker before the final
		// UI update, so current may reset without ever rendering 100. The total
		// bar is the operation-completion invariant; current must still prove
		// that bytes visibly advanced beyond zero.
		if label == "total" && maxPercent != 100 {
			t.Errorf("large F5 %s total progress stopped at %d%%", direction, maxPercent)
		}
		if label == "current" && maxPercent == 0 {
			t.Errorf("large F5 %s current progress never advanced beyond zero", direction)
		}
		if !sawIntermediate {
			t.Errorf("large F5 %s %s progress never exposed an intermediate percentage", direction, label)
		}
	}
}
