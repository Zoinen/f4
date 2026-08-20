//go:build linux || darwin || openbsd || netbsd || dragonfly || freebsd || illumos || solaris

package main

import (
	"strings"
	"testing"
)

func TestRunGuiConvertsBackendPanicToError(t *testing.T) {
	err := runGuiWithRecovery("wayland", func() error {
		panic("cursor initialization failed")
	})

	if err == nil || !strings.Contains(err.Error(), "wayland GUI backend panicked") {
		t.Fatalf("panic was not converted to a useful backend error: %v", err)
	}
}
