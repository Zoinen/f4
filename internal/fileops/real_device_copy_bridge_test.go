//go:build integration

package fileops

import (
	"context"

	"github.com/unxed/f4/vfs"
)

// CopyForRealDeviceIntegration exposes the transfer engine only to external
// integration tests. The RPC host imports fileops indirectly, so its harness
// must live in fileops_test to avoid an import cycle. No production API is added.
func CopyForRealDeviceIntegration(
	ctx context.Context,
	source vfs.VFS,
	sourcePath string,
	destination vfs.VFS,
	destinationPath string,
	state *FileOpState,
) error {
	return recursiveCopy(ctx, source, sourcePath, destination, destinationPath, state, 0)
}
