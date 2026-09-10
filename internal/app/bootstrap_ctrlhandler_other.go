//go:build !windows

package app

// installConsoleCtrlHandler is a no-op on non-Windows platforms.
func installConsoleCtrlHandler() {}
