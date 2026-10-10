package f4vfs

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
)

type nativeURIPlugin struct {
	f4plugin.Plugin
	*Bridge
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
