//go:build !windows

package terminal

// ProbeConsole has nothing to report outside Windows: there is no console
// screen buffer API, and the size already comes from term.GetSize.
func ProbeConsole() consoleProbe { return consoleProbe{} }
