//go:build windows

package vfs

import (
	"os"
	"syscall"
)

func isHidden(path string, name string, info os.FileInfo) bool {
	var attrs uint32
	if info != nil {
		if stat, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
			attrs = stat.FileAttributes
		}
	}
	if attrs&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
		return true
	}
	// A leading dot has no hidden-file semantics on Windows. The POSIX
	// personality uses a different host implementation and does not reach
	// this Windows attribute path.
	return false
}
