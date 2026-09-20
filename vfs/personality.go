package vfs

import (
	"runtime"

	"github.com/unxed/f4/vfs/hostmode"
)

// WindowsPersonality reports whether the host filesystem layer behaves like
// Windows: a Windows build that is not running in the posix personality under
// Wine (WINE.md §13, Part E).
//
// In the posix personality the paths, permissions, ownership and case rules are
// Linux's, so every place that used to ask "is this GOOS windows" to decide
// about drive letters, junctions, Win32 attributes, ".exe" or case folding asks
// this instead. Off Windows it is false without consulting hostmode.
func WindowsPersonality() bool {
	return runtime.GOOS == "windows" && !hostmode.Posix()
}
