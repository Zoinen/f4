package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	androidfs "github.com/unxed/f4/plugins/android"
	iosfs "github.com/unxed/f4/plugins/ios"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const realIOSAndroidCopyEnv = "F4_REAL_IOS_ANDROID_COPY"

type realDeviceCopyHost struct {
	drives    map[string]func() vfs.VFS
	providers []vfs.VFSProvider
}

func newRealDeviceCopyHost() *realDeviceCopyHost {
	return &realDeviceCopyHost{drives: make(map[string]func() vfs.VFS)}
}

func (*realDeviceCopyHost) GetVersion() string                           { return "real-device-copy-test" }
func (*realDeviceCopyHost) Log(string)                                   {}
func (*realDeviceCopyHost) Message(string)                               {}
func (*realDeviceCopyHost) RegisterHighlighter(vtui.HighlighterProvider) {}
func (h *realDeviceCopyHost) RegisterVFSProvider(provider vfs.VFSProvider) {
	h.providers = append(h.providers, provider)
}
func (*realDeviceCopyHost) RegisterURIProvider(vfs.URIProvider) error { return nil }
func (h *realDeviceCopyHost) RegisterDrive(name string, factory func() vfs.VFS) {
	h.drives[name] = factory
}
func (*realDeviceCopyHost) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {
}
func (*realDeviceCopyHost) RegisterPluginMenuItem(string, func(vfs.App)) {}
func (*realDeviceCopyHost) RunAction(string) bool                        { return false }

// TestRealIOSDCIMToAndroidDCIMCopy is intentionally opt-in: it writes every
// source entry to a real Android device. It mounts both devices through the
// same production plugins as the panels, runs the core recursive-copy engine,
// then reads both sides again and compares every file by size and SHA-256.
func TestRealIOSDCIMToAndroidDCIMCopy(t *testing.T) {
	if os.Getenv(realIOSAndroidCopyEnv) == "" {
		t.Skip("set " + realIOSAndroidCopyEnv + "=1 to copy between connected real devices")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	iosPlugin, androidPlugin := iosfs.NewPlugin(), androidfs.NewPlugin()
	iosHost, androidHost := newRealDeviceCopyHost(), newRealDeviceCopyHost()
	if err := iosPlugin.Init(iosHost); err != nil {
		t.Fatalf("initialize iOS plugin: %v", err)
	}
	defer func() {
		if err := iosPlugin.Close(); err != nil {
			t.Errorf("close iOS plugin: %v", err)
		}
	}()
	if err := androidPlugin.Init(androidHost); err != nil {
		t.Fatalf("initialize Android plugin: %v", err)
	}
	defer func() {
		if err := androidPlugin.Close(); err != nil {
			t.Errorf("close Android plugin: %v", err)
		}
	}()

	source := openRealDeviceForCopy(t, ctx, iosHost, "iOS", os.Getenv("F4_REAL_IOS_DEVICE"))
	defer source.Close()
	destination := openRealDeviceForCopy(t, ctx, androidHost, "Android", os.Getenv("F4_REAL_ANDROID_DEVICE"))
	defer destination.Close()

	const sourceDir = "/DCIM/100APPLE"
	const destinationDir = "/sdcard/DCIM"
	if err := source.SetPath(sourceDir); err != nil {
		t.Fatalf("open iPhone source %s: %v", sourceDir, err)
	}
	if err := destination.SetPath(destinationDir); err != nil {
		t.Fatalf("open Android destination %s: %v", destinationDir, err)
	}

	sourceItems := readRealDeviceCopyDir(t, ctx, source, sourceDir)
	names := make([]string, 0, len(sourceItems))
	for _, item := range sourceItems {
		if item.Name != ".." {
			names = append(names, item.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("iPhone source directory is empty")
	}
	stats, err := vfs.CalculateStats(ctx, source, sourceDir, names, nil)
	if err != nil {
		t.Fatalf("scan iPhone source: %v", err)
	}

	destinationItems := readRealDeviceCopyDir(t, ctx, destination, destinationDir)
	existing := make(map[string]bool, len(destinationItems))
	for _, item := range destinationItems {
		existing[item.Name] = true
	}
	collisions := 0
	for _, name := range names {
		if existing[transferItemName(source, source.Join(sourceDir, name), destination, name)] {
			collisions++
		}
	}
	t.Logf("copying %d top-level item(s), %d file(s), %d directorie(s), %d bytes from %T to %T; %d destination collision(s)",
		len(names), stats.Files, stats.Dirs, stats.Bytes, source, destination, collisions)

	state := &FileOpState{OverwriteAll: true, Buffer: make([]byte, 128*1024)}
	copyStarted := time.Now()
	for index, name := range names {
		sourcePath := source.Join(sourceDir, name)
		targetName := transferItemName(source, sourcePath, destination, name)
		targetPath := destination.Join(destinationDir, targetName)
		if err := recursiveCopy(ctx, source, sourcePath, destination, targetPath, state, 0); err != nil {
			t.Fatalf("copy item %d/%d %q: %v", index+1, len(names), name, err)
		}
		if (index+1)%10 == 0 || index+1 == len(names) {
			t.Logf("copied %d/%d top-level item(s) in %s", index+1, len(names), time.Since(copyStarted).Round(time.Millisecond))
		}
	}
	t.Logf("copy completed in %s; starting full size and SHA-256 verification", time.Since(copyStarted).Round(time.Millisecond))

	verified := realDeviceCopyVerification{}
	verifyStarted := time.Now()
	for _, name := range names {
		sourcePath := source.Join(sourceDir, name)
		targetName := transferItemName(source, sourcePath, destination, name)
		targetPath := destination.Join(destinationDir, targetName)
		verifyRealDeviceCopyTree(t, ctx, source, sourcePath, destination, targetPath, name, &verified)
	}
	if verified.files != stats.Files || verified.dirs != stats.Dirs || verified.bytes != stats.Bytes {
		t.Fatalf("verified totals = %d files, %d dirs, %d bytes; source scan = %d files, %d dirs, %d bytes",
			verified.files, verified.dirs, verified.bytes, stats.Files, stats.Dirs, stats.Bytes)
	}
	t.Logf("verified %d file(s), %d directorie(s), and %d bytes by full SHA-256 in %s",
		verified.files, verified.dirs, verified.bytes, time.Since(verifyStarted).Round(time.Millisecond))
}

func openRealDeviceForCopy(t *testing.T, ctx context.Context, host *realDeviceCopyHost, driveName, selector string) vfs.VFS {
	t.Helper()
	factory := host.drives[driveName]
	if factory == nil {
		t.Fatalf("%s plugin did not register its drive", driveName)
	}
	manager := factory()
	items := readRealDeviceCopyDir(t, ctx, manager, manager.GetPath())
	selector = strings.ToLower(strings.TrimSpace(selector))
	var candidates []vfs.VFSItem
	for _, item := range items {
		if selector == "" || strings.Contains(strings.ToLower(item.Name), selector) {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) != 1 {
		var names []string
		for _, item := range items {
			names = append(names, item.Name)
		}
		t.Fatalf("%s device selector %q matched %d rows; available rows: %v", driveName, selector, len(candidates), names)
	}
	devicePath := manager.Join(manager.GetPath(), candidates[0].Name)
	for _, provider := range host.providers {
		if !provider.CanOpen(ctx, manager, devicePath) {
			continue
		}
		mounted, err := provider.Open(ctx, manager, devicePath)
		if err != nil {
			t.Fatalf("open %s device %q: %v", driveName, candidates[0].Name, err)
		}
		t.Logf("opened %s device %q through %T", driveName, candidates[0].Name, mounted)
		return mounted
	}
	t.Fatalf("no %s provider accepted device %q", driveName, candidates[0].Name)
	return nil
}

func readRealDeviceCopyDir(t *testing.T, ctx context.Context, filesystem vfs.VFS, dir string) []vfs.VFSItem {
	t.Helper()
	var items []vfs.VFSItem
	if err := filesystem.ReadDir(ctx, dir, func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("list %T %s: %v", filesystem, dir, err)
	}
	return items
}

type realDeviceCopyVerification struct {
	files int64
	dirs  int64
	bytes int64
}

func verifyRealDeviceCopyTree(t *testing.T, ctx context.Context, source vfs.VFS, sourcePath string, destination vfs.VFS, destinationPath, displayPath string, totals *realDeviceCopyVerification) {
	t.Helper()
	sourceItem, err := source.Stat(ctx, sourcePath)
	if err != nil {
		t.Fatalf("stat source %q: %v", displayPath, err)
	}
	destinationItem, err := destination.Stat(ctx, destinationPath)
	if err != nil {
		t.Fatalf("stat destination %q: %v", displayPath, err)
	}
	if sourceItem.IsDir != destinationItem.IsDir {
		t.Fatalf("type mismatch for %q: source dir=%v destination dir=%v", displayPath, sourceItem.IsDir, destinationItem.IsDir)
	}
	if sourceItem.IsDir {
		totals.dirs++
		for _, child := range readRealDeviceCopyDir(t, ctx, source, sourcePath) {
			if child.Name == ".." {
				continue
			}
			childSource := source.Join(sourcePath, child.Name)
			childTargetName := transferItemName(source, childSource, destination, child.Name)
			verifyRealDeviceCopyTree(t, ctx, source, childSource, destination, destination.Join(destinationPath, childTargetName), displayPath+"/"+child.Name, totals)
		}
		return
	}
	if sourceItem.Size != destinationItem.Size {
		t.Fatalf("size mismatch for %q: source=%d destination=%d", displayPath, sourceItem.Size, destinationItem.Size)
	}
	sourceHash := hashRealDeviceCopyFile(t, ctx, source, sourcePath)
	destinationHash := hashRealDeviceCopyFile(t, ctx, destination, destinationPath)
	if sourceHash != destinationHash {
		t.Fatalf("SHA-256 mismatch for %q: source=%s destination=%s", displayPath, sourceHash, destinationHash)
	}
	totals.files++
	totals.bytes += sourceItem.Size
}

func hashRealDeviceCopyFile(t *testing.T, ctx context.Context, filesystem vfs.VFS, filePath string) string {
	t.Helper()
	reader, err := filesystem.Open(ctx, filePath)
	if err != nil {
		t.Fatalf("open %T %s for verification: %v", filesystem, filePath, err)
	}
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		n, readErr := reader.Read(ctx, buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = reader.Close()
			t.Fatalf("read %T %s for verification: %v", filesystem, filePath, readErr)
		}
		if n == 0 {
			_ = reader.Close()
			t.Fatalf("read %T %s made no progress", filesystem, filePath)
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close %T %s after verification: %v", filesystem, filePath, err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

var _ vfs.HostAPI = (*realDeviceCopyHost)(nil)
