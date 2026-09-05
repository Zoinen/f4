//go:build !windows

package main

// The Win32 Console overlay has no meaning outside Windows: everywhere else the
// console view is a VT stream and the ANSI emitter handles it.
func winConsoleOverlayAvailable() bool { return false }

func winDrawConsoleOverlay(ov consoleOverlayContent) {}

func winClearConsoleOverlay(n int) {}

func clearConsoleViewBackground(w, h int) {}
func isPlatformPTYUsable() bool           { return true }
