//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || solaris || illumos

package vtui

// registryFontFiles has nothing to read outside Windows: the font directories
// are the whole record there, and on the systems that keep a database of their
// own it is fontconfig, which gui_font_scripts.go already asks.
func registryFontFiles() []string { return nil }
