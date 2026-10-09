//go:build windows

package vfs

import (
	"os"
	"syscall"

	"github.com/unxed/f4/vfs/hostmode"
)

func isHidden(path string, name string, info os.FileInfo) bool {
	var attrs uint32
	var have bool
	if info != nil {
		if stat, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
			attrs, have = stat.FileAttributes, true
		}
	}
	return hiddenByRule(name, attrs, have, hostmode.Posix())
}
