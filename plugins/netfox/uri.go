package netfox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
)

// netURIProvider restores a saved NetFox connection from the same qualified
// address that panels, command prompts and folder history publish. It keeps a
// history entry useful after the user has left the connection or restarted f4.
type netURIProvider struct{}

func (*netURIProvider) Scheme() string { return "net" }

func netFoxConnectionsPath() string {
	cfgDir := vfs.CustomConfigDir
	if cfgDir == "" {
		sysDir, _ := os.UserConfigDir()
		cfgDir = filepath.Join(sysDir, "f4")
	}
	return filepath.Join(cfgDir, "NetFox.json")
}

func (p *netURIProvider) OpenURI(ctx context.Context, current vfs.VFS, raw string) (vfs.VFS, error) {
	if strings.EqualFold(raw, "net://") {
		return &netFoxVFSWrapper{NewNetFoxVFS(netFoxConnectionsPath())}, nil
	}
	scheme, connection, remote, err := vfs.ParseDevicePath(raw)
	if err != nil {
		return nil, fmt.Errorf("netfox: parse URI: %w", err)
	}
	if !strings.EqualFold(scheme, "net") {
		return nil, fmt.Errorf("netfox: foreign URI scheme %q", scheme)
	}

	manager := &netFoxVFSWrapper{NewNetFoxVFS(netFoxConnectionsPath())}
	if source, ok := current.(*netFoxVFSWrapper); ok && source.NetFoxVFS != nil {
		manager = &netFoxVFSWrapper{NewNetFoxVFS(source.path)}
	}
	target := manager.Join(manager.GetPath(), connection)
	provider := vfs.FindProvider(ctx, manager, target)
	if provider == nil {
		return nil, fmt.Errorf("netfox: connection %q: %w", connection, os.ErrNotExist)
	}
	mounted, err := provider.Open(ctx, manager, target)
	if err != nil {
		return nil, err
	}

	item, statErr := mounted.Stat(ctx, remote)
	if statErr == nil && !item.IsDir {
		remote = mounted.Dir(remote)
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		_ = mounted.Close()
		return nil, statErr
	}
	if err := mounted.SetPath(remote); err != nil {
		_ = mounted.Close()
		return nil, err
	}
	return mounted, nil
}

var _ vfs.URIProvider = (*netURIProvider)(nil)
