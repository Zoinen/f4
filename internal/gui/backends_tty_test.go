//go:build tty_only

package gui

import (
	"strings"
	"testing"
)

func TestTTYOnlyBuildHasNoGUIBackend(t *testing.T) {
	for _, backend := range []string{"x11", "wayland", "gogpu", "ebiten", "qt", "ext:example"} {
		if BackendBuilt(backend) {
			t.Errorf("BackendBuilt(%q) = true in TTY-only build", backend)
		}
	}
}

func TestTTYOnlyBuildRejectsGUIStartup(t *testing.T) {
	err := RunGui("x11", func() { t.Fatal("GUI setup ran") })
	if err == nil || !strings.Contains(err.Error(), "TTY-only build") {
		t.Fatalf("RunGui error = %v, want TTY-only build error", err)
	}
}
