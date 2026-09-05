//go:build linux || darwin || openbsd || netbsd || dragonfly || freebsd || illumos || solaris

package main

import (
	"strings"
	"sync/atomic"
	"testing"
)

func TestRunGuiConvertsStartupPanicToError(t *testing.T) {
	var startupComplete atomic.Bool
	err := runGuiWithStartupRecovery("wayland", &startupComplete, func() error {
		panic("cursor initialization failed")
	})

	if err == nil || !strings.Contains(err.Error(), "wayland GUI backend panicked") {
		t.Fatalf("panic was not converted to a useful backend error: %v", err)
	}
}

func TestRunGuiPreservesPanicAfterStartup(t *testing.T) {
	var startupComplete atomic.Bool
	startupComplete.Store(true)

	defer func() {
		if recovered := recover(); recovered != "render failed" {
			t.Fatalf("recovered panic = %v, want %q", recovered, "render failed")
		}
	}()

	_ = runGuiWithStartupRecovery("wayland", &startupComplete, func() error {
		panic("render failed")
	})
	t.Fatal("runtime panic was unexpectedly suppressed")
}
