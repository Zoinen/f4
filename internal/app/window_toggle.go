package app

const windowsTerminalHostingWindowClass = "CASCADIA_HOSTING_WINDOW_CLASS"

// shouldToggleDirectWindowsTerminalWindow limits the foreground-window
// fallback to the one host window class that Windows Terminal uses for the
// default-terminal handoff. A foreground window is only useful while f4 is
// in a terminal; GUI renderers keep handling Alt+F9 themselves.
func shouldToggleDirectWindowsTerminalWindow(activeBackend, class string) bool {
	return activeBackend == "" && class == windowsTerminalHostingWindowClass
}
