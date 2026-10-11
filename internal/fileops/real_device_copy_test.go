//go:build integration

package fileops_test

import (
	context "context"
	sha256 "crypto/sha256"
	hex "encoding/hex"
	errors "errors"
	fileops "github.com/unxed/f4/internal/fileops"
	plughost "github.com/unxed/f4/internal/plughost"
	f4rpc "github.com/unxed/f4/sdk/f4rpc"
	vfs "github.com/unxed/f4/vfs"
	io "io"
	os "os"
	exec "os/exec"
	sort "sort"
	strings "strings"
	testing "testing"
	time "time"
)

const realIOSAndroidCopyEnv = "F4_REAL_IOS_ANDROID_COPY"

// Supply separately built standalone plugins in F4_REAL_IOS_PLUGIN and
// F4_REAL_ANDROID_PLUGIN. Run with -tags=integration -timeout=50m and enable
// F4_REAL_IOS_ANDROID_COPY only when overwriting the destination is intended.
// Plugin dependencies stay outside the root module's build list.
type realDeviceCopyTransport struct {
	ctx     context.Context
	session *f4rpc.Session
}

func (transport realDeviceCopyTransport) Call(method string, params, result any) error {
	return transport.session.CallContext(transport.ctx, method, params, result)
}

func TestRealIOSDCIMToAndroidDCIMCopy(t *testing.T) {
	if os.Getenv(realIOSAndroidCopyEnv) == "" {
		t.Skip("set " + realIOSAndroidCopyEnv + "=1 to copy between connected real devices")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	iosManager := startRealDeviceCopyPlugin(t, ctx, "F4_REAL_IOS_PLUGIN", "iOS")
	androidManager := startRealDeviceCopyPlugin(t, ctx, "F4_REAL_ANDROID_PLUGIN", "Android")
	source := openRealDeviceForCopy(t, ctx, iosManager, "iOS", os.Getenv("F4_REAL_IOS_DEVICE"))
	defer source.Close()
	destination := openRealDeviceForCopy(t, ctx, androidManager, "Android", os.Getenv("F4_REAL_ANDROID_DEVICE"))
	defer destination.Close()

	// RPC paths include the device row; unlike the old provider mount they do
	// not discard that prefix when entering the device's native filesystem.
	sourceDir := source.Join(source.GetPath(), "DCIM", "100APPLE")
	destinationDir := destination.Join(destination.GetPath(), "sdcard", "DCIM")
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
		if existing[fileops.TransferItemName(source, source.Join(sourceDir, name), destination, name)] {
			collisions++
		}
	}
	t.Logf("copying %d top-level item(s), %d file(s), %d directorie(s), %d bytes from %T to %T; %d destination collision(s)",
		len(names), stats.Files, stats.Dirs, stats.Bytes, source, destination, collisions)

	state := &fileops.FileOpState{OverwriteAll: true, Buffer: make([]byte, 128*1024)}
	copyStarted := time.Now()
	for index, name := range names {
		sourcePath := source.Join(sourceDir, name)
		targetName := fileops.TransferItemName(source, sourcePath, destination, name)
		targetPath := destination.Join(destinationDir, targetName)
		if err := fileops.CopyForRealDeviceIntegration(ctx, source, sourcePath, destination, targetPath, state); err != nil {
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
		targetName := fileops.TransferItemName(source, sourcePath, destination, name)
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

func startRealDeviceCopyPlugin(t *testing.T, ctx context.Context, binaryEnv, driveName string) vfs.VFS {
	t.Helper()
	binary := os.Getenv(binaryEnv)
	if binary == "" {
		t.Fatalf("set %s to the separately built %s plugin executable", binaryEnv, driveName)
	}
	pluginCtx, stop := context.WithCancel(ctx)
	t.Cleanup(stop)
	command := exec.CommandContext(pluginCtx, binary)
	command.Stderr = os.Stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		t.Fatalf("start %s plugin: %v", driveName, err)
	}
	session := f4rpc.NewSession(stdout, stdin)
	served := make(chan error, 1)
	go func() { served <- session.Serve() }()
	t.Cleanup(func() {
		stop()
		_ = stdin.Close()
		_ = command.Wait() // Cancellation intentionally terminates this test's child.
		_ = stdout.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("%s RPC transport did not stop", driveName)
		}
	})
	transport := realDeviceCopyTransport{ctx: pluginCtx, session: session}
	var initialized plughost.PluginInitResponse
	if err := transport.Call("Plugin.Init", nil, &initialized); err != nil {
		t.Fatalf("initialize %s plugin: %v", driveName, err)
	}
	for _, drive := range initialized.Drives {
		if drive == driveName {
			t.Logf("[FIX] initialized standalone %s plugin %q", driveName, binary)
			return plughost.NewRPCVFS(transport, driveName)
		}
	}
	t.Fatalf("%s plugin did not register its drive: %v", driveName, initialized.Drives)
	return nil
}

func openRealDeviceForCopy(t *testing.T, ctx context.Context, manager vfs.VFS, driveName, selector string) vfs.VFS {
	t.Helper()
	items := readRealDeviceCopyDir(t, ctx, manager, manager.GetPath())
	selector = strings.ToLower(strings.TrimSpace(selector))
	var candidates []vfs.VFSItem
	for _, item := range items {
		if item.Name == ".." || !item.IsDir {
			continue
		}
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
	mounted := manager.Clone()
	if err := mounted.SetPath(devicePath); err != nil {
		t.Fatalf("open %s device %q: %v", driveName, candidates[0].Name, err)
	}
	// SetPath on RPCVFS is local; listing verifies the remote device can open.
	readRealDeviceCopyDir(t, ctx, mounted, devicePath)
	t.Logf("opened %s device %q through %T", driveName, candidates[0].Name, mounted)
	return mounted
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
			childTargetName := fileops.TransferItemName(source, childSource, destination, child.Name)
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
