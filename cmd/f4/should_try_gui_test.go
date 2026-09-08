package main

import (
	"github.com/unxed/vtui"
	"runtime"
	"testing"
)

func TestShouldTryGui_WineDefaultsToConsole(t *testing.T) {
	if vtui.IsWine() {
		if shouldTryGui() {
			t.Error("shouldTryGui() under Wine must return false by default")
		}
	}
}

func TestShouldTryGui_WindowsDefaultsToConsole(t *testing.T) {
	if runtime.GOOS == "windows" && !vtui.IsWine() {
		if shouldTryGui() {
			t.Error("shouldTryGui() on native Windows must return false by default")
		}
	}
}

func TestShouldTryGui_TTYTakesPrecedenceOverDisplay(t *testing.T) {
	oldProbeTTY := probeHostTTY
	t.Cleanup(func() { probeHostTTY = oldProbeTTY })
	probeHostTTY = func() bool { return true }
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	if shouldTryGui() {
		t.Error("shouldTryGui() must keep a terminal launch in console mode when DISPLAY is set")
	}
}
