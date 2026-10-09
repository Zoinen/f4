package netfox

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/unxed/f4/vfs"
)

// This file closes coverage gaps in sftp_vfs.go's platform-independent logic:
// path/metadata handling, the connection-refcounting helpers, context-guard
// early returns, and the stored-connection provider. It reuses the real
// in-memory SFTP server fixture from sftp_readat_test.go (startTestSFTPServer)
// instead of hitting an actual network, and the fakeSFTPCommandSession fixture
// from sftp_command_test.go for the command-runner internals.

func TestSFTPConnectionRefsNilReceiverIsNoop(t *testing.T) {
	var r *sftpConnectionRefs
	r.retain() // Must not panic.
	if err := r.release(); err != nil {
		t.Fatalf("release on a nil *sftpConnectionRefs = %v, want nil", err)
	}
}

func TestSFTPConnectionRefsReleaseClosesClientOnLastRef(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	shared := &sftpConnectionRefs{refs: 1, client: client}

	if err := shared.release(); err != nil {
		t.Fatalf("release = %v, want nil", err)
	}
	shared.Lock()
	clientCleared, sshCleared := shared.client == nil, shared.ssh == nil
	shared.Unlock()
	if !clientCleared || !sshCleared {
		t.Fatal("release on the last reference did not clear the shared connection fields")
	}
}

func TestSFTPVFSCloneWithoutSharedReturnsSameInstance(t *testing.T) {
	v := &SFTPVFS{path: "/solo"}
	if clone := v.Clone(); clone != v {
		t.Fatalf("Clone() without a shared connection = %#v, want the same instance", clone)
	}
}

func TestSFTPVFSCloseWithoutSharedClosesClientDirectly(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: client}
	if err := v.Close(); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
	if err := v.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil (closeOnce must make it idempotent)", err)
	}
}

func TestSFTPVFSSetPathAcceptsDirectoriesAndRejectsOthers(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	if err := client.Mkdir("/dir"); err != nil {
		t.Fatal(err)
	}
	wf, err := client.Create("/dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf.Close(); err != nil {
		t.Fatal(err)
	}

	v := &SFTPVFS{client: client, path: "/"}
	if err := v.SetPath("dir"); err != nil {
		t.Fatalf("SetPath(relative dir) = %v", err)
	}
	if got := v.GetPath(); got != "/dir" {
		t.Fatalf("path after relative SetPath = %q, want /dir", got)
	}
	if err := v.SetPath("/dir"); err != nil {
		t.Fatalf("SetPath(absolute dir) = %v", err)
	}
	if err := v.SetPath("file.txt"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("SetPath(non-directory) = %v, want os.ErrInvalid", err)
	}
	if got := v.GetPath(); got != "/dir" {
		t.Fatalf("a failed SetPath must not change the current path, got %q", got)
	}
	if err := v.SetPath("/missing"); err == nil {
		t.Fatal("SetPath(missing) unexpectedly succeeded")
	}
}

func TestSFTPVFSReadDirReportsMetadataAndHiddenFiles(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	if err := client.Mkdir("/list"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/list/.hidden", "/list/visible.txt"} {
		wf, err := client.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := wf.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		if err := wf.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Mkdir("/list/subdir"); err != nil {
		t.Fatal(err)
	}

	v := &SFTPVFS{client: client}
	var got []vfs.VFSItem
	if err := v.ReadDir(context.Background(), "/list", func(items []vfs.VFSItem) {
		got = append(got, items...)
	}); err != nil {
		t.Fatalf("ReadDir = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ReadDir returned %d items, want 3", len(got))
	}

	byName := map[string]vfs.VFSItem{}
	for _, item := range got {
		byName[item.Name] = item
	}

	hidden, ok := byName[".hidden"]
	if !ok || !hidden.IsHidden || hidden.IsDir {
		t.Fatalf("hidden file metadata = %+v", hidden)
	}
	visible, ok := byName["visible.txt"]
	if !ok || visible.IsHidden || visible.IsDir || visible.Size != 4 {
		t.Fatalf("visible file metadata = %+v", visible)
	}
	if !visible.HasMetadata(vfs.MetadataUID) {
		t.Fatalf("ReadDir did not report UID metadata: %+v", visible)
	}
	sub, ok := byName["subdir"]
	if !ok || !sub.IsDir {
		t.Fatalf("subdir metadata = %+v", sub)
	}

	// A context canceled before the loop reaches an entry aborts ReadDir
	// instead of finishing the listing.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.ReadDir(canceled, "/list", func([]vfs.VFSItem) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadDir with a canceled context = %v, want context.Canceled", err)
	}

	if err := v.ReadDir(context.Background(), "/missing", func([]vfs.VFSItem) {}); err == nil {
		t.Fatal("ReadDir(missing) unexpectedly succeeded")
	}
}

func TestSFTPVFSStatReportsMetadataAndMissingError(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	wf, err := client.Create("/report.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wf.Write([]byte("hello!")); err != nil {
		t.Fatal(err)
	}
	if err := wf.Close(); err != nil {
		t.Fatal(err)
	}

	v := &SFTPVFS{client: client}
	item, err := v.Stat(context.Background(), "/report.bin")
	if err != nil {
		t.Fatalf("Stat = %v", err)
	}
	if item.Name != "report.bin" || item.Size != 6 || item.IsDir {
		t.Fatalf("Stat metadata = %+v", item)
	}
	if !item.HasMetadata(vfs.MetadataUID) {
		t.Fatalf("Stat did not report UID metadata: %+v", item)
	}

	if _, err := v.Stat(context.Background(), "/missing"); err == nil {
		t.Fatal("Stat(missing) unexpectedly succeeded")
	}
}

func TestSFTPVFSMkDirRemoveAndRename(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: client}

	if err := v.MkDir(context.Background(), "/work"); err != nil {
		t.Fatalf("MkDir = %v", err)
	}
	wf, err := client.Create("/work/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Mkdir("/work/sub"); err != nil {
		t.Fatal(err)
	}
	wf2, err := client.Create("/work/sub/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf2.Close(); err != nil {
		t.Fatal(err)
	}

	if err := v.Rename(context.Background(), "/work/a.txt", "/work/renamed.txt"); err != nil {
		t.Fatalf("Rename = %v", err)
	}
	if _, err := client.Lstat("/work/renamed.txt"); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	// A single non-directory Remove takes the direct-delete branch.
	wf3, err := client.Create("/lonely.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf3.Close(); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove(context.Background(), "/lonely.txt"); err != nil {
		t.Fatalf("Remove(file) = %v", err)
	}
	if _, err := client.Lstat("/lonely.txt"); err == nil {
		t.Fatal("Remove(file) did not delete it")
	}

	// A directory Remove walks and deletes the whole tree bottom-up.
	if err := v.Remove(context.Background(), "/work"); err != nil {
		t.Fatalf("Remove(recursive) = %v", err)
	}
	if _, err := client.Lstat("/work"); err == nil {
		t.Fatal("Remove(recursive) did not delete the directory tree")
	}

	if err := v.Remove(context.Background(), "/missing"); err == nil {
		t.Fatal("Remove(missing) unexpectedly succeeded")
	}
}

func TestSFTPVFSSetAttributesAppliesModeOwnerAndTimes(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	wf, err := client.Create("/attrs.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf.Close(); err != nil {
		t.Fatal(err)
	}

	v := &SFTPVFS{client: client}
	mtime := time.Unix(1_700_000_000, 0)

	// Mode, owner and both timestamps set: exercises Chmod, Chown and Chtimes.
	if err := v.SetAttributes(context.Background(), "/attrs.txt", vfs.VFSItem{
		UnixMode: 0644, Uid: 1000, Gid: 1000, MTime: mtime,
	}); err != nil {
		t.Fatalf("SetAttributes(full) = %v", err)
	}

	// Only ATime set: MTime is mirrored from it, so Chtimes still runs.
	if err := v.SetAttributes(context.Background(), "/attrs.txt", vfs.VFSItem{ATime: mtime}); err != nil {
		t.Fatalf("SetAttributes(atime only) = %v", err)
	}

	// Uid == -1 || Gid == -1 skips Chown; a zero-value item otherwise also
	// skips Chmod (UnixMode == 0) and Chtimes (both times zero).
	if err := v.SetAttributes(context.Background(), "/attrs.txt", vfs.VFSItem{Uid: -1, Gid: -1}); err != nil {
		t.Fatalf("SetAttributes(no owner change) = %v", err)
	}

	if err := v.SetAttributes(context.Background(), "/missing", vfs.VFSItem{UnixMode: 0644}); err == nil {
		t.Fatal("SetAttributes(missing) unexpectedly succeeded")
	}
}

func TestSFTPVFSOpenReportsSizeAndMissingError(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	wf, err := client.Create("/open.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wf.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := wf.Close(); err != nil {
		t.Fatal(err)
	}

	v := &SFTPVFS{client: client}
	f, err := v.Open(context.Background(), "/open.txt")
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer func() { _ = f.Close() }()
	if f.Size() != 7 {
		t.Fatalf("Size() = %d, want 7", f.Size())
	}
	buf := make([]byte, 7)
	n, err := f.Read(context.Background(), buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read = %v", err)
	}
	if n != 7 || string(buf) != "payload" {
		t.Fatalf("Read = (%d, %q), want (7, \"payload\")", n, buf)
	}

	if _, err := v.Open(context.Background(), "/missing"); err == nil {
		t.Fatal("Open(missing) unexpectedly succeeded")
	}
}

func TestSFTPVFSCreateWritesNewFile(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: client}

	wc, err := v.Create(context.Background(), "/created.txt")
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	if _, err := wc.Write([]byte("new content")); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	rf, err := client.Open("/created.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rf.Close() }()
	got, err := io.ReadAll(rf)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new content" {
		t.Fatalf("stored content = %q, want %q", got, "new content")
	}
}

func TestSFTPVFSSymlinkReadlinkAndOpenWriteAtRespectCanceledContext(t *testing.T) {
	v := &SFTPVFS{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := v.Readlink(ctx, "/x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Readlink(canceled) = %v, want context.Canceled", err)
	}
	if err := v.Symlink(ctx, "/target", "/link"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Symlink(canceled) = %v, want context.Canceled", err)
	}
	if _, err := v.OpenWriteAt(ctx, "/x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenWriteAt(canceled) = %v, want context.Canceled", err)
	}
}

func TestSFTPVFSSymlinkReadlinkAndOpenWriteAtAgainstServer(t *testing.T) {
	client, _ := startTestSFTPServer(t)
	wf, err := client.Create("/target.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wf.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if err := wf.Close(); err != nil {
		t.Fatal(err)
	}

	v := &SFTPVFS{client: client}
	if err := v.Symlink(context.Background(), "/target.txt", "/link.txt"); err != nil {
		t.Fatalf("Symlink = %v", err)
	}
	if got, err := v.Readlink(context.Background(), "/link.txt"); err != nil || got != "/target.txt" {
		t.Fatalf("Readlink = (%q, %v), want (\"/target.txt\", nil)", got, err)
	}

	wac, err := v.OpenWriteAt(context.Background(), "/target.txt")
	if err != nil {
		t.Fatalf("OpenWriteAt = %v", err)
	}
	if _, err := wac.WriteAt([]byte("AB"), 2); err != nil {
		t.Fatal(err)
	}
	if err := wac.Close(); err != nil {
		t.Fatal(err)
	}

	rf, err := client.Open("/target.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rf.Close() }()
	content, err := io.ReadAll(rf)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "01AB456789" {
		t.Fatalf("content after OpenWriteAt = %q, want %q", content, "01AB456789")
	}
}

func TestSFTPVFSRunCommandValidatesBeforeDialingSSH(t *testing.T) {
	v := &SFTPVFS{path: "/home"}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.RunCommand(canceled, "", "echo hi", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunCommand(canceled context) = %v, want context.Canceled", err)
	}

	if _, err := v.RunCommand(context.Background(), "", "echo hi", nil); err == nil {
		t.Fatal("RunCommand without an SSH connection unexpectedly succeeded")
	}

	// A non-nil (if unconnected) SSH client clears the "no connection" guard,
	// letting the blank-command validation below run and return before any
	// attempt to actually use that connection.
	v.ssh = &ssh.Client{}
	if _, err := v.RunCommand(context.Background(), "", "   ", nil); err == nil {
		t.Fatal("RunCommand with a blank command unexpectedly succeeded")
	}
}

func TestSFTPVFSCommandCodecDefaultsToPassthrough(t *testing.T) {
	for _, cp := range []string{"", "65001"} {
		codec, err := (&SFTPVFS{codepage: cp}).commandCodec()
		if err != nil {
			t.Fatalf("commandCodec(%q) = %v", cp, err)
		}
		if codec.encode != nil || codec.decode != nil {
			t.Fatalf("commandCodec(%q) = %+v, want the zero-value passthrough codec", cp, codec)
		}
	}
}

func TestRunSFTPCommandSessionWithCodecHonorsPreCanceledContext(t *testing.T) {
	session := newFakeSFTPCommandSession()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	code, err := runSFTPCommandSessionWithCodec(canceled, session, "/", "echo hi", nil, sftpCommandCodec{})
	if code != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("result = (%d, %v), want (0, context.Canceled)", code, err)
	}
	select {
	case <-session.closed:
	default:
		t.Fatal("a pre-canceled context must still close the session")
	}
}

func TestRunSFTPCommandSessionWithCodecPropagatesEncodeError(t *testing.T) {
	session := newFakeSFTPCommandSession()
	wantErr := errors.New("encode failed")
	codec := sftpCommandCodec{encode: func(string) (string, error) { return "", wantErr }}

	code, err := runSFTPCommandSessionWithCodec(context.Background(), session, "/", "echo hi", nil, codec)
	if code != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("result = (%d, %v), want (0, %v)", code, err, wantErr)
	}
	if session.command != "" {
		t.Fatalf("Start was called with %q despite the encode failure", session.command)
	}
}

func TestRunSFTPCommandSessionPropagatesStartError(t *testing.T) {
	session := newFakeSFTPCommandSession()
	session.startErr = errors.New("start failed")

	code, err := runSFTPCommandSession(context.Background(), session, "/", "echo hi", nil)
	if code != 0 || !errors.Is(err, session.startErr) {
		t.Fatalf("result = (%d, %v), want (0, %v)", code, err, session.startErr)
	}
}

func TestSFTPProviderCanOpenValidatesParentAndConfigType(t *testing.T) {
	provider := &sftpProvider{}

	if provider.CanOpen(context.Background(), vfs.NewNullVFS(0), "site") {
		t.Fatal("CanOpen accepted a parent that is not a NetFox wrapper")
	}

	manager := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	parent := &netFoxVFSWrapper{NetFoxVFS: manager}

	if err := manager.SaveConfig("ftp-site", NetFoxConfig{Type: "ftp", Host: "h"}); err != nil {
		t.Fatal(err)
	}
	if provider.CanOpen(context.Background(), parent, "ftp-site") {
		t.Fatal("CanOpen accepted a stored connection of a different type")
	}

	if err := manager.SaveConfig("sftp-site", NetFoxConfig{Type: "sftp", Host: "h"}); err != nil {
		t.Fatal(err)
	}
	if !provider.CanOpen(context.Background(), parent, "sftp-site") {
		t.Fatal("CanOpen rejected a stored sftp connection")
	}

	if err := manager.SaveConfig("default-site", NetFoxConfig{Host: "h"}); err != nil {
		t.Fatal(err)
	}
	if !provider.CanOpen(context.Background(), parent, "default-site") {
		t.Fatal("CanOpen rejected a stored connection with an empty type (defaults to sftp)")
	}

	if provider.CanOpen(context.Background(), parent, "missing-site") {
		t.Fatal("CanOpen accepted a nonexistent stored connection")
	}
}

func TestSFTPProtocolHandlerBuildExtraUIIsANoOpStub(t *testing.T) {
	ph := &sftpProtocolHandler{}
	el, save := ph.BuildExtraUI(&NetFoxConfig{}, 0, 0, 0, 0)
	if el != nil {
		t.Fatalf("BuildExtraUI element = %v, want nil", el)
	}
	if save == nil {
		t.Fatal("BuildExtraUI save callback is nil")
	}
	save() // Must not panic.
}
