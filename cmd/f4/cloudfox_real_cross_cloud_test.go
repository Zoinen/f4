package main

import (
	"context"
	"crypto/sha256"
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
	realCrossCloudEnv       = "F4_CLOUDFOX_REAL_CROSS_CLOUD"
	realCrossCloudConfirmed = "CONFIRMED"
	realCrossCloudPrefix    = realCloudFoxUIFolderPrefix + "cross-"
)

var errRealCrossCloudOperationTimeout = errors.New("real cross-cloud file operation did not quiesce")

// TestRealSavedCloudCrossProviderF5 performs destructive, real-account
// Google Drive <-> Yandex.Disk tests through CloudFox's production ManagerVFS,
// VFSProvider, CloudVFS, F5 action, ExecuteFileOpAt and conflict dialogs.
//
// The test is intentionally separate from TestRealSavedCloudConnectionsUI and
// cannot run accidentally with its opt-in. It does not inspect profiles or
// credentials until F4_CLOUDFOX_REAL_CROSS_CLOUD has the exact value
// CONFIRMED and F4_CLOUDFOX_REAL_CONFIG_DIR names an absolute directory.
// Every frame is hosted by SilentScreenBuf; no terminal, browser or native
// window is opened. The optional Yandex dial-retry environment variable
// documented by TestRealSavedCloudConnectionsUI affects only failed TCP
// connection establishment, before an HTTP request can exist. By default the
// exact production transports are used. Mutations are never retried here.
func TestRealSavedCloudCrossProviderF5(t *testing.T) {
	if os.Getenv(realCrossCloudEnv) != realCrossCloudConfirmed {
		t.Skip("real cross-cloud mutations require explicit confirmation")
	}

	configDir := strings.TrimSpace(os.Getenv(realCloudFoxConfigEnv))
	if configDir == "" {
		t.Fatal("real cross-cloud config directory is required")
	}
	if !filepath.IsAbs(configDir) {
		t.Fatal("real cross-cloud config directory must be absolute")
	}
	info, err := os.Stat(configDir) // #nosec G703 -- this opt-in integration config must be an operator-supplied absolute directory.
	if err != nil || !info.IsDir() {
		t.Fatal("real cross-cloud config directory is unavailable")
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
			t.Errorf("close real cross-cloud plugin: %v", err)
		}
	})

	host := newRealCloudFoxUIHost()
	if err := plugin.Init(host); err != nil {
		t.Fatalf("initialize production CloudFox registrations: %v", err)
	}
	if host.driveFactory == nil || host.uriProvider == nil || len(host.vfsProviders) == 0 {
		t.Fatal("CloudFox did not register its production drive providers")
	}

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved CloudFox profiles: %v", err)
	}
	googleConnection := selectRealCloudFoxUIConnection(t, connections, cloudfox.ProviderGoogleDrive, realCloudFoxGoogleEnv)
	yandexConnection := selectRealCloudFoxUIConnection(t, connections, cloudfox.ProviderYandexDisk, realCloudFoxYandexEnv)

	googleRoot := mountRealCrossCloudWriteRoot(t, host, googleConnection)
	yandexRoot := mountRealCrossCloudWriteRoot(t, host, yandexConnection)
	runID := strings.ReplaceAll(uuid.NewString(), "-", "")
	google := createRealCrossCloudWorkspace(t, googleRoot, cloudfox.ProviderGoogleDrive, "google-"+runID)
	yandex := createRealCrossCloudWorkspace(t, yandexRoot, cloudfox.ProviderYandexDisk, "yandex-"+runID)

	directions := []struct {
		name string
		src  *realCrossCloudEndpoint
		dst  *realCrossCloudEndpoint
	}{
		{name: "google-to-yandex", src: google, dst: yandex},
		{name: "yandex-to-google", src: yandex, dst: google},
	}

	unsafeToContinue := false
	for _, direction := range directions {
		direction := direction
		t.Run(direction.name, func(t *testing.T) {
			run := func(name string, operation func(*testing.T) error) {
				if unsafeToContinue {
					return
				}
				t.Run(name, func(t *testing.T) {
					if err := operation(t); err != nil {
						if errors.Is(err, errRealCrossCloudOperationTimeout) {
							unsafeToContinue = true
							direction.src.cleanupBlocked = true
							direction.dst.cleanupBlocked = true
						}
						t.Fatal(err)
					}
				})
			}

			run("file-copy", func(t *testing.T) error {
				name := "cross-" + direction.name + "-file.bin"
				payload := realCrossCloudPayload(direction.name+"/file", 384*1024+137)
				putRealCrossCloudFile(t, direction.src, direction.src.workspace.GetPath(), name, payload)
				if err := runRealCrossCloudF5(t, direction.src, direction.dst, name, nil); err != nil {
					return fmt.Errorf("production F5 file copy: %w", err)
				}
				assertRealCrossCloudFile(t, direction.dst, direction.dst.workspace.GetPath(), name, payload)
				return nil
			})

			run("recursive-directory-copy", func(t *testing.T) error {
				name := "cross-" + direction.name + "-tree"
				fixture := createRealCrossCloudTree(t, direction.src, name, direction.name+"/tree")
				if err := runRealCrossCloudF5(t, direction.src, direction.dst, name, nil); err != nil {
					diagnostic := diagnoseRealCrossCloudRecursiveDestination(t, direction.dst, name)
					return fmt.Errorf("production F5 recursive directory copy: %w (%s)", err, diagnostic)
				}
				verifyRealCrossCloudTree(t, direction.dst, name, fixture)
				return nil
			})

			for _, conflict := range []struct {
				name   string
				choice realCrossCloudConflict
			}{
				{name: "overwrite-conflict", choice: realCrossCloudConflict{button: "Overwrite"}},
				{name: "skip-conflict", choice: realCrossCloudConflict{button: "Skip"}},
				{name: "rename-conflict", choice: realCrossCloudConflict{button: "Rename", rename: "cross-" + direction.name + "-renamed.bin"}},
			} {
				conflict := conflict
				run(conflict.name, func(t *testing.T) error {
					name := "cross-" + direction.name + "-" + conflict.name + ".bin"
					sourcePayload := realCrossCloudPayload(direction.name+"/"+conflict.name+"/source", 96*1024+19)
					destinationPayload := realCrossCloudPayload(direction.name+"/"+conflict.name+"/destination", 80*1024+23)
					putRealCrossCloudFile(t, direction.src, direction.src.workspace.GetPath(), name, sourcePayload)
					putRealCrossCloudFile(t, direction.dst, direction.dst.workspace.GetPath(), name, destinationPayload)
					if conflict.choice.rename != "" {
						assertRealCrossCloudAbsent(t, direction.dst, direction.dst.workspace.GetPath(), conflict.choice.rename)
					}
					if err := runRealCrossCloudF5(t, direction.src, direction.dst, name, &conflict.choice); err != nil {
						return fmt.Errorf("production F5 %s: %w", conflict.name, err)
					}
					switch conflict.choice.button {
					case "Overwrite":
						assertRealCrossCloudFile(t, direction.dst, direction.dst.workspace.GetPath(), name, sourcePayload)
					case "Skip":
						assertRealCrossCloudFile(t, direction.dst, direction.dst.workspace.GetPath(), name, destinationPayload)
					case "Rename":
						assertRealCrossCloudFile(t, direction.dst, direction.dst.workspace.GetPath(), name, destinationPayload)
						assertRealCrossCloudFile(t, direction.dst, direction.dst.workspace.GetPath(), conflict.choice.rename, sourcePayload)
					}
					return nil
				})
			}
		})
		if unsafeToContinue {
			t.Fatalf("stopped after a file operation failed to quiesce; later real mutations were not started")
		}
	}
}

type realCrossCloudEndpoint struct {
	provider       cloudfox.ProviderType
	root           vfs.VFS
	workspace      vfs.VFS
	workspaceName  string
	workspacePath  string
	creationTried  bool
	cleanupBlocked bool
}

func mountRealCrossCloudWriteRoot(t *testing.T, host *realCloudFoxUIHost, connection cloudfox.Connection) vfs.VFS {
	t.Helper()
	manager := host.driveFactory()
	if manager == nil {
		t.Fatal("production CloudFox drive factory returned nil")
	}
	t.Cleanup(func() { _ = manager.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	managerItems, err := readRealCrossCloudDir(ctx, manager, manager.GetPath())
	if err != nil {
		t.Fatalf("list production CloudFox connection manager: %v", err)
	}
	found := false
	for _, item := range managerItems {
		if item.Name == connection.Name && item.IsDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("saved connection is not exposed as a folder by the CloudFox manager")
	}

	connectionPath := manager.Join(manager.GetPath(), connection.Name)
	var mounted vfs.VFS
	for _, provider := range host.vfsProviders {
		if !provider.CanOpen(ctx, manager, connectionPath) {
			continue
		}
		mounted, err = provider.Open(ctx, manager, connectionPath)
		break
	}
	if err != nil {
		t.Fatalf("open saved connection through production VFS provider: %v", err)
	}
	if mounted == nil {
		t.Fatal("no production VFS provider accepted the saved connection folder")
	}
	t.Cleanup(func() { _ = mounted.Close() })
	if !strings.HasPrefix(mounted.GetPath(), connection.Name+":") || strings.Contains(mounted.GetPath(), "cloud://") || strings.Contains(strings.ToLower(mounted.GetPath()), strings.ToLower(connection.ID)) {
		t.Fatalf("mounted CloudVFS exposed a non-visual path")
	}

	if connection.Provider == cloudfox.ProviderGoogleDrive {
		items, err := readRealCrossCloudDir(ctx, mounted, mounted.GetPath())
		if err != nil {
			t.Fatalf("list Google Drive virtual root: %v", err)
		}
		myDrivePath := ""
		for _, item := range items {
			if item.Name != "My Drive" || !item.IsDir {
				continue
			}
			myDrivePath = mounted.Join(mounted.GetPath(), item.Name)
			break
		}
		if myDrivePath == "" {
			t.Fatal("Google Drive virtual root does not expose canonical My Drive")
		}
		if err := mounted.SetPath(myDrivePath); err != nil {
			t.Fatalf("enter Google My Drive through CloudVFS: %v", err)
		}
	}
	if _, err := mounted.Stat(ctx, mounted.GetPath()); err != nil {
		t.Fatalf("stat writable cloud root: %v", err)
	}
	return mounted
}

func createRealCrossCloudWorkspace(t *testing.T, root vfs.VFS, provider cloudfox.ProviderType, suffix string) *realCrossCloudEndpoint {
	t.Helper()
	name := realCrossCloudPrefix + suffix
	if !strings.HasPrefix(name, realCrossCloudPrefix) {
		t.Fatal("unsafe real cross-cloud workspace name")
	}
	endpoint := &realCrossCloudEndpoint{provider: provider, root: root, workspaceName: name}
	t.Cleanup(func() { cleanupRealCrossCloudWorkspace(t, endpoint) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if path, found, err := findRealCrossCloudDirectory(ctx, root, provider, name); err != nil {
		t.Fatalf("check isolated real cross-cloud workspace name: %v", err)
	} else if found {
		t.Fatalf("refusing to reuse an existing real cross-cloud workspace (%s)", root.Base(path))
	}
	candidate := root.Join(root.GetPath(), name)
	if candidate == "" || strings.Contains(candidate, "cloud://") {
		t.Fatal("CloudVFS produced an unsafe non-visual real cross-cloud workspace target")
	}
	endpoint.creationTried = true
	if err := root.MkDir(ctx, candidate); err != nil {
		// The mutation is not retried. One exact, read-only discovery pass is
		// allowed solely so a response-lost create can still be cleaned up.
		if discovered, found, discoverErr := findRealCrossCloudDirectory(ctx, root, provider, name); discoverErr == nil && found {
			endpoint.workspacePath = discovered
		}
		t.Fatalf("create isolated real cross-cloud workspace: %v", err)
	}
	path, found, err := findRealCrossCloudDirectory(ctx, root, provider, name)
	if err != nil {
		t.Fatalf("discover isolated real cross-cloud workspace: %v", err)
	}
	if !found {
		t.Fatal("created real cross-cloud workspace was not returned as one exact directory row")
	}
	endpoint.workspacePath = path
	workspace := root.Clone()
	if workspace == nil || workspace == root {
		t.Fatal("CloudVFS did not provide an independent real cross-cloud workspace clone")
	}
	endpoint.workspace = workspace
	t.Cleanup(func() { _ = workspace.Close() })
	if err := workspace.SetPath(path); err != nil {
		t.Fatalf("enter isolated real cross-cloud workspace: %v", err)
	}
	return endpoint
}

func cleanupRealCrossCloudWorkspace(t *testing.T, endpoint *realCrossCloudEndpoint) {
	t.Helper()
	if endpoint == nil || endpoint.root == nil || !strings.HasPrefix(endpoint.workspaceName, realCrossCloudPrefix) {
		if endpoint != nil && endpoint.workspacePath != "" {
			t.Errorf("refusing unsafe real cross-cloud workspace cleanup")
		}
		return
	}
	if !endpoint.creationTried {
		return
	}
	if endpoint.cleanupBlocked {
		t.Errorf("skipping real cross-cloud cleanup because a file operation is still active")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	path, found, err := findRealCrossCloudDirectory(ctx, endpoint.root, endpoint.provider, endpoint.workspaceName)
	if err != nil {
		t.Errorf("prove exact real cross-cloud workspace identity for cleanup: %v", err)
		return
	}
	if !found {
		if endpoint.workspacePath != "" {
			t.Errorf("isolated real cross-cloud workspace disappeared before cleanup proof")
		}
		return
	}
	if endpoint.workspacePath != "" && path != endpoint.workspacePath {
		t.Errorf("refusing real cross-cloud cleanup after canonical identity changed")
		return
	}
	endpoint.workspacePath = path
	// Exactly one permanent recursive removal is submitted. In particular, an
	// uncertain timeout is reported and never followed by a second mutation.
	if err := endpoint.root.Remove(ctx, path); err != nil {
		t.Errorf("remove isolated real cross-cloud workspace: %v", err)
		return
	}
	if _, found, err := findRealCrossCloudDirectory(ctx, endpoint.root, endpoint.provider, endpoint.workspaceName); err != nil {
		t.Errorf("verify real cross-cloud workspace cleanup: %v", err)
	} else if found {
		t.Errorf("isolated real cross-cloud workspace remains after cleanup")
	}
}

func findRealCrossCloudDirectory(ctx context.Context, filesystem vfs.VFS, provider cloudfox.ProviderType, name string) (string, bool, error) {
	items, err := readRealCrossCloudDir(ctx, filesystem, filesystem.GetPath())
	if err != nil {
		return "", false, err
	}
	count := 0
	for _, item := range items {
		if item.Name == name {
			count++
			if !item.IsDir {
				return "", false, fmt.Errorf("exact workspace row is not a directory")
			}
		}
	}
	if count == 0 {
		return "", false, nil
	}
	if count != 1 {
		return "", false, fmt.Errorf("workspace name has %d exact rows", count)
	}
	path := filesystem.Join(filesystem.GetPath(), name)
	if path == "" || strings.Contains(path, "cloud://") {
		return "", false, errors.New("workspace row has a non-visual public identity")
	}
	stat, err := filesystem.Stat(ctx, path)
	if err != nil {
		return "", false, err
	}
	if !stat.IsDir || stat.Name != name {
		return "", false, errors.New("workspace canonical stat does not match its exact row")
	}
	return path, true, nil
}

func readRealCrossCloudDir(ctx context.Context, filesystem vfs.VFS, path string) ([]vfs.VFSItem, error) {
	var items []vfs.VFSItem
	err := filesystem.ReadDir(ctx, path, func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	})
	return items, err
}

func putRealCrossCloudFile(t *testing.T, endpoint *realCrossCloudEndpoint, parent, name string, payload []byte) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	assertRealCrossCloudEntryAbsent(t, ctx, endpoint, parent, name)
	path := endpoint.workspace.Join(parent, name)
	if err := writeRealCloudFoxUIFile(ctx, endpoint.workspace, path, payload); err != nil {
		t.Fatalf("create isolated real cross-cloud fixture: %s", redactRealCloudFoxError(err))
	}
	canonical, item := requireRealCrossCloudEntry(t, ctx, endpoint, parent, name, false)
	if item.Size != int64(len(payload)) {
		t.Fatalf("real cross-cloud fixture size=%d, want=%d", item.Size, len(payload))
	}
	return canonical
}

func createRealCrossCloudDirectory(t *testing.T, endpoint *realCrossCloudEndpoint, parent, name string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	assertRealCrossCloudEntryAbsent(t, ctx, endpoint, parent, name)
	candidate := endpoint.workspace.Join(parent, name)
	if err := endpoint.workspace.MkDir(ctx, candidate); err != nil {
		t.Fatalf("create isolated real cross-cloud fixture directory: %v", err)
	}
	path, _ := requireRealCrossCloudEntry(t, ctx, endpoint, parent, name, true)
	return path
}

func requireRealCrossCloudEntry(t *testing.T, ctx context.Context, endpoint *realCrossCloudEndpoint, parent, name string, isDir bool) (string, vfs.VFSItem) {
	t.Helper()
	items, err := readRealCrossCloudDir(ctx, endpoint.workspace, parent)
	if err != nil {
		t.Fatalf("list real cross-cloud fixture directory: %v", err)
	}
	count := 0
	for _, item := range items {
		if item.Name == name {
			count++
			if item.IsDir != isDir {
				t.Fatalf("real cross-cloud row directory=%t, want=%t", item.IsDir, isDir)
			}
		}
	}
	if count != 1 {
		t.Fatalf("real cross-cloud directory contains %d exact %q rows, want 1", count, name)
	}
	path := endpoint.workspace.Join(parent, name)
	if path == "" || strings.Contains(path, "cloud://") {
		t.Fatal("real cross-cloud row has a non-visual public identity")
	}
	stat, err := endpoint.workspace.Stat(ctx, path)
	if err != nil {
		t.Fatalf("stat real cross-cloud row: %v", err)
	}
	if stat.Name != name || stat.IsDir != isDir {
		t.Fatal("real cross-cloud canonical stat does not match its exact row")
	}
	return path, stat
}

func assertRealCrossCloudEntryAbsent(t *testing.T, ctx context.Context, endpoint *realCrossCloudEndpoint, parent, name string) {
	t.Helper()
	items, err := readRealCrossCloudDir(ctx, endpoint.workspace, parent)
	if err != nil {
		t.Fatalf("list real cross-cloud destination before mutation: %v", err)
	}
	for _, item := range items {
		if item.Name == name {
			t.Fatalf("refusing to reuse existing real cross-cloud fixture %q", name)
		}
	}
}

func assertRealCrossCloudAbsent(t *testing.T, endpoint *realCrossCloudEndpoint, parent, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	assertRealCrossCloudEntryAbsent(t, ctx, endpoint, parent, name)
}

func assertRealCrossCloudFile(t *testing.T, endpoint *realCrossCloudEndpoint, parent, name string, payload []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	path, item := requireRealCrossCloudEntry(t, ctx, endpoint, parent, name, false)
	if item.Size != int64(len(payload)) {
		t.Fatalf("copied real cross-cloud file size=%d, want=%d", item.Size, len(payload))
	}
	gotHash, gotSize, err := hashRealCrossCloudFile(ctx, endpoint.workspace, path)
	if err != nil {
		t.Fatalf("hash copied real cross-cloud file: %s", redactRealCloudFoxError(err))
	}
	wantHash := sha256.Sum256(payload)
	if gotSize != int64(len(payload)) || gotHash != wantHash {
		t.Fatalf("copied real cross-cloud file hash/size mismatch: size=%d want=%d hash=%x want=%x", gotSize, len(payload), gotHash, wantHash)
	}
}

func hashRealCrossCloudFile(ctx context.Context, filesystem vfs.VFS, path string) ([sha256.Size]byte, int64, error) {
	var zero [sha256.Size]byte
	f, err := filesystem.Open(ctx, path)
	if err != nil {
		return zero, 0, err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	var total int64
	for {
		n, readErr := f.Read(ctx, buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
			total += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return zero, total, readErr
		}
		if n == 0 {
			return zero, total, io.ErrNoProgress
		}
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	return sum, total, nil
}

type realCrossCloudTreeFixture struct {
	rootFile   []byte
	nestedFile []byte
	deepFile   []byte
}

func createRealCrossCloudTree(t *testing.T, endpoint *realCrossCloudEndpoint, name, seed string) realCrossCloudTreeFixture {
	t.Helper()
	fixture := realCrossCloudTreeFixture{
		rootFile:   realCrossCloudPayload(seed+"/root", 128*1024+3),
		nestedFile: realCrossCloudPayload(seed+"/nested", 64*1024+5),
		deepFile:   realCrossCloudPayload(seed+"/deep", 192*1024+7),
	}
	root := createRealCrossCloudDirectory(t, endpoint, endpoint.workspace.GetPath(), name)
	putRealCrossCloudFile(t, endpoint, root, "root.bin", fixture.rootFile)
	levelOne := createRealCrossCloudDirectory(t, endpoint, root, "level-one")
	putRealCrossCloudFile(t, endpoint, levelOne, "nested.bin", fixture.nestedFile)
	levelTwo := createRealCrossCloudDirectory(t, endpoint, levelOne, "level-two")
	putRealCrossCloudFile(t, endpoint, levelTwo, "deep.bin", fixture.deepFile)
	createRealCrossCloudDirectory(t, endpoint, root, "empty-directory")
	return fixture
}

func verifyRealCrossCloudTree(t *testing.T, endpoint *realCrossCloudEndpoint, name string, fixture realCrossCloudTreeFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root, _ := requireRealCrossCloudEntry(t, ctx, endpoint, endpoint.workspace.GetPath(), name, true)
	assertRealCrossCloudFile(t, endpoint, root, "root.bin", fixture.rootFile)
	levelOne, _ := requireRealCrossCloudEntry(t, ctx, endpoint, root, "level-one", true)
	assertRealCrossCloudFile(t, endpoint, levelOne, "nested.bin", fixture.nestedFile)
	levelTwo, _ := requireRealCrossCloudEntry(t, ctx, endpoint, levelOne, "level-two", true)
	assertRealCrossCloudFile(t, endpoint, levelTwo, "deep.bin", fixture.deepFile)
	empty, _ := requireRealCrossCloudEntry(t, ctx, endpoint, root, "empty-directory", true)
	items, err := readRealCrossCloudDir(ctx, endpoint.workspace, empty)
	if err != nil {
		t.Fatalf("list copied real cross-cloud empty directory: %v", err)
	}
	for _, item := range items {
		if item.Name != ".." {
			t.Fatalf("copied empty directory unexpectedly contains %q", item.Name)
		}
	}
}

func diagnoseRealCrossCloudRecursiveDestination(t *testing.T, endpoint *realCrossCloudEndpoint, name string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Probe the visual destination retained by recursiveCopy. Provider IDs are
	// deliberately unavailable above the backend boundary.
	guessedRoot := endpoint.workspace.Join(endpoint.workspace.GetPath(), name)
	_, guessedRootStatErr := endpoint.workspace.Stat(ctx, guessedRoot)
	guessedChild := endpoint.workspace.Join(guessedRoot, "root.bin")
	_, guessedChildStatErr := endpoint.workspace.Stat(ctx, guessedChild)

	items, listErr := readRealCrossCloudDir(ctx, endpoint.workspace, endpoint.workspace.GetPath())
	rowFound := false
	for _, item := range items {
		if item.Name == name && item.IsDir {
			rowFound = true
			break
		}
	}
	canonicalRoot := endpoint.workspace.Join(endpoint.workspace.GetPath(), name)
	canonicalChild := endpoint.workspace.Join(canonicalRoot, "root.bin")
	_, canonicalChildStatErr := endpoint.workspace.Stat(ctx, canonicalChild)

	return fmt.Sprintf(
		"read-only visual-path probe: guessed-root-visual=%t guessed-root-stat=%s guessed-child-visual=%t guessed-child-stat=%s list=%s row-found=%t canonical-root-visual=%t canonical-child-stat=%s",
		!strings.Contains(guessedRoot, "cloud://"),
		realCrossCloudErrorClass(guessedRootStatErr),
		!strings.Contains(guessedChild, "cloud://"),
		realCrossCloudErrorClass(guessedChildStatErr),
		realCrossCloudErrorClass(listErr),
		rowFound,
		!strings.Contains(canonicalRoot, "cloud://"),
		realCrossCloudErrorClass(canonicalChildStatErr),
	)
}

func realCrossCloudErrorClass(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, os.ErrNotExist):
		return "not-exist"
	case errors.Is(err, os.ErrInvalid):
		return "invalid"
	case errors.Is(err, os.ErrPermission):
		return "permission"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	default:
		return "other"
	}
}

func realCrossCloudPayload(seed string, size int) []byte {
	payload := make([]byte, size)
	offset := 0
	for counter := uint64(0); offset < len(payload); counter++ {
		block := sha256.Sum256([]byte(fmt.Sprintf("%s/%016x", seed, counter)))
		offset += copy(payload[offset:], block[:])
	}
	return payload
}

type realCrossCloudConflict struct {
	button string
	rename string
}

func runRealCrossCloudF5(t *testing.T, source, destination *realCrossCloudEndpoint, name string, conflict *realCrossCloudConflict) error {
	t.Helper()
	realCloudFoxUIResetScreen()
	sourceVFS := source.workspace.Clone()
	destinationVFS := destination.workspace.Clone()
	if sourceVFS == nil || sourceVFS == source.workspace || destinationVFS == nil || destinationVFS == destination.workspace {
		return errors.New("CloudVFS did not clone for production cross-cloud F5 panels")
	}
	defer func() { _ = sourceVFS.Close() }()      // Connection cleanup errors do not affect the operation result.
	defer func() { _ = destinationVFS.Close() }() // Connection cleanup errors do not affect the operation result.

	pf := realCloudFoxUIPanels(t, sourceVFS, destinationVFS)
	defer pf.Close()
	left := pf.panels[0].(*FileSystemPanel)
	right := pf.panels[1].(*FileSystemPanel)
	realCloudFoxUIWaitPanelName(t, left, name, 3*time.Minute)
	if conflict != nil {
		realCloudFoxUIWaitPanelName(t, right, name, 3*time.Minute)
	} else {
		realCloudFoxUIWait(t, 3*time.Minute, "cross-cloud destination panel to load", func() bool { return !right.isLoading })
	}
	pf.activeIdx = 0
	left.SetFocus(true)
	right.SetFocus(false)

	// This is the production F5 handler. With ConfirmCopy disabled it captures
	// the panel paths and dispatches ExecuteFileOpAt exactly as an accepted F5
	// dialog does, including the real progress and conflict UI routes.
	actionCopyMove(pf, false)
	return awaitRealCrossCloudFileOp(conflict, 10*time.Minute)
}

func awaitRealCrossCloudFileOp(conflict *realCrossCloudConflict, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	started := false
	conflictHandled := conflict == nil
	warningSeen := false
	renamePending := false
	var operationErr error

	for {
		activeProgress := false
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				if frame == nil || frame.IsDone() {
					continue
				}
				if _, ok := frame.(*FileOpProgressDialog); ok {
					started = true
					activeProgress = true
					continue
				}
				title := strings.ToLower(strings.TrimSpace(frame.GetTitle()))
				if strings.Contains(title, "error") || strings.Contains(title, "ошиб") {
					summary := realCrossCloudDialogSummary(frame)
					if summary == "" {
						summary = "no readable dialog text"
					}
					operationErr = fmt.Errorf("production file operation surfaced an error dialog: %s", summary)
					closeRealCrossCloudDialog(frame)
					continue
				}
				container, ok := frame.(vtui.Container)
				if !ok {
					continue
				}
				switch frame.GetTitle() {
				case " Warning ":
					if conflict == nil {
						operationErr = errors.New("production file operation presented an unexpected conflict")
						_ = clickRealCrossCloudButton(container, "Cancel")
						continue
					}
					if warningSeen {
						if conflictHandled {
							operationErr = errors.New("production file operation presented an unexpected additional conflict")
							_ = clickRealCrossCloudButton(container, "Cancel")
						}
						continue
					}
					warningSeen = true
					if !clickRealCrossCloudButton(container, conflict.button) {
						operationErr = fmt.Errorf("production conflict dialog has no %s action", conflict.button)
						_ = clickRealCrossCloudButton(container, "Cancel")
						continue
					}
					if conflict.button == "Rename" {
						renamePending = true
					} else {
						conflictHandled = true
					}
				case " Rename ":
					if !renamePending {
						continue
					}
					if !enterRealCrossCloudRename(container, conflict.rename) {
						operationErr = errors.New("production rename conflict dialog could not be completed")
						closeRealCrossCloudDialog(frame)
						continue
					}
					renamePending = false
					conflictHandled = true
				}
			}
		}

		if operationErr != nil && !activeProgress {
			return operationErr
		}
		if started && !activeProgress {
			if !conflictHandled {
				return errors.New("expected production conflict dialog was not completed")
			}
			if conflict != nil && !warningSeen {
				return errors.New("expected production conflict dialog was not presented")
			}
			return nil
		}

		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-ticker.C:
		case <-deadline.C:
			cancelRealCrossCloudProgressDialogs()
			if quiesceRealCrossCloudFileOps(time.Minute) {
				return errors.New("production file operation timed out, was cancelled, and was not retried")
			}
			return errRealCrossCloudOperationTimeout
		}
	}
}

func realCrossCloudDialogSummary(frame vtui.Frame) string {
	container, ok := frame.(vtui.Container)
	if !ok {
		return ""
	}
	var values []string
	var visit func(vtui.Container)
	visit = func(current vtui.Container) {
		for _, child := range current.GetChildren() {
			if textProvider, ok := child.(interface{ GetText() string }); ok {
				if value := strings.TrimSpace(textProvider.GetText()); value != "" {
					values = append(values, value)
				}
			}
			if nested, ok := child.(vtui.Container); ok {
				visit(nested)
			}
		}
	}
	visit(container)
	words := strings.Fields(strings.Join(values, " "))
	for i, word := range words {
		if strings.Contains(word, "://") {
			words[i] = "<redacted-uri>"
		}
	}
	result := strings.Join(words, " ")
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}

func clickRealCrossCloudButton(dialog vtui.Container, text string) bool {
	for _, child := range dialog.GetChildren() {
		button, ok := child.(*vtui.Button)
		if !ok || getCleanText(button) != text || button.OnClick == nil {
			continue
		}
		button.OnClick()
		return true
	}
	return false
}

func enterRealCrossCloudRename(dialog vtui.Container, name string) bool {
	var edit *vtui.Edit
	var okButton *vtui.Button
	for _, child := range dialog.GetChildren() {
		switch item := child.(type) {
		case *vtui.Edit:
			edit = item
		case *vtui.Button:
			if strings.EqualFold(getCleanText(item), "Ok") {
				okButton = item
			}
		}
	}
	if edit == nil || okButton == nil || okButton.OnClick == nil || name == "" {
		return false
	}
	edit.SetText(name)
	okButton.OnClick()
	return true
}

func closeRealCrossCloudDialog(frame vtui.Frame) {
	if container, ok := frame.(vtui.Container); ok {
		if clickRealCrossCloudButton(container, "Ok") || clickRealCrossCloudButton(container, "Cancel") {
			return
		}
	}
	frame.SetExitCode(-1)
}

func cancelRealCrossCloudProgressDialogs() {
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			dialog, ok := frame.(*FileOpProgressDialog)
			if !ok || dialog.IsDone() || dialog.btnCancel.OnClick == nil {
				continue
			}
			dialog.btnCancel.OnClick()
		}
	}
}

func quiesceRealCrossCloudFileOps(timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		active := false
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				if frame == nil || frame.IsDone() {
					continue
				}
				if _, ok := frame.(*FileOpProgressDialog); ok {
					active = true
					continue
				}
				title := strings.ToLower(strings.TrimSpace(frame.GetTitle()))
				if strings.Contains(title, "error") || strings.Contains(title, "ошиб") || frame.GetTitle() == " Warning " || frame.GetTitle() == " Rename " {
					closeRealCrossCloudDialog(frame)
				}
			}
		}
		if !active {
			return true
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-ticker.C:
		case <-deadline.C:
			return false
		}
	}
}
