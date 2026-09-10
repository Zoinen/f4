//go:build !windows

package terminal

// The Win32 Console overlay has no meaning outside Windows: everywhere else the
// console view is a VT stream and the ANSI emitter handles it.
func WinConsoleOverlayAvailable() bool { return false }

func WinDrawConsoleOverlay(ov ConsoleOverlayContent) {}

func WinClearConsoleOverlay(n int) {}

func ClearConsoleViewBackground(w, h int) {}
func isPlatformPTYUsable() bool           { return true }
