//go:build lite && !tty_only

package gui

import "strings"

const Available = true

// BackendBuilt reports whether this binary carries the named GUI backend.
//
// The lite build keeps the backends that draw with nothing but the display
// server's own protocol: X11 and Wayland, Win32 GDI on Windows, and the
// native Cocoa window on macOS. It is built with vtui's vtui_noebiten and
// vtui_nogogpu tags, which leave Ebitengine and gogpu, and the GPU stack
// behind them, out of the binary (f4#1178); Cocoa carries no such tag, so it
// stays in and becomes the lite build's macOS default (f4#1571). liteguard.go
// refuses a lite build without the two tags above. External UI plugins (qt,
// ext:) run as their own processes and link nothing here, so they stay
// available.
func BackendBuilt(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "gogpu", "ebiten":
		return false
	}
	return true
}
