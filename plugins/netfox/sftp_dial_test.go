package netfox

import (
	"context"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"path/filepath"
	"testing"
)

// TestNetFoxProvidersFailedDialReturnPlainNil covers the stored-site paths used
// by the panel for SFTP, FISH+, and FTP. Their constructors return nil concrete
// VFS pointers when dialing fails; providers must not wrap those pointers in a
// non-nil vfs.VFS interface, because the asynchronous opener closes every
// non-nil result while reporting err.
func TestNetFoxProvidersFailedDialReturnPlainNil(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		provider vfs.VFSProvider
	}{
		{name: "sftp", typeName: "sftp", provider: &sftpProvider{}},
		{name: "fish+", typeName: "fish+", provider: &fishProvider{}},
		{name: "ftp", typeName: "ftp", provider: &ftpProvider{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
			if err := manager.SaveConfig("unreachable", NetFoxConfig{
				Type:      tt.typeName,
				Host:      "127.0.0.1",
				Port:      deadTCPPort(t),
				User:      "nobody",
				Pass:      "",
				Timeout:   "1",
				ProxyMode: netproxy.ModeDirect,
			}); err != nil {
				t.Fatal(err)
			}

			parent := &netFoxVFSWrapper{NetFoxVFS: manager}
			opened, err := tt.provider.Open(context.Background(), parent, "unreachable")
			if err == nil {
				t.Errorf("opening an unreachable %s site succeeded", tt.name)
				if opened != nil {
					_ = opened.Close()
				}
				return
			}
			if opened != nil {
				t.Errorf("failed %s open returned non-nil file system %T", tt.name, opened)
				// This is the cleanup performed by the asynchronous panel opener. A
				// typed nil reaches this call while the opener is reporting err.
				_ = opened.Close()
			}
		})
	}
}
