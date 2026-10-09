package vfs

import "strings"

// win32HiddenAttr is FILE_ATTRIBUTE_HIDDEN. It is spelled out here because
// syscall defines the constant only on Windows and this file builds everywhere.
const win32HiddenAttr = 0x2

// hiddenByRule is the one rule for "does the panel dim this entry", with the
// host personality passed in so both sides can be tested without Wine
// (hostmode.Posix() is a sync.Once and cannot be flipped from a test).
//
// In the posix personality only the leading dot counts -- byte for byte what
// hidden_unix.go does -- because there is no FILE_ATTRIBUTE_HIDDEN on the far
// side of libwinescape. Reaching that answer used to depend on a type
// assertion failing (Sys() is a *winescape.Stat_t there, not the Win32 data),
// which is the trap that fillPhysicalSizeCheap fell into; here it is explicit.
//
// On Windows the hidden attribute wins, and the dot prefix stays as the
// cross-platform fallback when the attributes are unavailable or clear.
func hiddenByRule(name string, winAttrs uint32, haveWinAttrs, posix bool) bool {
	if !posix && haveWinAttrs && winAttrs&win32HiddenAttr != 0 {
		return true
	}
	return strings.HasPrefix(name, ".")
}
