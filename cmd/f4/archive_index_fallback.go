//go:build dragonfly || netbsd || solaris || illumos

package main

import (
	"context"

	"github.com/unxed/f4/vfs"
)

func handleArchiveIndexOp(srcVfs vfs.VFS, oldPath string, dstVfs vfs.VFS, newPath string, isMove bool) {
}

func handleArchiveIndexDelete(ctx context.Context, v vfs.VFS, p string) {}

func collectArchiveIndexes(ctx context.Context, v vfs.VFS, p string) []string { return nil }

func removeArchiveIndexes(indexes []string) {}
