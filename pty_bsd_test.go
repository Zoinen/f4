//go:build freebsd || dragonfly

package main

import (
	"testing"
)

func TestPTYSlaveName_InvalidFd(t *testing.T) {
	_, err := ptySlaveName(-1)
	if err == nil {
		t.Error("expected error for invalid master fd, got nil")
	}
}
