package dotnet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/dotnet"
	"github.com/unxed/f4/vfs"
)

// provider mounts an assembly the way an archive is mounted, but only on an
// explicit Ctrl+PgDn: Enter on a .exe must keep running it and on a .dll keep
// doing what it did.
type provider struct{}

func (*provider) Name() string  { return "dotnet" }
func (*provider) Priority() int { return 15 }

// PanelEnterAllowed keeps Enter and double-click for their usual meaning.
func (*provider) PanelEnterAllowed(context.Context, vfs.VFS, string) bool { return false }

func assemblyFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".dll", ".exe", ".netmodule", ".winmd":
		return true
	}
	return false
}

// localAssembly answers for a regular file on the local disk with a name an
// assembly has; the metadata itself is checked by open.
func localAssembly(ctx context.Context, parent vfs.VFS, path string) (string, bool) {
	if ctx != nil && ctx.Err() != nil {
		return "", false
	}
	local, ok := parent.(*vfs.OSVFS)
	if !ok || local == nil || !assemblyFile(path) {
		return "", false
	}
	abs, err := local.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return abs, true
}

func readAssembly(path string) (*dotnet.Info, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the file the user chose in the panel
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return dotnet.Read(f, st.Size())
}

func (p *provider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	abs, ok := localAssembly(ctx, parent, path)
	if !ok {
		return false
	}
	_, err := readAssembly(abs)
	return err == nil
}

func (p *provider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	abs, ok := localAssembly(ctx, parent, path)
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, dotnet.ErrNotAssembly)
	}
	info, err := readAssembly(abs)
	if err != nil {
		if errors.Is(err, dotnet.ErrNotAssembly) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return nil, err
	}
	return newAssemblyVFSIn(parent, filepath.Base(abs), info, filepath.Dir(abs), []string{abs}), nil
}
