//go:build lite && !tty_only

package gui

import (
	"strings"
	"testing"
)

// The lite build draws with X11, Wayland or Win32 only (f4#1178), and says
// so when asked for one of the backends it leaves out.
func TestLiteBuildCarriesOnlyDisplayServerBackends(t *testing.T) {
	for _, backend := range []string{"x11", "wayland", "win32", "qt", "ext:foo"} {
		if !BackendBuilt(backend) {
			t.Errorf("BackendBuilt(%q) = false in the lite build", backend)
		}
	}
	for _, backend := range []string{"gogpu", "ebiten", " GoGPU "} {
		if BackendBuilt(backend) {
			t.Errorf("BackendBuilt(%q) = true in the lite build", backend)
		}
		err := checkGUIBackendAvailability(backend)
		if err == nil || !strings.Contains(err.Error(), "not built into the lite build") {
			t.Errorf("checkGUIBackendAvailability(%q) = %v, want a not-built error", backend, err)
		}
	}
}
