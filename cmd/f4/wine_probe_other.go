//go:build !windows

package main

// probeConsole has nothing to report outside Windows: there is no console
// screen buffer API, and the size already comes from term.GetSize.
func probeConsole() consoleProbe { return consoleProbe{} }
