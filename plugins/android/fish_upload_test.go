package androidfs

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/vfs"
)

// uploadSyncStub is fakeSyncFS with controllable Lstat and Send failures.
type uploadSyncStub struct {
	*fakeSyncFS
	lstatErr error
	sendErr  error
	sends    int
}

func (s *uploadSyncStub) Lstat(ctx context.Context, p string) (SyncEntry, error) {
	if s.lstatErr != nil {
		return SyncEntry{}, s.lstatErr
	}
	return s.fakeSyncFS.Lstat(ctx, p)
}

func (s *uploadSyncStub) Send(ctx context.Context, p string, mode uint32, mtime time.Time) (io.WriteCloser, error) {
	s.sends++
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return s.fakeSyncFS.Send(ctx, p, mode, mtime)
}

func newUploadStub(entries map[string]SyncEntry) *uploadSyncStub {
	return &uploadSyncStub{fakeSyncFS: &fakeSyncFS{entries: entries, files: map[string][]byte{}}}
}

func TestOpenSyncUploadSendsANewFileWithTheAdbPushMode(t *testing.T) {
	stub := newUploadStub(nil)
	w := openSyncUpload(context.Background(), stub, "/sdcard/new.bin")
	if w == nil {
		t.Fatal("a free path must be sent over Sync")
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if stub.sendPath != "/sdcard/new.bin" || stub.sendMode != 0644 {
		t.Fatalf("Send(%q, %#o), want /sdcard/new.bin with 0644", stub.sendPath, stub.sendMode)
	}
	if got := string(stub.files["/sdcard/new.bin"]); got != "payload" {
		t.Fatalf("file content = %q", got)
	}
}

func TestOpenSyncUploadKeepsThePermissionsOfARegularFile(t *testing.T) {
	stub := newUploadStub(map[string]SyncEntry{"/data/local/tmp/run.sh": {Mode: 0100755}})
	if w := openSyncUpload(context.Background(), stub, "/data/local/tmp/run.sh"); w == nil {
		t.Fatal("a regular file must be replaced over Sync")
	}
	if stub.sendMode != 0755 {
		t.Fatalf("mode = %#o, want 0755 (the file type bits are not part of a SEND mode)", stub.sendMode)
	}
}

func TestOpenSyncUploadLeavesWhatTheShellShouldDecideToFish(t *testing.T) {
	cases := map[string]struct {
		target   string
		entries  map[string]SyncEntry
		lstatErr error
		sendErr  error
	}{
		"directory":          {target: "/sdcard/d", entries: map[string]SyncEntry{"/sdcard/d": {Mode: remoteModeDir | 0755}}},
		"symlink":            {target: "/sdcard/l", entries: map[string]SyncEntry{"/sdcard/l": {Mode: remoteModeLink | 0777}}},
		"character device":   {target: "/dev/null", entries: map[string]SyncEntry{"/dev/null": {Mode: 0020666}}},
		"lstat fails":        {target: "/sdcard/f", lstatErr: errors.New("adb sync: connection reset")},
		"send cannot start":  {target: "/sdcard/f", sendErr: errors.New("adb sync: service unavailable")},
		"device root":        {target: "/"},
		"dot dot component":  {target: "/sdcard/../data/x"},
		"relative path":      {target: "sdcard/f"},
		"permission refused": {target: "/sdcard/f", lstatErr: os.ErrPermission},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stub := newUploadStub(tc.entries)
			stub.lstatErr, stub.sendErr = tc.lstatErr, tc.sendErr
			if w := openSyncUpload(context.Background(), stub, tc.target); w != nil {
				t.Fatalf("openSyncUpload(%q) took the Sync path, want the FISH+ writer", tc.target)
			}
		})
	}
}

func TestWithSyncUploadsWrapsOnlyAFishView(t *testing.T) {
	client := &fakeSyncFS{}
	null := vfs.NewNullVFS(0)
	if got := withSyncUploads(null, client); got != vfs.VFS(null) {
		t.Fatalf("a non-FISH+ view was wrapped: %T", got)
	}
	fish := &netfox.FishVFS{}
	if got := withSyncUploads(fish, nil); got != vfs.VFS(fish) {
		t.Fatalf("a view without a Sync client was wrapped: %T", got)
	}
	wrapped, ok := withSyncUploads(fish, client).(*syncUploadVFS)
	if !ok || wrapped.FishVFS != fish || wrapped.client != syncFS(client) {
		t.Fatalf("withSyncUploads(fish, client) = %#v", wrapped)
	}
}

func TestSyncUploadVFSCloneKeepsTheRedirect(t *testing.T) {
	client := &fakeSyncFS{}
	wrapped := withSyncUploads(&netfox.FishVFS{}, client).(*syncUploadVFS)
	clone, ok := wrapped.Clone().(*syncUploadVFS)
	if !ok || clone == wrapped || clone.FishVFS == wrapped.FishVFS || clone.client != syncFS(client) {
		t.Fatalf("Clone() = %#v, want a second view that still uploads over Sync", clone)
	}
}

func TestHybridOpenerRedirectsFishUploadsToSync(t *testing.T) {
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline, Model: "phone"}
	fish := &netfox.FishVFS{}
	client := &fakeSyncFS{}
	var gotFeatures map[string]bool
	opener := &hybridDeviceOpener{
		info: newDeviceInfoService(nil),
		features: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{"shell_v2": true, "sendrecv_v2": true}, nil
		},
		openFish: func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) { return fish, nil },
		uploads: func(d DeviceInfo, features map[string]bool) syncFS {
			if d.Serial != device.Serial {
				t.Fatalf("uploads asked for %q", d.Serial)
			}
			gotFeatures = features
			return client
		},
	}
	mounted, err := opener.OpenDevice(context.Background(), nil, device)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	wrapped, ok := mounted.(*syncUploadVFS)
	if !ok || wrapped.FishVFS != fish || wrapped.client != syncFS(client) {
		t.Fatalf("mounted = %T, want *syncUploadVFS around the FISH+ view", mounted)
	}
	if !gotFeatures["sendrecv_v2"] {
		t.Fatalf("the Sync client was not given the device features: %v", gotFeatures)
	}
	provider, ok := mounted.(vfs.PanelInfoProvider)
	if !ok || provider.PanelInfoKey(vfs.PanelInfoRequest{Path: "/"}) == "" {
		t.Fatal("the wrapped view has no attached Android panel-info provider")
	}
}

func TestOpenSyncReadStreamsARegularFile(t *testing.T) {
	stub := newUploadStub(map[string]SyncEntry{"/sdcard/a.bin": {Name: "a.bin", Mode: 0100644, Size: 4}})
	stub.files["/sdcard/a.bin"] = []byte("data")
	r := openSyncRead(context.Background(), stub, "/sdcard/a.bin")
	if r == nil {
		t.Fatal("a regular file must be read over Sync")
	}
	defer func() { _ = r.Close() }()
	buf := make([]byte, 4)
	if n, err := r.ReadAt(context.Background(), buf, 0); err != nil || n != 4 || string(buf) != "data" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, buf)
	}
	if stub.receiveCall != 1 {
		t.Fatalf("RECV calls = %d, want 1", stub.receiveCall)
	}
}

func TestOpenSyncReadLeavesOddPathsToTheShell(t *testing.T) {
	stub := newUploadStub(map[string]SyncEntry{
		"/sdcard/dir":  {Name: "dir", Mode: 040755},
		"/sdcard/link": {Name: "link", Mode: 0120777},
	})
	for _, target := range []string{"/sdcard/dir", "/sdcard/link", "/sdcard/missing", "relative", "/sdcard/../etc/hosts"} {
		if r := openSyncRead(context.Background(), stub, target); r != nil {
			t.Fatalf("openSyncRead(%q) took the path over Sync", target)
		}
	}
	stub.lstatErr = errors.New("sync service is down")
	if r := openSyncRead(context.Background(), stub, "/sdcard/a.bin"); r != nil {
		t.Fatal("a Sync service that does not answer must leave the read to the shell")
	}
	if stub.receiveCall != 0 {
		t.Fatalf("RECV started for a path that was left to the shell: %d", stub.receiveCall)
	}
}
