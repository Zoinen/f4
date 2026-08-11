//go:build !windows

package main

// installConsoleCtrlHandler is a no-op on non-Windows platforms.
func installConsoleCtrlHandler() {}
