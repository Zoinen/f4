package cloudfox

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

type rpcRenameMount struct {
	vfs.VFS
	old, new string
}

func (m *rpcRenameMount) Rename(_ context.Context, old, new string) error {
	m.old, m.new = old, new
	return nil
}

func TestRPCPluginRenameMountBoundary(t *testing.T) {
	for _, destination := range []string{"/other/renamed", "/storage", "/storage/renamed"} {
		t.Run(destination, func(t *testing.T) {
			mount := &rpcRenameMount{}
			plugin := &RPCPlugin{conns: map[string]vfs.VFS{"storage": mount}}
			err := plugin.Rename(DriveName, "/storage/original", destination)
			if destination == "/storage/renamed" {
				if err != nil {
					t.Fatal(err)
				}
				if mount.old != "original" || mount.new != "renamed" {
					t.Fatalf("rename paths = %q -> %q", mount.old, mount.new)
				}
				return
			}
			if err == nil {
				t.Fatal("rename outside the source connection root succeeded")
			}
			if mount.old != "" || mount.new != "" {
				t.Fatal("source mutated after rejected rename")
			}
		})
	}
}
