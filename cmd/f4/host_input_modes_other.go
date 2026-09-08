//go:build !windows

package main

// Only Windows has a console host that speaks to the terminal on its client's
// behalf. Everywhere else the sequences hostInputRestoreSeq returns are the
// whole request, and there is nothing to announce through a console mode.
func reannounceConsoleMouseMode() {}
