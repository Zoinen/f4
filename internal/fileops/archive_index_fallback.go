//go:build dragonfly || netbsd || solaris || illumos || lite

package fileops

import (
	"context"

	"github.com/unxed/f4/vfs"
)

// These stand in for archive_index.go. The lite build takes them too: it
// has no archive plugin to write tar index sidecars, so there is nothing to
// move or delete along with an archive, and it must not link
// github.com/unxed/tar just for tar.GetStandardIndexPath (f4#1178).

func handleArchiveIndexOp(srcVfs vfs.VFS, oldPath string, dstVfs vfs.VFS, newPath string, isMove bool) {
}

func handleArchiveIndexDelete(ctx context.Context, v vfs.VFS, p string) {}

func collectArchiveIndexes(ctx context.Context, v vfs.VFS, p string) []string { return nil }

func removeArchiveIndexes(indexes []string) {}
