//go:build windows

package vfs

import (
	"os"
	"syscall"
)

func isHidden(path string, name string, info os.FileInfo) bool {
	if info != nil {
		if stat, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
			if stat.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
				return true
			}
		}
	}
	// A leading dot has no hidden-file semantics on Windows. Treating it like
	// Unix made ordinary entries such as .gitignore render with the Gallery's
	// 50% hidden-entry opacity even though Explorer considers them visible.
	return false
}
