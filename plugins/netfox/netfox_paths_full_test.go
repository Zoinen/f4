//go:build !lite

package netfox

import (
	"github.com/unxed/f4/vfs"
	"testing"
)

const netFoxURIRegistrationsForTest = 4

func TestNetFoxWindowsPathsArePublishedReadable(t *testing.T) {
	for _, backend := range []vfs.VFS{
		&SFTPVFS{path: "/C:/Users"},
		&FTPVFS{cwd: "/C:/Users"},
		&FishVFS{path: "/C:/Users"},
	} {
		configureNetFoxConnection(backend, "HC_SFTP", "sftp", NetFoxConfig{})
		const want = "net://HC_SFTP/C:/Users"
		if got := backend.GetPath(); got != want {
			t.Fatalf("%T GetPath = %q, want %q", backend, got, want)
		}
		if got, err := backend.Abs("net://HC_SFTP/C%3A/Users"); err != nil || got != want {
			t.Fatalf("%T legacy Abs = %q, %v", backend, got, err)
		}
		if got := backend.Join(want, "a #?@.txt"); got != want+"/a #?@.txt" {
			t.Fatalf("%T Join = %q", backend, got)
		}
		if got := backend.(vfs.PanelTitleProvider).PanelTitle(want); got != want {
			t.Fatalf("%T PanelTitle = %q", backend, got)
		}
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
