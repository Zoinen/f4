//go:build windows

package main

import "golang.org/x/sys/windows"

func processEnvironmentProcessState(pid int) (alive bool, known bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// OpenProcess documents INVALID_PARAMETER for a PID that does not
		// exist. Access denied is intentionally treated as unknown.
		if err == windows.ERROR_INVALID_PARAMETER {
			return false, true
		}
		return false, false
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false, false
	}
	return exitCode == 259, true // STILL_ACTIVE
}
