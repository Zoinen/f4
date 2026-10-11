//go:build integration

package fileops_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/vmihailenco/msgpack/v5"
)

// Exercise the same device-selection, RPC reads, transfer engine and complete
// hash verification as the hardware test without touching a connected device.
func TestRealDeviceCopyHarnessRPCPathsAndHashes(t *testing.T) {
	core, plugin := testutil.RPCSessionPair(t)
	content := []byte(strings.Repeat("photo-data", 20000))
	const sourcePath = "/iPhone Test/DCIM/100APPLE/photo.jpg"
	plugin.Register("VFS.ReadDir", func(data msgpack.RawMessage) (any, error) {
		var request map[string]string
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		switch request["Path"] {
		case "/":
			return []vfs.VFSItem{
				{Name: "..", IsDir: true},
				{Name: "not a device"},
				{Name: "iPhone Test", IsDir: true},
			}, nil
		case "/iPhone Test":
			return []vfs.VFSItem{{Name: "DCIM", IsDir: true}}, nil
		default:
			return nil, fmt.Errorf("unexpected directory path %q", request["Path"])
		}
	})
	plugin.Register("VFS.Stat", func(data msgpack.RawMessage) (any, error) {
		var request map[string]string
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		if request["Path"] != sourcePath {
			return nil, fmt.Errorf("unexpected stat path %q", request["Path"])
		}
		return vfs.VFSItem{Name: "photo.jpg", Size: int64(len(content)), SizeKnown: true}, nil
	})
	plugin.Register("VFS.Open", func(data msgpack.RawMessage) (any, error) {
		var request plughost.OpenReq
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		if request.Path != sourcePath {
			return nil, fmt.Errorf("unexpected open path %q", request.Path)
		}
		return plughost.OpenRes{ID: 1, Size: int64(len(content))}, nil
	})
	plugin.Register("VFS.ReadAt", func(data msgpack.RawMessage) (any, error) {
		var request plughost.ReadAtReq
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		if request.Off >= int64(len(content)) {
			return []byte{}, nil
		}
		end := min(int64(len(content)), request.Off+int64(request.Len))
		return content[request.Off:end], nil
	})
	plugin.Register("VFS.CloseFile", func(msgpack.RawMessage) (any, error) { return nil, nil })

	ctx := t.Context()
	manager := plughost.NewRPCVFS(realDeviceCopyTransport{ctx: ctx, session: core}, "iOS")
	// No selector must still resolve exactly one device after ignoring parent
	// and ordinary-file rows. The real harness also accepts explicit selectors.
	source := openRealDeviceForCopy(t, ctx, manager, "iOS", "")
	path := source.Join(source.GetPath(), "DCIM", "100APPLE", "photo.jpg")
	if filepath.ToSlash(path) != sourcePath {
		t.Fatalf("device-qualified RPC source = %q, want %q", path, sourcePath)
	}
	destination := vfs.NewOSVFS(t.TempDir())
	target := destination.Join(destination.GetPath(), "photo.jpg")
	state := &fileops.FileOpState{OverwriteAll: true, Buffer: make([]byte, 128*1024)}
	if err := fileops.CopyForRealDeviceIntegration(ctx, source, path, destination, target, state); err != nil {
		t.Fatalf("copy through RPC: %v", err)
	}
	verified := realDeviceCopyVerification{}
	verifyRealDeviceCopyTree(t, ctx, source, path, destination, target, "photo.jpg", &verified)
	if verified.files != 1 || verified.dirs != 0 || verified.bytes != int64(len(content)) {
		t.Fatalf("verified totals = %+v", verified)
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != string(content) {
		t.Fatalf("RPC copy data mismatch: %v", err)
	}
}

func TestRealDeviceCopyTransportHonorsCancellation(t *testing.T) {
	core, _ := testutil.RPCSessionPair(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	transport := realDeviceCopyTransport{ctx: ctx, session: core}
	if err := transport.Call("Plugin.Init", nil, nil); err != context.Canceled {
		t.Fatalf("cancelled RPC call = %v, want context.Canceled", err)
	}
}
