//go:build windows && !nocrashreport

package vtui

import "syscall"

// stillActive is STILL_ACTIVE, the exit code Windows reports for a process
// that has not exited yet.
const stillActive = 259

// processAlive reports whether a process with this pid currently exists.
// Anything uncertain counts as alive: the caller deletes files based on this
// answer, so a false negative is the expensive direction.
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}
