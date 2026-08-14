//go:build linux || openbsd || netbsd || dragonfly || freebsd || illumos || solaris

package main

// applyDarwinDockIcon is a no-op away from macOS; on X11/Wayland the window
// icon comes from the .desktop file shipped in packaging/linux.
func applyDarwinDockIcon(string) {}
