package androidfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type syncPanelInfoStub struct {
	key          string
	snapshot     vfs.PanelInfoSnapshot
	refreshCalls int
}

func (p *syncPanelInfoStub) PanelInfoKey(vfs.PanelInfoRequest) string { return p.key }
func (p *syncPanelInfoStub) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	return p.snapshot, false
}
func (p *syncPanelInfoStub) RefreshPanelInfo(context.Context, vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	p.refreshCalls++
	return p.snapshot, nil
}

func TestSyncVFSPanelInfoProviderSurvivesClone(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	original := newSyncVFS(vfs.NewNullVFS(0), "serial", "phone", client, nil)
	provider := &syncPanelInfoStub{
		key:      "android:serial:/",
		snapshot: vfs.PanelInfoSnapshot{Authoritative: true},
	}
	original.SetPanelInfoProvider(provider)
	clone, ok := original.Clone().(*SyncVFS)
	if !ok || clone == original || clone.panelInfoProvider() != provider {
		t.Fatalf("invalid clone/provider: clone=%T same=%v provider=%T", clone, clone == original, clone.panelInfoProvider())
	}
	req := vfs.PanelInfoRequest{Path: "/"}
	if got := clone.PanelInfoKey(req); got != provider.key {
		t.Fatalf("PanelInfoKey = %q", got)
	}
	if snapshot, fresh := clone.CachedPanelInfo(req); fresh || !snapshot.Authoritative {
		t.Fatalf("CachedPanelInfo = %#v, fresh %v", snapshot, fresh)
	}
	if _, err := clone.RefreshPanelInfo(context.Background(), req); err != nil || provider.refreshCalls != 1 {
		t.Fatalf("RefreshPanelInfo = %v, calls %d", err, provider.refreshCalls)
	}
}

type fakeSyncFS struct {
	entries     map[string]SyncEntry
	lists       map[string][]SyncEntry
	files       map[string][]byte
	receiveCall int
	receiveCtx  []context.Context
	receiveFn   func(context.Context, string) (io.ReadCloser, error)
	sendPath    string
	sendMode    uint32
}

func (f *fakeSyncFS) List(_ context.Context, p string) ([]SyncEntry, error) {
	entries, ok := f.lists[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]SyncEntry(nil), entries...), nil
}
func (f *fakeSyncFS) Lstat(_ context.Context, p string) (SyncEntry, error) {
	e, ok := f.entries[p]
	if !ok {
		return SyncEntry{}, os.ErrNotExist
	}
	return e, nil
}
func (f *fakeSyncFS) Stat(ctx context.Context, p string) (SyncEntry, error) {
	return f.Lstat(ctx, p)
}
func (f *fakeSyncFS) Receive(ctx context.Context, p string) (io.ReadCloser, error) {
	f.receiveCall++
	f.receiveCtx = append(f.receiveCtx, ctx)
	if f.receiveFn != nil {
		return f.receiveFn(ctx, p)
	}
	data, ok := f.files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (f *fakeSyncFS) Send(_ context.Context, p string, mode uint32, _ time.Time) (io.WriteCloser, error) {
	f.sendPath, f.sendMode = p, mode
	return &fakeSyncWriter{close: func(data []byte) { f.files[p] = data }}, nil
}

type fakeSyncWriter struct {
	bytes.Buffer
	close func([]byte)
}

func (w *fakeSyncWriter) Close() error {
	w.close(append([]byte(nil), w.Bytes()...))
	return nil
}

func TestSyncVFSReadDirAndMetadata(t *testing.T) {
	client := &fakeSyncFS{
		entries: map[string]SyncEntry{},
		lists: map[string][]SyncEntry{"/": {
			{Name: ".", Mode: remoteModeDir | 0755},
			{Name: "..", Mode: remoteModeDir | 0755},
			{Name: ".hidden", Mode: remoteModeDir | 0750, UID: 2000, GID: 2000, Inode: 4, Device: 2, metadataV2: true},
			{Name: "run.sh", Mode: 0100755, Size: 9, ModTime: time.Unix(10, 0)},
		}},
	}
	fs := newSyncVFS(nil, "serial", "phone [ADB Sync]", client, nil)
	var got []vfs.VFSItem
	if err := fs.ReadDir(context.Background(), "/", func(chunk []vfs.VFSItem) { got = append(got, chunk...) }); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ReadDir returned %d items", len(got))
	}
	if !got[0].IsDir || !got[0].IsHidden || got[0].UnixMode != remoteModeDir|0750 || got[0].Inode != 4 || got[0].Uid != 2000 {
		t.Fatalf("directory metadata = %#v", got[0])
	}
	if got[1].Size != 9 || !got[1].IsExecutable || got[1].MTime.Unix() != 10 {
		t.Fatalf("file metadata = %#v", got[1])
	}
}

func TestSyncEntryItemMarksLegacyOwnershipUnknown(t *testing.T) {
	item := syncEntryItem(SyncEntry{Name: "legacy", Mode: 0100644})
	if item.Uid != -1 || item.Gid != -1 {
		t.Fatalf("legacy ownership = %d:%d, want unknown", item.Uid, item.Gid)
	}
}

func TestSyncReadFileDoesNotBindStreamToOneReadCallContext(t *testing.T) {
	client := &fakeSyncFS{files: map[string][]byte{"/file": []byte("abcd")}}
	file := &syncReadFile{client: client, path: "/file", size: 4}
	callCtx, cancel := context.WithCancel(context.Background())
	one := make([]byte, 1)
	if n, err := file.Read(callCtx, one); err != nil || n != 1 || string(one) != "a" {
		t.Fatalf("first Read = %d, %v, %q", n, err, one)
	}
	cancel()
	if len(client.receiveCtx) != 1 || client.receiveCtx[0].Err() != nil {
		t.Fatal("RECV lifetime was bound to the completed per-call context")
	}
	next := make([]byte, 1)
	if n, err := file.Read(context.Background(), next); err != nil || n != 1 || string(next) != "b" {
		t.Fatalf("second Read = %d, %v, %q", n, err, next)
	}
	_ = file.Close()
}

type alwaysErrorReader struct{ err error }

func (r alwaysErrorReader) Read([]byte) (int, error) { return 0, r.err }
func (alwaysErrorReader) Close() error               { return nil }

func TestSyncReadFileDoesNotRestartAfterTerminalReadError(t *testing.T) {
	wantErr := errors.New("connection lost")
	client := &fakeSyncFS{receiveFn: func(context.Context, string) (io.ReadCloser, error) {
		return alwaysErrorReader{err: wantErr}, nil
	}}
	file := &syncReadFile{client: client, path: "/file"}
	if _, err := file.Read(context.Background(), make([]byte, 1)); !errors.Is(err, wantErr) {
		t.Fatalf("first Read error = %v", err)
	}
	if _, err := file.Read(context.Background(), make([]byte, 1)); !errors.Is(err, wantErr) {
		t.Fatalf("second Read error = %v", err)
	}
	if client.receiveCall != 1 {
		t.Fatalf("RECV calls = %d, want 1", client.receiveCall)
	}
	_ = file.Close()
}

func TestSyncReadFileStreamsThenMaterializesForReadAt(t *testing.T) {
	data := []byte("abcdefgh")
	client := &fakeSyncFS{
		entries: map[string]SyncEntry{"/file": {Name: "file", Mode: 0100644, Size: uint64(len(data))}},
		files:   map[string][]byte{"/file": data},
	}
	fs := newSyncVFS(nil, "serial", "phone", client, nil)
	opened, err := fs.Open(context.Background(), "/file")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	file := opened.(*syncReadFile)
	if client.receiveCall != 0 {
		t.Fatal("Open eagerly started RECV")
	}

	buf := make([]byte, 3)
	if n, err := file.Read(context.Background(), buf); err != nil || n != 3 || string(buf) != "abc" {
		t.Fatalf("Read = %d, %v, %q", n, err, buf)
	}
	if client.receiveCall != 1 || file.temp != nil {
		t.Fatalf("sequential read did not remain streaming: calls=%d temp=%v", client.receiveCall, file.temp)
	}

	at := make([]byte, 2)
	if n, err := file.ReadAt(context.Background(), at, 4); err != nil || n != 2 || string(at) != "ef" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, at)
	}
	if client.receiveCall != 2 || file.temp == nil {
		t.Fatalf("ReadAt did not materialize exactly once: calls=%d temp=%v", client.receiveCall, file.temp)
	}

	next := make([]byte, 3)
	if n, err := file.Read(context.Background(), next); err != nil || n != 3 || string(next) != "def" {
		t.Fatalf("Read after ReadAt = %d, %v, %q", n, err, next)
	}
	tmp := file.tmp
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file still exists: %v", err)
	}
}

func TestSyncVFSMutationsAreQuotedAndGuarded(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	var commands []string
	run := func(_ context.Context, serial, command string) (shellResult, error) {
		if serial != "serial" {
			t.Fatalf("serial = %q", serial)
		}
		commands = append(commands, command)
		return shellResult{}, nil
	}
	fs := newSyncVFS(nil, "serial", "phone", client, run)
	ctx := context.Background()
	if err := fs.MkDir(ctx, "/data/local/tmp/a'b"); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	if err := fs.Rename(ctx, "/data/local/tmp/a'b", "/data/local/tmp/$new"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := fs.Remove(ctx, "/data/local/tmp/$new"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{
		"mkdir -p -- '/data/local/tmp/a'\"'\"'b'",
		"mv -f -- '/data/local/tmp/a'\"'\"'b' '/data/local/tmp/$new'",
		"rm -rf -- '/data/local/tmp/$new'",
	}
	if len(commands) != len(want) {
		t.Fatalf("commands = %#v", commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Errorf("command %d = %q, want %q", i, commands[i], want[i])
		}
	}
	for _, dangerous := range []string{"/", "/data/local/tmp/a/../..", "../outside"} {
		if err := fs.Remove(ctx, dangerous); err == nil {
			t.Errorf("Remove(%q) succeeded", dangerous)
		}
	}
}

func TestSyncVFSRunCommandUsesDeviceDirectoryAndStreamsOutput(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	var gotCommand string
	fs := newSyncVFS(nil, "serial", "phone", client, func(_ context.Context, serial, command string) (shellResult, error) {
		if serial != "serial" {
			t.Fatalf("serial = %q", serial)
		}
		gotCommand = command
		return shellResult{
			Stdout:   []byte("first\n\nsecond\n"),
			Stderr:   []byte("warning\r\n"),
			ExitCode: 7,
		}, nil
	})
	var lines []string
	code, err := fs.RunCommand(context.Background(), "/sd card/a'b", "ls -la # note", func(line string) {
		lines = append(lines, line)
	})
	if err != nil || code != 7 {
		t.Fatalf("RunCommand = code %d, err %v", code, err)
	}
	wantCommand := "cd '/sd card/a'\"'\"'b' && (\nls -la # note\n) </dev/null"
	if gotCommand != wantCommand {
		t.Fatalf("device command = %q, want %q", gotCommand, wantCommand)
	}
	wantLines := []string{"first", "", "second", "warning"}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Fatalf("output lines = %#v, want %#v", lines, wantLines)
	}
}

func TestSyncVFSRunCommandStreamsMergedPacketOrder(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	fs := newSyncVFS(nil, "serial", "phone", client, func(context.Context, string, string) (shellResult, error) {
		t.Fatal("collected compatibility runner was called")
		return shellResult{}, nil
	})
	var gotCommand string
	fs.stream = func(_ context.Context, serial, command string, emit func([]byte)) (int, error) {
		if serial != "serial" {
			t.Fatalf("serial = %q", serial)
		}
		gotCommand = command
		emit([]byte("stdout"))
		emit([]byte("+stderr\r\n"))
		emit([]byte("tail"))
		return 7, nil
	}

	var lines []string
	code, err := fs.RunCommand(context.Background(), "/sd card/a'b", "ls -la", func(line string) {
		lines = append(lines, line)
	})
	if err != nil || code != 7 {
		t.Fatalf("RunCommand = code %d, err %v", code, err)
	}
	if want := "cd '/sd card/a'\"'\"'b' && (\nls -la\n) </dev/null"; gotCommand != want {
		t.Fatalf("device command = %q, want %q", gotCommand, want)
	}
	if want := []string{"stdout+stderr", "tail"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("merged lines = %#v, want %#v", lines, want)
	}
}

func TestSyncVFSRunCommandStreamingCancellationFlushesPartialLine(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	fs := newSyncVFS(nil, "serial", "phone", client, nil)
	started := make(chan struct{})
	fs.stream = func(ctx context.Context, _, _ string, emit func([]byte)) (int, error) {
		emit([]byte("partial"))
		close(started)
		<-ctx.Done()
		return -1, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	var lines []string
	done := make(chan error, 1)
	go func() {
		_, err := fs.RunCommand(ctx, "/", "sleep", func(line string) { lines = append(lines, line) })
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunCommand error = %v, want context.Canceled", err)
	}
	if want := []string{"partial"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("flushed output = %#v, want %#v", lines, want)
	}
}

func TestAndroidCommandLineWriterBoundsUnterminatedOutput(t *testing.T) {
	var chunks []string
	w := newAndroidCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	payload := bytes.Repeat([]byte{'x'}, androidCommandOutputChunkBytes*2+17)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if len(w.pending) > androidCommandOutputChunkBytes {
		t.Fatalf("pending output = %d bytes", len(w.pending))
	}
	w.Flush()
	total := 0
	for _, chunk := range chunks {
		if len(chunk) > androidCommandOutputChunkBytes {
			t.Fatalf("callback chunk = %d bytes", len(chunk))
		}
		total += len(chunk)
	}
	if total != len(payload) {
		t.Fatalf("streamed bytes = %d, want %d", total, len(payload))
	}
}

func TestAndroidCommandLineWriterDoesNotSplitUTF8Rune(t *testing.T) {
	var chunks []string
	w := newAndroidCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	payload := append(bytes.Repeat([]byte{'x'}, androidCommandOutputChunkBytes-1), []byte("яz")...)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if got := strings.Join(chunks, ""); got != string(payload) {
		t.Fatalf("joined chunks lost UTF-8 data at boundary: %q", got[len(got)-8:])
	}
}

func TestSyncVFSCreateAndSetAttributes(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	var commands []string
	fs := newSyncVFS(nil, "serial", "phone", client, func(_ context.Context, _, command string) (shellResult, error) {
		commands = append(commands, command)
		return shellResult{}, nil
	})
	w, err := fs.Create(context.Background(), "/data/local/tmp/new")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := io.WriteString(w, "payload"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("writer Close: %v", err)
	}
	if client.sendPath != "/data/local/tmp/new" || client.sendMode != 0644 || string(client.files[client.sendPath]) != "payload" {
		t.Fatalf("send = path %q mode %#o data %q", client.sendPath, client.sendMode, client.files[client.sendPath])
	}

	stamp := time.Unix(1700000000, 0)
	item := vfs.VFSItem{UnixMode: 0100600, Uid: -1, Gid: -1, MTime: stamp, ATime: stamp.Add(-time.Hour)}
	if err := fs.SetAttributes(context.Background(), "/data/local/tmp/new", item); err != nil {
		t.Fatalf("SetAttributes: %v", err)
	}
	want := []string{
		"chmod 600 -- '/data/local/tmp/new'",
		"touch -m -d @" + strconv.FormatInt(stamp.Unix(), 10) + " -- '/data/local/tmp/new'",
		"touch -a -d @" + strconv.FormatInt(stamp.Add(-time.Hour).Unix(), 10) + " -- '/data/local/tmp/new'",
	}
	if len(commands) != len(want) {
		t.Fatalf("commands = %#v", commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Errorf("command %d = %q, want %q", i, commands[i], want[i])
		}
	}
}

func TestSyncVFSCreatePrivateCommandFileUsesSendMode0600(t *testing.T) {
	client := &fakeSyncFS{entries: map[string]SyncEntry{}, files: map[string][]byte{}}
	fs := newSyncVFS(nil, "serial", "phone", client, nil)
	var creator vfs.PrivateCommandFileCreator = fs
	w, err := creator.CreatePrivateCommandFile(context.Background(), "/data/local/tmp/list")
	if err != nil {
		t.Fatalf("CreatePrivateCommandFile: %v", err)
	}
	if _, err := io.WriteString(w, "private\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if client.sendPath != "/data/local/tmp/list" || client.sendMode != 0600 || string(client.files[client.sendPath]) != "private\n" {
		t.Fatalf("private send = path %q mode %#o data %q", client.sendPath, client.sendMode, client.files[client.sendPath])
	}
}
