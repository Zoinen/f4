//go:build !windows

package main

// readHostConsoleFullText has nothing to read outside the Windows Console
// API path -- non-Windows platforms only reach ShellModeSimpleInline's
// no-PTY console view via the ANSI backend, which has no equivalent
// full-buffer readback (WINE.md §4 Etap A2 never grew one; ANSI consoles
// don't expose their scrollback for query). Callers fall back to
// pf.termView, same as before this file existed.
func readHostConsoleFullText(overlayLines int) []byte {
	return nil
}
