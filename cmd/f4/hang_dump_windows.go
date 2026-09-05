//go:build windows

package main

// installHangDumpHandler is a no-op on Windows: there is no SIGUSR1
// equivalent wired up here yet. See hang_dump_unix.go and
// PORTABILITY_BSD.md, 4.8.
func installHangDumpHandler() {}
