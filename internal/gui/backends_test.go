//go:build !lite && !tty_only

package gui

import "testing"

// The regular build carries every backend, so nothing is refused for being
// absent; only the FFI preflight can still say no.
func TestRegularBuildCarriesEveryBackend(t *testing.T) {
	for _, backend := range []string{"x11", "wayland", "win32", "gogpu", "ebiten", "qt", "ext:foo"} {
		if !BackendBuilt(backend) {
			t.Errorf("BackendBuilt(%q) = false in the regular build", backend)
		}
	}
}
