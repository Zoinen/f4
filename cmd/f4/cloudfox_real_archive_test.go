package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	archiveplugin "github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/plugins/cloudfox"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const (
	realCloudFoxArchiveEnv           = "F4_CLOUDFOX_REAL_ARCHIVE"
	realCloudFoxArchiveConfirmed     = "CONFIRMED"
	realCloudFoxArchivePathEnv       = "F4_CLOUDFOX_REAL_ARCHIVE_PATH"
	realCloudFoxArchiveMarkerEnv     = "F4_CLOUDFOX_REAL_ARCHIVE_MARKER"
	realCloudFoxArchiveMarkerSHAEnv  = "F4_CLOUDFOX_REAL_ARCHIVE_MARKER_SHA256"
	realCloudFoxArchiveMarkerSizeEnv = "F4_CLOUDFOX_REAL_ARCHIVE_MARKER_SIZE"

	realCloudFoxArchiveFolderPrefix = realCloudFoxUIFolderPrefix + "archive-"
)

// TestRealSavedCloudArchives exercises a caller-supplied .7z file through the
// saved Google Drive and Yandex.Disk connections, the production CloudVFS,
// the production archive provider and the production F3 viewer.
//
// This test is intentionally inert unless F4_CLOUDFOX_REAL_ARCHIVE has the
// exact value CONFIRMED. Before any profile, keyring, vault or network access,
// it also requires all of the following:
//
//	F4_CLOUDFOX_REAL_CONFIG_DIR          absolute saved-profile directory
//	F4_CLOUDFOX_REAL_ARCHIVE_PATH        absolute path to an existing .7z
//	F4_CLOUDFOX_REAL_ARCHIVE_MARKER      relative member path inside the .7z
//	F4_CLOUDFOX_REAL_ARCHIVE_MARKER_SIZE exact decimal member size
//	F4_CLOUDFOX_REAL_ARCHIVE_MARKER_SHA256 exact hexadecimal member SHA-256
//
// The ordinary CloudFox selector and vault variables documented by
// TestRealSavedCloudConnectionsUI are reused. The test first proves the marker
// against the local archive, then creates one UUID-named directory per cloud,
// uploads the archive once, and removes that exact directory once. It never
// retries a mutation. A read-only lookup after an uncertain mutation is used
// only to prove whether that exact unique object needs cleanup.
//
// All UI is hosted by vtui.NewSilentScreenBuf. The network meter records only
// aggregate request/byte counts; it never retains URLs, headers, bodies,
// settings, selectors, secret references or credentials.
func TestRealSavedCloudArchives(t *testing.T) {
	if os.Getenv(realCloudFoxArchiveEnv) != realCloudFoxArchiveConfirmed {
		t.Skip("real CloudFox archive mutations require explicit confirmation")
	}

	fixture := loadRealCloudFoxArchiveFixture(t)
	configDir := requireRealCloudFoxArchiveConfigDir(t)
	validateRealCloudFoxLocalArchive(t, &fixture)

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	originalConfig := AppConfig
	t.Cleanup(func() { AppConfig = originalConfig })
	AppConfig.ViewerAutodetectCodePage = false
	AppConfig.ViewerDefaultCodePage = 65001

	meter := installRealCloudFoxArchiveNetworkMeter(t)
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
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close real CloudFox archive plugin: %v", err)
		}
	})

	host := newRealCloudFoxUIHost()
	if err := plugin.Init(host); err != nil {
		t.Fatalf("initialize production CloudFox registrations for archive test: %v", err)
	}
	archiveSupport := &archiveplugin.ArchivePlugin{}
	if err := archiveSupport.Init(host); err != nil {
		t.Fatalf("initialize production archive registration: %v", err)
	}
	t.Cleanup(func() {
		if err := archiveSupport.Close(); err != nil {
			t.Errorf("close real archive plugin: %v", err)
		}
	})
	archiveProvider := findRealCloudFoxArchiveProvider(t, host)

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved CloudFox profiles for archive test: %v", err)
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
			runRealCloudFoxArchiveProvider(t, host, archiveProvider, meter, connection, fixture)
		})
	}
}

type realCloudFoxArchiveFixture struct {
	localPath   string
	archiveSize int64
	marker      string
	markerSize  int64
	markerHash  [sha256.Size]byte
	probes      []realCloudFoxArchiveProbe
}

type realCloudFoxArchiveProbe struct {
	offset int64
	data   []byte
}

func loadRealCloudFoxArchiveFixture(t *testing.T) realCloudFoxArchiveFixture {
	t.Helper()
	localPath := strings.TrimSpace(os.Getenv(realCloudFoxArchivePathEnv))
	if localPath == "" || !filepath.IsAbs(localPath) {
		t.Fatal("real CloudFox archive fixture path must be an explicit absolute path")
	}
	if !strings.EqualFold(filepath.Ext(localPath), ".7z") {
		t.Fatal("real CloudFox archive fixture must have the .7z extension")
	}
	info, err := os.Stat(localPath) // #nosec G703 -- this opt-in integration fixture must be an operator-supplied absolute path.
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		t.Fatal("real CloudFox archive fixture is not an available non-empty regular file")
	}

	marker := strings.ReplaceAll(strings.TrimSpace(os.Getenv(realCloudFoxArchiveMarkerEnv)), "\\", "/")
	cleanMarker := path.Clean(marker)
	if marker == "" || cleanMarker == "." || path.IsAbs(marker) || cleanMarker != marker || strings.HasPrefix(cleanMarker, "../") {
		t.Fatal("real CloudFox archive marker must be one clean relative slash-separated member path")
	}
	for _, part := range strings.Split(marker, "/") {
		if part == "" || part == "." || part == ".." {
			t.Fatal("real CloudFox archive marker contains an unsafe path component")
		}
	}

	rawSize := strings.TrimSpace(os.Getenv(realCloudFoxArchiveMarkerSizeEnv))
	markerSize, err := strconv.ParseInt(rawSize, 10, 64)
	if err != nil || markerSize <= 0 {
		t.Fatal("real CloudFox archive marker size must be an explicit positive decimal integer")
	}
	rawHash := strings.TrimSpace(os.Getenv(realCloudFoxArchiveMarkerSHAEnv))
	decodedHash, err := hex.DecodeString(rawHash)
	if err != nil || len(decodedHash) != sha256.Size || len(rawHash) != sha256.Size*2 {
		t.Fatal("real CloudFox archive marker SHA-256 must be exactly 64 hexadecimal characters")
	}
	var markerHash [sha256.Size]byte
	copy(markerHash[:], decodedHash)

	return realCloudFoxArchiveFixture{
		localPath: localPath, archiveSize: info.Size(), marker: marker,
		markerSize: markerSize, markerHash: markerHash,
	}
}

func requireRealCloudFoxArchiveConfigDir(t *testing.T) string {
	t.Helper()
	configDir := strings.TrimSpace(os.Getenv(realCloudFoxConfigEnv))
	if configDir == "" || !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox archive config directory must be an explicit absolute path")
	}
	info, err := os.Stat(configDir) // #nosec G703 -- this opt-in integration config must be an operator-supplied absolute directory.
	if err != nil || !info.IsDir() {
		t.Fatal("real CloudFox archive config directory is unavailable")
	}
	return configDir
}

func validateRealCloudFoxLocalArchive(t *testing.T, fixture *realCloudFoxArchiveFixture) {
	t.Helper()
	provider := &archiveplugin.ArchiveProvider{}
	osvfs := vfs.NewOSVFS(filepath.Dir(fixture.localPath))
	defer func() { _ = osvfs.Close() }() // The local VFS has no teardown state.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if !provider.CanOpen(ctx, osvfs, fixture.localPath) {
		t.Fatal("production archive provider does not recognize the local .7z fixture")
	}
	archiveVFS, err := provider.Open(ctx, osvfs, fixture.localPath)
	if err != nil {
		t.Fatalf("open local .7z fixture through production archive provider: %v", err)
	}
	defer func() { _ = archiveVFS.Close() }()
	markerPath, err := resolveRealCloudFoxArchiveMarker(ctx, archiveVFS, archiveVFS.GetPath(), fixture.marker, fixture.markerSize)
	if err != nil {
		t.Fatalf("locate expected marker in local .7z fixture: %v", err)
	}
	progress := &realCloudFoxArchiveProgress{}
	readerCtx := realCloudFoxArchiveProgressContext(ctx, progress)
	reader, err := archiveVFS.Open(readerCtx, markerPath)
	if err != nil {
		t.Fatalf("open expected marker in local .7z fixture: %v", err)
	}
	probes, gotHash, gotSize, readErr := inspectRealCloudFoxArchiveReader(readerCtx, reader, fixture.markerSize)
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("read expected marker in local .7z fixture: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("close expected marker in local .7z fixture: %v", closeErr)
	}
	if gotSize != fixture.markerSize || gotHash != fixture.markerHash {
		t.Fatal("local .7z marker does not match the explicit expected size and SHA-256")
	}
	fixture.probes = probes
}

func findRealCloudFoxArchiveProvider(t *testing.T, host *realCloudFoxUIHost) vfs.VFSProvider {
	t.Helper()
	var match vfs.VFSProvider
	for _, provider := range host.vfsProviders {
		if provider != nil && provider.Name() == "zipper/archive" {
			if match != nil {
				t.Fatal("production archive plugin registered its provider more than once")
			}
			match = provider
		}
	}
	if match == nil {
		t.Fatal("production archive plugin did not register its VFS provider")
	}
	return match
}

func runRealCloudFoxArchiveProvider(t *testing.T, host *realCloudFoxUIHost, archiveProvider vfs.VFSProvider, meter *realCloudFoxArchiveNetworkMeter, connection cloudfox.Connection, fixture realCloudFoxArchiveFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	manager := host.driveFactory()
	if manager == nil {
		t.Fatal("production CloudFox drive factory returned nil for archive test")
	}
	defer func() { _ = manager.Close() }() // Connection cleanup errors do not affect the assertions.
	managerItems, err := readRealCloudFoxUIDir(ctx, manager, manager.GetPath(), connection.Provider)
	if err != nil {
		t.Fatalf("list production CloudFox manager for archive test: %v", err)
	}
	foundConnection := false
	for _, item := range managerItems {
		if item.Name == connection.Name && item.IsDir {
			foundConnection = true
			break
		}
	}
	if !foundConnection {
		t.Fatal("saved archive-test connection is not exposed as a manager folder")
	}

	connectionPath := manager.Join(manager.GetPath(), connection.Name)
	var mounted vfs.VFS
	for _, provider := range host.vfsProviders {
		if provider == archiveProvider || !provider.CanOpen(ctx, manager, connectionPath) {
			continue
		}
		mounted, err = provider.Open(ctx, manager, connectionPath)
		break
	}
	if err != nil {
		t.Fatalf("mount saved archive-test connection: %v", err)
	}
	if mounted == nil {
		t.Fatal("no production CloudFox provider mounted the saved archive-test connection")
	}
	defer func() { _ = mounted.Close() }() // Connection cleanup errors do not affect the assertions.

	writeRoot := mounted
	if connection.Provider == cloudfox.ProviderGoogleDrive {
		rootItems, err := readRealCloudFoxUIDir(ctx, writeRoot, writeRoot.GetPath(), connection.Provider)
		if err != nil {
			t.Fatalf("list Google virtual root for archive test: %v", err)
		}
		myDrivePath := ""
		for _, item := range rootItems {
			if item.Name != "My Drive" || !item.IsDir {
				continue
			}
			myDrivePath = writeRoot.Join(writeRoot.GetPath(), item.Name)
			break
		}
		if myDrivePath == "" {
			t.Fatal("Google Drive archive test could not find canonical My Drive")
		}
		if err := writeRoot.SetPath(myDrivePath); err != nil {
			t.Fatalf("enter Google My Drive for archive test: %v", err)
		}
	}

	folderName := realCloudFoxArchiveFolderPrefix + string(connection.Provider) + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	workspaceCandidate := writeRoot.Join(writeRoot.GetPath(), folderName)
	if workspaceCandidate == "" || workspaceCandidate == writeRoot.GetPath() || !strings.HasPrefix(folderName, realCloudFoxArchiveFolderPrefix) {
		t.Fatal("CloudVFS produced an unsafe real archive workspace target")
	}
	workspacePath := ""
	creationTried := false
	defer func() {
		if creationTried {
			if workspacePath == "" {
				cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Minute)
				if discovered, ok := findRealCloudFoxUIWorkspace(cleanupCtx, writeRoot, connection.Provider, folderName); ok {
					workspacePath = discovered
				}
				cancelCleanup()
			}
			cleanupRealCloudFoxUIWorkspace(t, writeRoot, connection.Provider, folderName, workspacePath)
		}
	}()
	creationTried = true
	if err := writeRoot.MkDir(ctx, workspaceCandidate); err != nil {
		if discovered, ok := findRealCloudFoxUIWorkspace(ctx, writeRoot, connection.Provider, folderName); ok {
			workspacePath = discovered
		}
		t.Fatalf("create isolated real archive workspace: %v", err)
	}
	workspacePath = requireRealCloudFoxUIWorkspace(t, ctx, writeRoot, connection.Provider, folderName)

	workspace := writeRoot.Clone()
	if workspace == nil || workspace == writeRoot {
		t.Fatal("CloudVFS did not provide an independent archive workspace clone")
	}
	defer func() { _ = workspace.Close() }() // Connection cleanup errors do not affect the assertions.
	if err := workspace.SetPath(workspacePath); err != nil {
		t.Fatalf("enter isolated real archive workspace: %v", err)
	}

	remoteName := "fixture.7z"
	remoteCandidate := workspace.Join(workspace.GetPath(), remoteName)
	uploadProgress := &realCloudFoxArchiveProgress{}
	uploadCtx := realCloudFoxArchiveProgressContext(ctx, uploadProgress)
	uploadBefore := meter.snapshot()
	if err := uploadRealCloudFoxArchive(uploadCtx, workspace, remoteCandidate, fixture.localPath, fixture.archiveSize); err != nil {
		t.Fatalf("upload real .7z fixture once: %s", redactRealCloudFoxError(err))
	}
	uploadDelta := meter.snapshot().sub(uploadBefore)
	remotePath := requireRealCloudFoxUIFile(t, ctx, workspace, connection.Provider, remoteName, fixture.archiveSize)
	t.Logf("archive upload: requests=%d request-bytes=%d response-bytes=%d progress-samples=%d", uploadDelta.requests, uploadDelta.requestBytes, uploadDelta.responseBytes, uploadProgress.count())

	// CanOpen is exactly the decision made when Enter/Ctrl+PgDn is used on a
	// file-panel row. Keep collecting deeper results after recording a failure:
	// Open itself is still the production provider implementation and can expose
	// independent download, archive and viewer defects.
	if !archiveProvider.CanOpen(ctx, workspace, remotePath) {
		t.Errorf("production archive provider rejected the canonical remote .7z path; opaque cloud identities lose the extension needed by CanOpen")
	}

	firstProgress := &realCloudFoxArchiveProgress{}
	firstOpenCtx := realCloudFoxArchiveProgressContext(ctx, firstProgress)
	firstBefore := meter.snapshot()
	firstStarted := time.Now()
	firstArchive, err := archiveProvider.Open(firstOpenCtx, workspace, remotePath)
	firstDuration := time.Since(firstStarted)
	firstDelta := meter.snapshot().sub(firstBefore)
	if err != nil {
		t.Fatalf("open uploaded .7z through production archive provider: %s", redactRealCloudFoxError(err))
	}
	firstClosed := false
	defer func() {
		if !firstClosed {
			_ = firstArchive.Close()
		}
	}()

	firstRoot := usableRealCloudFoxArchiveRoot(t, firstArchive, remotePath)
	firstMarker, err := resolveRealCloudFoxArchiveMarker(ctx, firstArchive, firstRoot, fixture.marker, fixture.markerSize)
	if err != nil {
		t.Fatalf("list expected marker through first remote archive: %v", err)
	}
	verifyRealCloudFoxArchiveMarker(t, ctx, firstArchive, firstMarker, fixture)
	runRealCloudFoxArchiveViewer(t, firstArchive, firstMarker, fixture)
	if err := firstArchive.Close(); err != nil {
		t.Fatalf("close first remote archive instance: %v", err)
	}
	firstClosed = true

	secondProgress := &realCloudFoxArchiveProgress{}
	secondOpenCtx := realCloudFoxArchiveProgressContext(ctx, secondProgress)
	secondBefore := meter.snapshot()
	secondStarted := time.Now()
	secondArchive, err := archiveProvider.Open(secondOpenCtx, workspace, remotePath)
	secondDuration := time.Since(secondStarted)
	secondDelta := meter.snapshot().sub(secondBefore)
	if err != nil {
		t.Fatalf("reopen unchanged .7z in the same CloudFox session: %s", redactRealCloudFoxError(err))
	}
	defer func() { _ = secondArchive.Close() }()
	secondRoot := usableRealCloudFoxArchiveRoot(t, secondArchive, remotePath)
	secondMarker, err := resolveRealCloudFoxArchiveMarker(ctx, secondArchive, secondRoot, fixture.marker, fixture.markerSize)
	if err != nil {
		t.Fatalf("list expected marker through reopened remote archive: %v", err)
	}
	verifyRealCloudFoxArchiveMarker(t, ctx, secondArchive, secondMarker, fixture)

	t.Logf("archive first open: duration=%s requests=%d response-bytes=%d opening-progress-samples=%d", firstDuration.Round(time.Millisecond), firstDelta.requests, firstDelta.responseBytes, firstProgress.count())
	t.Logf("archive same-session reopen: duration=%s requests=%d response-bytes=%d opening-progress-samples=%d", secondDuration.Round(time.Millisecond), secondDelta.requests, secondDelta.responseBytes, secondProgress.count())
	if fixture.archiveSize >= 8<<20 && firstProgress.count() == 0 {
		t.Errorf("opening the large remote archive emitted no ProgressKey/TaskReporter samples; ArchiveProvider.Open discarded the caller context")
	}
	if reasonableRequestCeiling := fixture.archiveSize/(1<<20) + 64; fixture.archiveSize >= 8<<20 && firstDelta.requests > reasonableRequestCeiling {
		t.Errorf("opening one %d-byte archive required %d HTTP requests; the remote reader is being copied in pathologically small ranges", fixture.archiveSize, firstDelta.requests)
	}
	if fixture.archiveSize >= 8<<20 && secondDelta.responseBytes >= fixture.archiveSize*3/4 {
		t.Errorf("unchanged same-session archive reopen transferred %d response bytes for a %d-byte archive; no effective download cache was observed", secondDelta.responseBytes, fixture.archiveSize)
	}
}

func uploadRealCloudFoxArchive(ctx context.Context, workspace vfs.VFS, remotePath, localPath string, expectedSize int64) error {
	local, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = local.Close() }()
	remote, err := workspace.Create(ctx, remotePath)
	if err != nil {
		return err
	}
	written, copyErr := io.CopyBuffer(remote, local, make([]byte, 1<<20))
	closeErr := remote.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != expectedSize {
		return fmt.Errorf("local archive copy wrote %d bytes, want %d", written, expectedSize)
	}
	return nil
}

func usableRealCloudFoxArchiveRoot(t *testing.T, archiveVFS vfs.VFS, sourcePath string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	userRoot := archiveVFS.GetPath()
	if err := archiveVFS.ReadDir(ctx, userRoot, func([]vfs.VFSItem) {}); err == nil {
		return userRoot
	} else if userRoot == sourcePath {
		t.Fatalf("list remote archive root through production GetPath: %v", err)
	} else {
		t.Errorf("ArchiveVFS.GetPath produced an unusable remote path; source=%q get-path=%q: %v", redactRealCloudFoxArchivePath(sourcePath), redactRealCloudFoxArchivePath(userRoot), err)
	}
	if err := archiveVFS.ReadDir(ctx, sourcePath, func([]vfs.VFSItem) {}); err != nil {
		t.Fatalf("list remote archive root even through its original canonical identity: %v", err)
	}
	return sourcePath
}

// redactRealCloudFoxArchivePath deliberately emits only the shape needed to
// diagnose URI corruption. Connection IDs and provider object IDs stay out of
// test logs.
func redactRealCloudFoxArchivePath(value string) string {
	if strings.HasPrefix(strings.ToLower(value), "cloud://") {
		return "cloud://<redacted>"
	}
	if strings.HasPrefix(strings.ToLower(value), "cloud:\\") {
		return "cloud:\\<redacted>"
	}
	return "<non-cloud-path>"
}

func resolveRealCloudFoxArchiveMarker(ctx context.Context, archiveVFS vfs.VFS, root, marker string, expectedSize int64) (string, error) {
	current := root
	parts := strings.Split(marker, "/")
	for index, part := range parts {
		var items []vfs.VFSItem
		if err := archiveVFS.ReadDir(ctx, current, func(chunk []vfs.VFSItem) { items = append(items, chunk...) }); err != nil {
			return "", err
		}
		matches := 0
		var found vfs.VFSItem
		for _, item := range items {
			if item.Name == part {
				matches++
				found = item
			}
		}
		if matches != 1 {
			return "", fmt.Errorf("archive path component %q has %d exact rows", part, matches)
		}
		last := index == len(parts)-1
		if !last && !found.IsDir {
			return "", fmt.Errorf("archive path component %q is not a directory", part)
		}
		if last && (found.IsDir || found.Size != expectedSize) {
			return "", fmt.Errorf("archive marker metadata directory=%t size=%d want=%d", found.IsDir, found.Size, expectedSize)
		}
		current = joinRealCloudFoxArchivePath(root, strings.Join(parts[:index+1], "/"))
	}
	stat, err := archiveVFS.Stat(ctx, current)
	if err != nil {
		return "", err
	}
	if stat.IsDir || stat.Size != expectedSize {
		return "", fmt.Errorf("archive marker stat directory=%t size=%d want=%d", stat.IsDir, stat.Size, expectedSize)
	}
	return current, nil
}

func joinRealCloudFoxArchivePath(root, member string) string {
	if strings.Contains(root, "://") {
		return strings.TrimRight(root, "/\\") + "/" + strings.TrimLeft(member, "/")
	}
	return filepath.ToSlash(filepath.Join(root, filepath.FromSlash(member)))
}

func verifyRealCloudFoxArchiveMarker(t *testing.T, ctx context.Context, archiveVFS vfs.VFS, markerPath string, fixture realCloudFoxArchiveFixture) {
	t.Helper()
	progress := &realCloudFoxArchiveProgress{}
	readerCtx := realCloudFoxArchiveProgressContext(ctx, progress)
	reader, err := archiveVFS.Open(readerCtx, markerPath)
	if err != nil {
		t.Fatalf("open marker through remote archive: %v", err)
	}
	probes, gotHash, gotSize, readErr := inspectRealCloudFoxArchiveReader(readerCtx, reader, fixture.markerSize)
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("hash marker through remote archive: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("close marker through remote archive: %v", closeErr)
	}
	if gotSize != fixture.markerSize || gotHash != fixture.markerHash {
		t.Fatal("remote archive marker changed size or SHA-256")
	}
	if !equalRealCloudFoxArchiveProbes(probes, fixture.probes) {
		t.Fatal("remote archive marker returned unexpected arbitrary-seek bytes")
	}
	if progress.count() == 0 || progress.maxPercent() != 100 {
		t.Errorf("archive member extraction did not report a complete 100%% progress sequence")
	}
}

func inspectRealCloudFoxArchiveReader(ctx context.Context, reader vfs.ReadAtCloser, expectedSize int64) ([]realCloudFoxArchiveProbe, [sha256.Size]byte, int64, error) {
	var zero [sha256.Size]byte
	if reader.Size() != expectedSize {
		return nil, zero, 0, fmt.Errorf("archive reader size=%d want=%d", reader.Size(), expectedSize)
	}
	probeOffsets := []int64{0, expectedSize / 2, expectedSize - 4096}
	seen := make(map[int64]bool)
	probes := make([]realCloudFoxArchiveProbe, 0, len(probeOffsets))
	for _, offset := range probeOffsets {
		if offset < 0 {
			offset = 0
		}
		if offset >= expectedSize || seen[offset] {
			continue
		}
		seen[offset] = true
		length := int64(4096)
		if remaining := expectedSize - offset; remaining < length {
			length = remaining
		}
		data := make([]byte, int(length))
		n, err := reader.ReadAt(ctx, data, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, zero, 0, err
		}
		if n != len(data) {
			return nil, zero, 0, io.ErrUnexpectedEOF
		}
		probes = append(probes, realCloudFoxArchiveProbe{offset: offset, data: data})
	}

	hasher := sha256.New()
	buffer := make([]byte, 1<<20)
	var total int64
	for {
		n, err := reader.Read(ctx, buffer)
		if n > 0 {
			_, _ = hasher.Write(buffer[:n])
			total += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, zero, total, err
		}
		if n == 0 {
			return nil, zero, total, io.ErrNoProgress
		}
	}
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	return probes, sum, total, nil
}

func equalRealCloudFoxArchiveProbes(first, second []realCloudFoxArchiveProbe) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index].offset != second[index].offset || !bytes.Equal(first[index].data, second[index].data) {
			return false
		}
	}
	return true
}

func runRealCloudFoxArchiveViewer(t *testing.T, archiveVFS vfs.VFS, markerPath string, fixture realCloudFoxArchiveFixture) {
	t.Helper()
	realCloudFoxUIResetScreen()
	pf := realCloudFoxUIBarePanels(t)
	defer pf.Close()
	actionOpenViewer(pf, archiveVFS, markerPath)
	var viewer *ViewerView
	realCloudFoxUIWait(t, 10*time.Minute, "remote archive marker F3 viewer to open", func() bool {
		viewer, _ = findOpenedViewer(archiveVFS, markerPath)
		return viewer != nil
	})
	defer viewer.Close()
	if viewer.backend.Size() != fixture.markerSize {
		t.Fatalf("archive marker F3 viewer size=%d want=%d", viewer.backend.Size(), fixture.markerSize)
	}
	for _, probe := range fixture.probes {
		got := realCloudFoxUIViewerRead(t, viewer, probe.offset, len(probe.data))
		if !equalRealCloudFoxArchiveProbes(
			[]realCloudFoxArchiveProbe{{offset: probe.offset, data: got}},
			[]realCloudFoxArchiveProbe{probe},
		) {
			t.Fatalf("archive marker F3 viewer changed bytes at offset %d", probe.offset)
		}
	}
	viewer.SetPosition(0, 0, 119, 38)
	if !viewer.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END}) {
		t.Fatal("archive marker F3 viewer did not handle End navigation")
	}
	realCloudFoxUIWait(t, 5*time.Minute, "archive marker F3 viewer End navigation", func() bool {
		return !viewer.Busy
	})
	if fixture.markerSize >= 256<<10 && viewer.TopOffset == 0 {
		t.Error("archive marker F3 viewer handled End but did not move in a large member")
	}
}

type realCloudFoxArchiveProgress struct {
	mu       sync.Mutex
	percents []int
}

func (p *realCloudFoxArchiveProgress) record(percent int) {
	p.mu.Lock()
	p.percents = append(p.percents, percent)
	p.mu.Unlock()
}

func (p *realCloudFoxArchiveProgress) callback(_ string, percent int) { p.record(percent) }
func (*realCloudFoxArchiveProgress) UpdateScan(string, int64, int64)  {}
func (p *realCloudFoxArchiveProgress) UpdateTransfer(_ string, _ string, currentPct int, _ string, totalPct int, _ string) {
	if totalPct >= 0 {
		p.record(totalPct)
	} else {
		p.record(currentPct)
	}
}
func (*realCloudFoxArchiveProgress) IsCancelled() bool { return false }
func (p *realCloudFoxArchiveProgress) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.percents)
}
func (p *realCloudFoxArchiveProgress) maxPercent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	maximum := -1
	for _, percent := range p.percents {
		if percent > maximum {
			maximum = percent
		}
	}
	return maximum
}

func realCloudFoxArchiveProgressContext(ctx context.Context, progress *realCloudFoxArchiveProgress) context.Context {
	ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(progress.callback))
	return context.WithValue(ctx, vfs.ReporterKey, vfs.TaskReporter(progress))
}

type realCloudFoxArchiveNetworkSnapshot struct {
	requests      int64
	requestBytes  int64
	responseBytes int64
}

func (s realCloudFoxArchiveNetworkSnapshot) sub(previous realCloudFoxArchiveNetworkSnapshot) realCloudFoxArchiveNetworkSnapshot {
	return realCloudFoxArchiveNetworkSnapshot{
		requests:      s.requests - previous.requests,
		requestBytes:  s.requestBytes - previous.requestBytes,
		responseBytes: s.responseBytes - previous.responseBytes,
	}
}

type realCloudFoxArchiveNetworkMeter struct {
	base          http.RoundTripper
	requests      atomic.Int64
	requestBytes  atomic.Int64
	responseBytes atomic.Int64
}

func (m *realCloudFoxArchiveNetworkMeter) RoundTrip(request *http.Request) (*http.Response, error) {
	m.requests.Add(1)
	clone := request.Clone(request.Context())
	if request.Body != nil {
		clone.Body = &realCloudFoxArchiveMeteredBody{ReadCloser: request.Body, bytes: &m.requestBytes}
	}
	response, err := m.base.RoundTrip(clone)
	if response != nil && response.Body != nil {
		response.Body = &realCloudFoxArchiveMeteredBody{ReadCloser: response.Body, bytes: &m.responseBytes}
	}
	return response, err
}

func (m *realCloudFoxArchiveNetworkMeter) snapshot() realCloudFoxArchiveNetworkSnapshot {
	return realCloudFoxArchiveNetworkSnapshot{
		requests: m.requests.Load(), requestBytes: m.requestBytes.Load(), responseBytes: m.responseBytes.Load(),
	}
}

type realCloudFoxArchiveMeteredBody struct {
	io.ReadCloser
	bytes *atomic.Int64
}

func (b *realCloudFoxArchiveMeteredBody) Read(buffer []byte) (int, error) {
	n, err := b.ReadCloser.Read(buffer)
	b.bytes.Add(int64(n))
	return n, err
}

func installRealCloudFoxArchiveNetworkMeter(t *testing.T) *realCloudFoxArchiveNetworkMeter {
	t.Helper()
	original := http.DefaultTransport
	base := original
	rawRetries := strings.TrimSpace(os.Getenv(realCloudFoxDialRetryEnv))
	if rawRetries != "" && rawRetries != "0" {
		retries, err := strconv.Atoi(rawRetries)
		if err != nil || retries < 1 || retries > 10 {
			t.Fatal("real archive TCP dial retries must be an integer from 0 through 10")
		}
		transport, ok := original.(*http.Transport)
		if !ok {
			t.Fatal("real archive diagnostic dial retries require the standard HTTP transport")
		}
		clone := transport.Clone()
		baseDial := clone.DialContext
		if baseDial == nil {
			t.Fatal("real archive diagnostic transport has no TCP dialer")
		}
		clone.DialContext = realCloudFoxUIRetryTCPDialContext(baseDial, retries, 250*time.Millisecond)
		base = clone
	}
	meter := &realCloudFoxArchiveNetworkMeter{base: base}
	http.DefaultTransport = meter
	t.Cleanup(func() {
		http.DefaultTransport = original
		if transport, ok := base.(*http.Transport); ok && base != original {
			transport.CloseIdleConnections()
		}
	})
	return meter
}
