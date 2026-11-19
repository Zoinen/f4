package f4vfs

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
)

type nativeURIPlugin struct {
	f4plugin.Plugin
	*Bridge
}

type blockingPreviewVFS struct {
	vfs.VFS
	reader *blockingPreviewReader
}

func (v *blockingPreviewVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return v.reader, nil
}

type blockingPreviewReader struct {
	started chan struct{}
	release chan struct{}
	closed  chan struct{}
}

func (r *blockingPreviewReader) ReadAt(ctx context.Context, _ []byte, _ int64) (int, error) {
	close(r.started)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-r.release:
		return 0, io.EOF
	}
}

func (r *blockingPreviewReader) Read(ctx context.Context, p []byte) (int, error) {
	return r.ReadAt(ctx, p, 0)
}

func (r *blockingPreviewReader) Size() int64 { return 1 }

func (r *blockingPreviewReader) Close() error {
	close(r.closed)
	return nil
}

func TestURIBridgeNavigatesWhilePreviewReadIsBlocked(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "child")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	reader := &blockingPreviewReader{started: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	bridge := NewBridge(func(context.Context, string) (vfs.VFS, error) {
		return &blockingPreviewVFS{VFS: vfs.NewOSVFS(root), reader: reader}, nil
	})
	defer bridge.CloseVFSURIs()
	defer close(reader.release)

	opened, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "openURI"})
	if err != nil {
		t.Fatal(err)
	}
	mount := opened.(f4plugin.URIMount).ID
	opened, err = bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "open", Mount: mount})
	if err != nil {
		t.Fatal(err)
	}
	file := opened.(f4plugin.URIFile).ID

	read := make(chan error, 1)
	go func() {
		_, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "readAt", Mount: mount, File: file, Length: 1})
		read <- err
	}()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("preview read did not start")
	}

	navigated := make(chan error, 1)
	go func() {
		_, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "set", Mount: mount, Path: target})
		navigated <- err
	}()
	select {
	case err := <-navigated:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("navigation waited for the preview read")
	}

	closed := make(chan error, 1)
	go func() {
		_, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "closeFile", Mount: mount, File: file})
		closed <- err
	}()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not cancel the preview read")
	}
	if err := <-read; !errors.Is(err, context.Canceled) {
		t.Fatalf("preview read ended with %v, want cancellation", err)
	}
	select {
	case <-reader.closed:
	default:
		t.Fatal("preview reader was not closed")
	}
}

type blockingPreviewStat struct {
	vfs.VFS
	started chan struct{}
}

func (v *blockingPreviewStat) Clone() vfs.VFS {
	return &blockingPreviewStat{VFS: v.VFS.Clone(), started: v.started}
}

func (v *blockingPreviewStat) Stat(ctx context.Context, _ string) (vfs.VFSItem, error) {
	close(v.started)
	<-ctx.Done()
	return vfs.VFSItem{}, ctx.Err()
}

func TestURIBridgeNavigatesWhilePreviewMountStatIsBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	root := t.TempDir()
	bridge := NewBridge(func(context.Context, string) (vfs.VFS, error) {
		return &blockingPreviewStat{VFS: vfs.NewOSVFS(root), started: started}, nil
	})
	defer bridge.CloseVFSURIs()
	defer cancel()

	opened, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "openURI"})
	if err != nil {
		t.Fatal(err)
	}
	mount := opened.(f4plugin.URIMount).ID
	cloned, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "clone", Mount: mount})
	if err != nil {
		t.Fatal(err)
	}
	previewMount := cloned.(f4plugin.URIMount).ID
	stat := make(chan error, 1)
	go func() {
		_, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "stat", Mount: previewMount})
		stat <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preview stat did not start")
	}

	listed := make(chan error, 1)
	go func() {
		_, err := bridge.CallVFSURI(ctx, f4plugin.URIRequest{Operation: "readDir", Mount: mount, Path: root})
		listed <- err
	}()
	select {
	case err := <-listed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("listing waited for the preview mount's stat")
	}
	cancel()
	if err := <-stat; !errors.Is(err, context.Canceled) {
		t.Fatalf("preview stat ended with %v, want cancellation", err)
	}
}

func TestURIBridgeNativePathsCloneAndPreview(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "100% tail.txt")
	if err := os.WriteFile(filePath, []byte("tail"), 0600); err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	done := make(chan struct{})
	go func() {
		_ = f4plugin.Serve(&nativeURIPlugin{Bridge: NewBridge(func(context.Context, string) (vfs.VFS, error) { return vfs.NewOSVFS(root), nil })}, right, right)
		_ = right.Close()
		close(done)
	}()
	session := f4rpc.NewSession(left, left)
	served := make(chan struct{})
	go func() { _ = session.Serve(); close(served) }()
	t.Cleanup(func() { _ = left.Close(); <-done; <-served })
	var mounted f4plugin.URIMount
	if err := session.Call("VFS.URI", f4plugin.URIRequest{Operation: "openURI", Path: "ios://Phone/"}, &mounted); err != nil {
		t.Fatal(err)
	}
	if mounted.ID == 0 || mounted.Path != root {
		t.Fatalf("native mount = %+v", mounted)
	}
	var joined string
	if err := session.Call("VFS.URI", f4plugin.URIRequest{Mount: mounted.ID, Operation: "join", Elements: []string{root, "100% tail.txt"}}, &joined); err != nil || joined != filePath {
		t.Fatalf("native Join = %q, %v", joined, err)
	}
	var cloned f4plugin.URIMount
	if err := session.Call("VFS.URI", f4plugin.URIRequest{Mount: mounted.ID, Operation: "clone"}, &cloned); err != nil {
		t.Fatal(err)
	}
	if cloned.ID == mounted.ID || cloned.Path != mounted.Path {
		t.Fatalf("clone = %+v", cloned)
	}
	var file f4plugin.URIFile
	if err := session.Call("VFS.URI", f4plugin.URIRequest{Mount: mounted.ID, Operation: "open", Path: joined}, &file); err != nil {
		t.Fatal(err)
	}
	bytes := []byte{}
	if err := session.Call("VFS.URI", f4plugin.URIRequest{Mount: mounted.ID, Operation: "readAt", File: file.ID, Offset: 1, Length: 8}, &bytes); err != nil || string(bytes) != "ail" {
		t.Fatalf("short preview = %q, %v", bytes, err)
	}
	if err := session.Call("VFS.URI", f4plugin.URIRequest{Mount: mounted.ID, Operation: "closeFile", File: file.ID}, nil); err != nil {
		t.Fatal(err)
	}
}
