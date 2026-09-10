//go:build !(windows && (amd64 || arm64))

package gui

// Nowhere but a Windows build can be running under Wine, and the 32-bit
// Windows builds are excluded because the ntdll exports these wrap are
// cdecl, which syscall.LazyProc cannot call correctly on x86. Reporting
// false everywhere leaves each caller with its own handling, which is what
// it had before.

func HostUnixPath(dos string) (string, bool) { return "", false }

func HostDosPath(unix string) (string, bool) { return "", false }
