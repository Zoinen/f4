//go:build !windows && !nocrashreport

package vtui

import "syscall"

// processAlive reports whether a process with this pid currently exists.
// Signal 0 runs the existence and permission checks without delivering
// anything; EPERM means the process is there, just owned by someone else.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
