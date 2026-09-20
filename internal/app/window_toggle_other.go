//go:build !windows

package app

func toggleDirectWindowsTerminalWindow() bool { return false }
