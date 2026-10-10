//go:build !windows

package vtui

// toggleConsoleMaximizedOS reports false: outside Windows a terminal's
// window belongs to the terminal emulator, not to the console API.
func toggleConsoleMaximizedOS() bool { return false }
