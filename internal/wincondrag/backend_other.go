//go:build !windows

package wincondrag

// Install does nothing off Windows: there is no Windows console to drag from.
func Install() {}
