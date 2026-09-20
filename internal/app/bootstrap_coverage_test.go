package app

import (
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

func TestShouldTryGuiHeadlessEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" || vtui.IsWine() {
		t.Skip("the console-first platform path is covered separately")
	}

	oldProbeTTY := terminal.ProbeHostTTY
	t.Cleanup(func() { terminal.ProbeHostTTY = oldProbeTTY })
	terminal.ProbeHostTTY = func() bool { return false }
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	if runtime.GOOS == "darwin" {
		if !shouldTryGui() {
			t.Fatal("macOS desktop startup must try GUI without a display variable")
		}
		return
	}
	if shouldTryGui() {
		t.Fatal("headless Unix startup must stay in console mode")
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if !shouldTryGui() {
		t.Fatal("a Wayland desktop startup must try GUI")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	if !shouldTryGui() {
		t.Fatal("an X11 desktop startup must try GUI")
	}
}

func TestTryRunDefaultGuiWithoutEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" || vtui.IsWine() {
		t.Skip("this test covers the headless Unix auto-detection path")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	err := tryRunDefaultGui()
	if err == nil || !strings.Contains(err.Error(), "no suitable GUI environment detected") {
		t.Fatalf("tryRunDefaultGui() error = %v, want a headless-environment error", err)
	}
}
