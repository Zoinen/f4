//go:build !windows

package fileops

import (
	"context"

	"github.com/unxed/f4/vfs"
)

// applyPlatformRights has nothing to add outside Windows: the permission bits
// destinationRights puts on a copy are all the access rights there are.
func applyPlatformRights(ctx context.Context, state *FileOpState, srcVfs vfs.VFS, srcPath string, dstVfs vfs.VFS, dstPath string) {
}
