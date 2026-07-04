//go:build darwin

package vtui

import (
	"os"
	"syscall"
)

func RedirectStderr(f *os.File) error {
	return syscall.Dup2(int(f.Fd()), 2)
}

func countOpenFDs() int {
	// macOS doesn't have /proc/self/fd, would need lsof or proc_pidinfo
	return -1
}
