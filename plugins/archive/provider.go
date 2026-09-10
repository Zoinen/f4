package archive

import (
	"context"
	"os"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/zipper/archive"
)

type ArchiveProvider struct{}

func (p *ArchiveProvider) Name() string  { return "zipper/archive" }
func (p *ArchiveProvider) Priority() int { return 10 }

// PanelEnterAllowed keeps self-extracting archives out of the ordinary Enter
// and double-click path. They remain available through the explicit
// Ctrl+PgDn action, which is the deliberate archive-entry gesture.
func (p *ArchiveProvider) PanelEnterAllowed(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	osvfs, ok := parent.(*vfs.OSVFS)
	if !ok {
		return true
	}
	localPath, err := osvfs.Abs(path)
	if err != nil {
		return true
	}
	embedded, found, err := findEmbeddedArchive(localPath)
	return err != nil || !found || embedded.offset <= 0
}

func (p *ArchiveProvider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		localPath, _ := osvfs.Abs(path)
		if fi, err := os.Stat(localPath); err == nil {
			if fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0 {
				return false
			}
		}
	}
	name := path
	if parent != nil {
		if base := parent.Base(path); base != "" {
			name = base
		}
	}
	format := archive.DetectFormat(name)
	if format != "" {
		return true
	}
	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		localPath, err := osvfs.Abs(path)
		if err != nil {
			return false
		}
		_, found, err := findEmbeddedArchive(localPath)
		return err == nil && found
	}
	return false
}

func (p *ArchiveProvider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	v, err := NewArchiveVFSContext(ctx, parent, path)
	if err != nil {
		// Return an untyped nil so the interface itself is nil: a typed
		// (*ArchiveVFS)(nil) would still compare != nil to callers, and a
		// later Close() on it panics (nil dereference in v.mu.Lock()).
		return nil, err
	}
	return v, nil
}
