package app

import (
	"github.com/unxed/f4/internal/terminal"
	"runtime"
	"testing"
)

// Wine starts the way Windows does (issue #474): f4.exe in the console,
// f4-gui.exe or --gui in a window. There is no separate Wine rule to test;
// under Wine this test runs as the Windows one.
func TestShouldTryGui_WindowsDefaultsToConsole(t *testing.T) {
	if runtime.GOOS == "windows" {
		if shouldTryGui() {
			t.Error("shouldTryGui() on Windows, native or under Wine, must return false by default")
		}
	}
}

func TestShouldTryGui_TTYTakesPrecedenceOverDisplay(t *testing.T) {
	oldProbeTTY := terminal.ProbeHostTTY
	t.Cleanup(func() { terminal.ProbeHostTTY = oldProbeTTY })
	terminal.ProbeHostTTY = func() bool { return true }
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	if shouldTryGui() {
		t.Error("shouldTryGui() must keep a terminal launch in console mode when DISPLAY is set")
	}
}
