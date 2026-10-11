//go:build !darwin

package gui

// EnsureDarwinIcon is a no-op away from macOS: Windows carries the icon as a
// resource in the executable, and on X11/Wayland it comes from the .desktop
// file shipped in packaging/linux.
func EnsureDarwinIcon() {}
