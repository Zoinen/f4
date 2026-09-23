package netfox

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestNetFoxRootJoinKeepsURIAuthority(t *testing.T) {
	manager := NewNetFoxVFS(t.TempDir() + "/connections.json")
	if got, want := manager.Join(manager.GetPath(), "de_zoin"), "net://de_zoin"; got != want {
		t.Fatalf("manager Join = %q, want %q", got, want)
	}
	if got, want := manager.PanelTitle(manager.GetPath()), "Network"; got != want {
		t.Fatalf("manager PanelTitle = %q, want %q", got, want)
	}
}

func TestNetFoxBackendsPublishQualifiedPathsAndPanelInfoCache(t *testing.T) {
	paths := vfs.DevicePath{Scheme: "net", Device: "de zoin"}

	sftp := &SFTPVFS{path: "/home/user", title: "de zoin"}
	ftp := &FTPVFS{cwd: "/home/user", title: "de zoin"}
	for _, backend := range []struct {
		name string
		vfs  vfs.VFS
	}{
		{name: "sftp", vfs: sftp},
		{name: "ftp", vfs: ftp},
	} {
		t.Run(backend.name, func(t *testing.T) {
			configureNetFoxConnection(backend.vfs, "de zoin", backend.name, NetFoxConfig{
				Host: "example.test", Port: "22", User: "zoin",
			})
			if got, want := backend.vfs.GetPath(), paths.Public("/home/user"); got != want {
				t.Fatalf("GetPath = %q, want %q", got, want)
			}
			if got, want := backend.vfs.Join(backend.vfs.GetPath(), "file.txt"), paths.Public("/home/user/file.txt"); got != want {
				t.Fatalf("Join = %q, want %q", got, want)
			}
			provider, ok := backend.vfs.(vfs.PanelInfoProvider)
			if !ok {
				t.Fatalf("%T does not expose the panel-info cache provider", backend.vfs)
			}
			if provider.PanelInfoKey(vfs.PanelInfoRequest{Path: backend.vfs.GetPath()}) == "" {
				t.Fatal("PanelInfoKey is empty")
			}
			snapshot, fresh := provider.CachedPanelInfo(vfs.PanelInfoRequest{Path: backend.vfs.GetPath()})
			if !fresh || !snapshot.Authoritative || len(snapshot.Sections) != 1 {
				t.Fatalf("cached panel info = %#v, fresh=%t", snapshot, fresh)
			}
			if _, err := backend.vfs.Abs("net://another-host/home/user"); err == nil {
				t.Fatal("foreign connection URI was accepted")
			}
		})
	}
}

func TestNetURIProviderRestoresManagerRootWithoutNetworkIO(t *testing.T) {
	oldConfigDir := vfs.CustomConfigDir
	vfs.CustomConfigDir = t.TempDir()
	t.Cleanup(func() { vfs.CustomConfigDir = oldConfigDir })

	mounted, err := (&netURIProvider{}).OpenURI(context.Background(), nil, "net://")
	if err != nil {
		t.Fatal(err)
	}
	if got := mounted.GetPath(); got != "net://" {
		t.Fatalf("restored NetFox root = %q", got)
	}
}

func TestNetFoxManagerUsesSharedPanelInfoCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manager := NewNetFoxVFS(t.TempDir() + "/connections.json")
	if err := manager.SaveConfig("de_zoin", NetFoxConfig{
		Type: "sftp", Host: "example.test", Port: "22", User: "zoin",
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReadDir(context.Background(), manager.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	provider, ok := any(manager).(vfs.PanelInfoProvider)
	if !ok {
		t.Fatal("NetFox manager does not expose panel information")
	}
	req := vfs.PanelInfoRequest{Path: manager.GetPath(), SelectedName: "de_zoin"}
	snapshot, fresh := provider.CachedPanelInfo(req)
	if !fresh || !snapshot.Authoritative || len(snapshot.Sections) != 1 {
		t.Fatalf("manager cached panel info = %#v, fresh=%t", snapshot, fresh)
	}
	if got := snapshot.Sections[0].Fields[0].Value; got != "de_zoin" {
		t.Fatalf("cached connection = %q", got)
	}

	clone := manager.Clone().(*NetFoxVFS)
	cloneSnapshot, cloneFresh := clone.CachedPanelInfo(req)
	if !cloneFresh || cloneSnapshot.Sections[0].Fields[0].Value != "de_zoin" {
		t.Fatalf("clone did not reuse manager cache: %#v, fresh=%t", cloneSnapshot, cloneFresh)
	}
}
