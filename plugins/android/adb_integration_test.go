package androidfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/plugins/netfox/fishplus"
	"github.com/unxed/f4/vfs"
)

type timedHandshakeWrite struct {
	bytes int
	start time.Time
	end   time.Time
}

type timedHandshakeWriter struct {
	w      io.Writer
	writes []timedHandshakeWrite
}

func (w *timedHandshakeWriter) Write(p []byte) (int, error) {
	entry := timedHandshakeWrite{bytes: len(p), start: time.Now()}
	n, err := w.w.Write(p)
	entry.end = time.Now()
	w.writes = append(w.writes, entry)
	return n, err
}

func TestADBFishStartupTiming(t *testing.T) {
	serial := strings.TrimSpace(os.Getenv("F4_ADB_TEST_SERIAL"))
	if serial == "" || os.Getenv("F4_ADB_BENCH") == "" {
		t.Skip("set F4_ADB_TEST_SERIAL and F4_ADB_BENCH=1 to profile FISH+ startup")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	server := NewServer()

	discoveryStart := time.Now()
	devices, err := server.Devices(ctx)
	if err != nil {
		t.Fatalf("discover devices: %v", err)
	}
	t.Logf("device discovery: %s (%d devices)", time.Since(discoveryStart), len(devices))
	var selected DeviceInfo
	for _, device := range devices {
		if device.Serial == serial {
			selected = DeviceInfo{
				Serial: device.Serial, State: device.State, Product: device.Product,
				Model: device.Model, Device: device.Device, TransportID: device.TransportID,
			}
			break
		}
	}
	if selected.Serial == "" {
		t.Fatalf("serial %q is not present in host:devices-l", serial)
	}

	helperStart := time.Now()
	helperBytes := len(fishplus.HelperScript("0123456789abcdef"))
	t.Logf("host helper preparation: %s (%d bytes)", time.Since(helperStart), helperBytes)

	const runs = 7
	for run := 1; run <= runs; run++ {
		featuresStart := time.Now()
		adbFeatures, featureErr := server.Features(ctx, serial)
		featuresEnd := time.Now()
		if featureErr != nil {
			t.Fatalf("run %d query features: %v", run, featureErr)
		}
		if !adbFeatures["shell_v2"] {
			t.Fatalf("run %d: device does not advertise shell_v2", run)
		}

		shellStart := time.Now()
		shell, shellErr := server.OpenShellV2(ctx, serial, "exec /system/bin/sh")
		shellEnd := time.Now()
		if shellErr != nil {
			t.Fatalf("run %d open shell-v2: %v", run, shellErr)
		}
		stopInterrupt := shell.InterruptOnCancel(ctx)
		writer := &timedHandshakeWriter{w: shell}
		session := fishplus.NewSession(writer, shell, shell)

		handshakeStart := time.Now()
		handshakeErr := session.HandshakeWithOptions(ctx, fishplus.HandshakeOptions{
			Bootstrap: fishplus.BootstrapBase64Line,
		})
		handshakeEnd := time.Now()
		if handshakeErr != nil {
			stopInterrupt()
			t.Fatalf("run %d handshake: %v (stderr=%q)", run, handshakeErr, shell.Stderr())
		}
		if len(writer.writes) != 1 {
			stopInterrupt()
			_ = session.Close()
			t.Fatalf("run %d base64 handshake made %d writes, want 1", run, len(writer.writes))
		}

		client := fishplus.NewClient(session)
		pwdStart := time.Now()
		cwd, pwdErr := client.Pwd(ctx)
		pwdEnd := time.Now()
		if pwdErr != nil {
			stopInterrupt()
			_ = session.Close()
			t.Fatalf("run %d pwd: %v", run, pwdErr)
		}

		statStart := time.Now()
		_, statErr := client.Stat(ctx, "/")
		statEnd := time.Now()
		if statErr != nil {
			stopInterrupt()
			_ = session.Close()
			t.Fatalf("run %d stat root: %v", run, statErr)
		}

		enumStart := time.Now()
		entries, enumErr := client.Enum(ctx, "/")
		enumEnd := time.Now()
		if enumErr != nil {
			stopInterrupt()
			_ = session.Close()
			t.Fatalf("run %d enum root: %v", run, enumErr)
		}

		bootstrapWrite := writer.writes[0]
		t.Logf("run %d: features=%s shell=%s base64-handshake=%s [one-line-send=%s helper-init=%s] pwd=%s stat(/)=%s enum(/)=%s entries=%d cwd=%q modes=%q",
			run,
			featuresEnd.Sub(featuresStart),
			shellEnd.Sub(shellStart),
			handshakeEnd.Sub(handshakeStart),
			bootstrapWrite.end.Sub(bootstrapWrite.start),
			handshakeEnd.Sub(bootstrapWrite.end),
			pwdEnd.Sub(pwdStart),
			statEnd.Sub(statStart),
			enumEnd.Sub(enumStart),
			len(entries), cwd, session.Features().Raw,
		)

		stopInterrupt()
		if closeErr := session.Close(); closeErr != nil {
			t.Fatalf("run %d close session: %v", run, closeErr)
		}
	}

	pool := newFishSessionPool()
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("close FISH+ session pool: %v", err)
		}
	})
	opener := &hybridDeviceOpener{
		features: server.Features,
		pool:     pool,
		openFish: func(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
			return openFishDevice(ctx, parent, server, device)
		},
	}

	openStart := time.Now()
	mounted, openErr := opener.OpenDevice(ctx, nil, selected)
	openEnd := time.Now()
	if openErr != nil {
		t.Fatalf("open full FishVFS: %v", openErr)
	}

	statStart := time.Now()
	_, statErr := mounted.Stat(ctx, "/")
	statEnd := time.Now()
	if statErr != nil {
		t.Fatalf("panel Stat(/): %v", statErr)
	}
	readDirStart := time.Now()
	itemCount := 0
	readDirErr := mounted.ReadDir(ctx, "/", func(items []vfs.VFSItem) { itemCount += len(items) })
	readDirEnd := time.Now()
	if readDirErr != nil {
		t.Fatalf("full FishVFS ReadDir(/): %v", readDirErr)
	}
	t.Logf("first full panel path: open=%s stat=%s ReadDir=%s items=%d total=%s",
		openEnd.Sub(openStart),
		statEnd.Sub(statStart),
		readDirEnd.Sub(readDirStart),
		itemCount,
		readDirEnd.Sub(openStart),
	)
	if closeErr := mounted.Close(); closeErr != nil {
		t.Fatalf("close first pooled view: %v", closeErr)
	}

	repeatStart := time.Now()
	repeated, repeatErr := opener.OpenDevice(ctx, nil, selected)
	repeatOpenEnd := time.Now()
	if repeatErr != nil {
		t.Fatalf("repeat pooled open: %v", repeatErr)
	}
	repeatStatStart := time.Now()
	_, repeatStatErr := repeated.Stat(ctx, "/")
	repeatStatEnd := time.Now()
	if repeatStatErr != nil {
		_ = repeated.Close()
		t.Fatalf("repeat panel Stat(/): %v", repeatStatErr)
	}
	repeatItems := 0
	repeatReadStart := time.Now()
	repeatReadErr := repeated.ReadDir(ctx, "/", func(items []vfs.VFSItem) { repeatItems += len(items) })
	repeatReadEnd := time.Now()
	if repeatReadErr != nil {
		_ = repeated.Close()
		t.Fatalf("repeat FishVFS ReadDir(/): %v", repeatReadErr)
	}
	t.Logf("repeated pooled panel path: open=%s stat=%s ReadDir=%s items=%d total=%s",
		repeatOpenEnd.Sub(repeatStart),
		repeatStatEnd.Sub(repeatStatStart),
		repeatReadEnd.Sub(repeatReadStart),
		repeatItems,
		repeatReadEnd.Sub(repeatStart),
	)
	if closeErr := repeated.Close(); closeErr != nil {
		t.Fatalf("close repeated pooled view: %v", closeErr)
	}
}

func TestADBDevicePanelInfoIntegration(t *testing.T) {
	serial := strings.TrimSpace(os.Getenv("F4_ADB_TEST_SERIAL"))
	if serial == "" {
		t.Skip("set F4_ADB_TEST_SERIAL to run the real-device information test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := NewServer()
	devices, err := server.Devices(ctx)
	if err != nil {
		t.Fatalf("discover devices: %v", err)
	}
	var selected DeviceInfo
	for _, device := range devices {
		if device.Serial == serial {
			selected = DeviceInfo{
				Serial: device.Serial, State: device.State, Product: device.Product,
				Model: device.Model, Device: device.Device, TransportID: device.TransportID,
			}
			break
		}
	}
	if selected.Serial == "" {
		t.Fatalf("serial %q is not present in host:devices-l", serial)
	}

	provider := newDeviceInfoService(server).provider(selected, "ADB", "host transport")
	req := vfs.PanelInfoRequest{Path: "/"}
	baseline, fresh := provider.CachedPanelInfo(req)
	if fresh || !baseline.Authoritative {
		t.Fatalf("initial snapshot = fresh %v authoritative %v", fresh, baseline.Authoritative)
	}
	if field, ok := panelField(baseline, "serial"); !ok || field.Value != serial {
		t.Fatalf("baseline serial = %#v, present %v", field, ok)
	}

	started := time.Now()
	snapshot, err := provider.RefreshPanelInfo(ctx, req)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("refresh device information after %s: %v", elapsed, err)
	}
	for _, id := range []string{"model", "android", "battery", "memory", "uptime", "storage"} {
		if field, ok := panelField(snapshot, id); !ok || (field.Kind == vfs.PanelInfoText && strings.TrimSpace(field.Value) == "") {
			t.Errorf("refreshed field %q = %#v, present %v", id, field, ok)
		}
	}
	for _, id := range []string{"memory", "storage"} {
		field, ok := panelField(snapshot, id)
		if !ok || field.Kind != vfs.PanelInfoUsage || field.TotalBytes == 0 || field.AvailableBytes > field.TotalBytes {
			t.Errorf("refreshed usage field %q = %#v, present %v", id, field, ok)
		}
	}
	if _, fresh := provider.CachedPanelInfo(req); !fresh {
		t.Fatal("successful real-device refresh was not cached")
	}
	t.Logf("device information refresh: %s (%s, serial %s)", elapsed, selected.Model, selected.Serial)
}

func TestADBDeviceIntegration(t *testing.T) {
	serial := strings.TrimSpace(os.Getenv("F4_ADB_TEST_SERIAL"))
	if serial == "" {
		t.Skip("set F4_ADB_TEST_SERIAL to run the real-device test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	server := NewServer()
	devices, err := server.Devices(ctx)
	if err != nil {
		t.Fatalf("discover devices: %v", err)
	}
	var selected DeviceInfo
	for _, device := range devices {
		if device.Serial == serial {
			selected = DeviceInfo{
				Serial: device.Serial, State: device.State, Product: device.Product,
				Model: device.Model, Device: device.Device, TransportID: device.TransportID,
			}
			break
		}
	}
	if selected.Serial == "" {
		t.Fatalf("serial %q is not present in host:devices-l", serial)
	}
	if selected.State != DeviceStateOnline {
		t.Fatalf("device %q state is %q, want %q", serial, selected.State, DeviceStateOnline)
	}

	features, err := server.Features(ctx, serial)
	if err != nil {
		t.Fatalf("query features: %v", err)
	}
	if os.Getenv("F4_ADB_TEST_FISH") != "" {
		fish, fishErr := openFishDevice(ctx, nil, server, selected)
		if fishErr != nil {
			t.Fatalf("open FISH+ without Sync fallback: %v", fishErr)
		}
		t.Logf("direct FISH+ handshake succeeded for %s", serial)
		if closeErr := fish.Close(); closeErr != nil {
			t.Fatalf("close direct FISH+ probe: %v", closeErr)
		}
	}
	pool := newFishSessionPool()
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("close FISH+ session pool: %v", err)
		}
	})
	opener := &hybridDeviceOpener{
		features: func(context.Context, string) (map[string]bool, error) { return features, nil },
		pool:     pool,
		openFish: func(ctx context.Context, parent vfs.VFS, device DeviceInfo) (vfs.VFS, error) {
			return openFishDevice(ctx, parent, server, device)
		},
		openSync: func(ctx context.Context, parent vfs.VFS, device DeviceInfo, features map[string]bool) (vfs.VFS, error) {
			return openSyncDevice(ctx, parent, server, device, features)
		},
	}
	manager := NewManagerVFS(nil, opener)
	mounted, err := opener.OpenDevice(ctx, manager, selected)
	if err != nil {
		t.Fatalf("open Android filesystem: %v", err)
	}
	t.Cleanup(func() {
		if err := mounted.Close(); err != nil {
			t.Errorf("close Android filesystem: %v", err)
		}
	})
	var fishMounted *netfox.FishVFS
	if os.Getenv("F4_ADB_TEST_FISH") != "" {
		var ok bool
		fishMounted, ok = mounted.(*netfox.FishVFS)
		if !ok {
			t.Fatalf("hybrid opener selected %T instead of FISH+", mounted)
		}
		t.Logf("hybrid selected FISH+ with features: %s", fishMounted.Client().Session().Features().Raw)
		repeated, repeatErr := opener.OpenDevice(ctx, manager, selected)
		if repeatErr != nil {
			t.Fatalf("open second pooled FISH+ view: %v", repeatErr)
		}
		if repeated.(vfs.SessionIdentity).SessionKey() != fishMounted.SessionKey() {
			_ = repeated.Close()
			t.Fatal("second Android view did not reuse the first FISH+ session")
		}
		if closeErr := repeated.Close(); closeErr != nil {
			t.Fatalf("close second pooled FISH+ view: %v", closeErr)
		}
	}

	sandbox := fmt.Sprintf("/data/local/tmp/f4-adb-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	if _, err := mutationPath(sandbox); err != nil {
		t.Fatalf("invalid test sandbox: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, stderr, code, cleanupErr := server.RunShell(cleanupCtx, serial, "rm -rf -- "+quoteShellArg(sandbox))
		if cleanupErr != nil || code != 0 {
			t.Logf("sandbox cleanup failed: exit=%d err=%v stderr=%s", code, cleanupErr, stderr)
		}
	}()

	if err := mounted.MkDir(ctx, sandbox); err != nil {
		t.Fatalf("MkDir sandbox: %v", err)
	}
	if err := mounted.SetPath(sandbox); err != nil {
		t.Fatalf("SetPath sandbox: %v", err)
	}
	runner, ok := mounted.(vfs.CommandRunner)
	if !ok {
		t.Fatalf("mounted Android backend %T does not implement CommandRunner", mounted)
	}
	var commandLines []string
	exitCode, commandErr := runner.RunCommand(ctx, sandbox, "pwd; printf 'f4-command-ok\\n'", func(line string) {
		commandLines = append(commandLines, line)
	})
	if commandErr != nil || exitCode != 0 {
		t.Fatalf("RunCommand: exit=%d err=%v output=%#v", exitCode, commandErr, commandLines)
	}
	if len(commandLines) < 2 || commandLines[0] != sandbox || commandLines[1] != "f4-command-ok" {
		t.Fatalf("RunCommand output = %#v, want pwd %q and marker", commandLines, sandbox)
	}

	source := path.Join(sandbox, "source file.txt")
	renamed := path.Join(sandbox, "renamed file.txt")
	payload := []byte("f4 Android integration\nsecond line\n")
	w, err := mounted.Create(ctx, source)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		_ = w.Close()
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("writer Close: %v", err)
	}
	expectedPayload := bytes.Clone(payload)
	if fishMounted != nil {
		const patchOffset = 3
		patchBytes := []byte("FISH+")
		if err := fishMounted.Client().Write(ctx, source, patchOffset, patchBytes); err != nil {
			t.Fatalf("positioned FISH+ write: %v", err)
		}
		copy(expectedPayload[patchOffset:], patchBytes)
	}
	if err := mounted.SetAttributes(ctx, source, vfs.VFSItem{UnixMode: 0600, Uid: -1, Gid: -1}); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	item, err := mounted.Stat(ctx, source)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if item.Size != int64(len(expectedPayload)) || item.UnixMode&0777 != 0600 {
		t.Fatalf("Stat metadata = size %d mode %#o", item.Size, item.UnixMode)
	}

	r, err := mounted.Open(ctx, source)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	read, err := readContextFile(ctx, r)
	if closeErr := r.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(read, expectedPayload) {
		t.Fatalf("read payload = %q, want %q", read, expectedPayload)
	}
	if fishMounted != nil {
		link := path.Join(sandbox, "source link.txt")
		_, stderr, code, linkErr := server.RunShell(ctx, serial, "ln -s "+quoteShellArg(source)+" "+quoteShellArg(link))
		if linkErr != nil || code != 0 {
			t.Fatalf("create test symlink: exit=%d err=%v stderr=%s", code, linkErr, stderr)
		}
		linkReader, openErr := mounted.Open(ctx, link)
		if openErr != nil {
			t.Fatalf("open symlink target: %v", openErr)
		}
		linkRead, readErr := readContextFile(ctx, linkReader)
		if closeErr := linkReader.Close(); readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			t.Fatalf("read symlink target: %v", readErr)
		}
		if !bytes.Equal(linkRead, expectedPayload) {
			t.Fatalf("symlink payload = %q, want %q", linkRead, expectedPayload)
		}
		if err := mounted.Remove(ctx, link); err != nil {
			t.Fatalf("Remove symlink: %v", err)
		}
	}

	if err := mounted.Rename(ctx, source, renamed); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	var names []string
	if err := mounted.ReadDir(ctx, sandbox, func(chunk []vfs.VFSItem) {
		for _, entry := range chunk {
			names = append(names, entry.Name)
		}
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(names) != 1 || names[0] != path.Base(renamed) {
		t.Fatalf("sandbox entries = %#v", names)
	}
	if err := mounted.Remove(ctx, renamed); err != nil {
		t.Fatalf("Remove file: %v", err)
	}
	if err := mounted.Remove(ctx, sandbox); err != nil {
		t.Fatalf("Remove sandbox: %v", err)
	}
}

func readContextFile(ctx context.Context, reader vfs.ReadAtCloser) ([]byte, error) {
	var result bytes.Buffer
	buffer := make([]byte, 32*1024)
	for {
		n, err := reader.Read(ctx, buffer)
		if n != 0 {
			_, _ = result.Write(buffer[:n])
		}
		if err == io.EOF {
			return result.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}
