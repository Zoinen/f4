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

func (p *ArchiveProvider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		localPath, _ := osvfs.Abs(path)
		if fi, err := os.Stat(localPath); err == nil {
			if fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0 {
				return false
			}
		}
	}
	format := archive.DetectFormat(path)
	return format != ""
}

func (p *ArchiveProvider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	return NewArchiveVFS(parent, path)
}
