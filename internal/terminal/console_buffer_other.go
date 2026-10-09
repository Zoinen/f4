//go:build !windows

package terminal

// Only the Windows console needs its screen buffer saved and put back: every
// other terminal restores itself from the alternate screen.

func CaptureHostConsoleBuffer(w, h int) {}
func RestoreHostConsoleBuffer()         {}

func HostConsoleBufferMatches(w, h int) bool { return false }
