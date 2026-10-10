package androidfs

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
	for _, destination := range []string{"/other/renamed", "/phone", "/phone/renamed"} {
		t.Run(destination, func(t *testing.T) {
			mount := &rpcRenameMount{}
			plugin := &RPCPlugin{devices: map[string]vfs.VFS{"phone": mount}}
			err := plugin.Rename("Android", "/phone/original", destination)
			if destination == "/phone/renamed" {
				if err != nil {
					t.Fatal(err)
				}
				if mount.old != "original" || mount.new != "renamed" {
					t.Fatalf("rename paths = %q -> %q", mount.old, mount.new)
				}
				return
			}
			if err == nil {
				t.Fatal("rename outside the source device root succeeded")
			}
			if mount.old != "" || mount.new != "" {
				t.Fatal("source mutated after rejected rename")
			}
		})
	}
}
