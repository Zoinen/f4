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
