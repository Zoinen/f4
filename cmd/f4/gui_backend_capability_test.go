package main

import (
	"strings"
	"testing"
)

func TestGUIBackendRequiresFFI(t *testing.T) {
	tests := []struct {
		backend string
		want    bool
	}{
		{backend: "gogpu", want: true},
		{backend: "wayland", want: true},
		{backend: " ebiten ", want: true},
		{backend: "x11", want: false},
		{backend: "ansi", want: false},
		{backend: "", want: false},
	}

	for _, tt := range tests {
		if got := guiBackendRequiresFFI(tt.backend); got != tt.want {
			t.Errorf("guiBackendRequiresFFI(%q) = %v, want %v", tt.backend, got, tt.want)
		}
	}
}

func TestUnavailableGUIBackendError(t *testing.T) {
	if err := unavailableGUIBackendError("gogpu", false); err == nil ||
		!strings.Contains(err.Error(), "static build without FFI") ||
		!strings.Contains(err.Error(), "--gui=x11") {
		t.Fatalf("unavailable gogpu error = %v, want actionable static-build guidance", err)
	}

	for _, tt := range []struct {
		backend   string
		available bool
	}{
		{backend: "gogpu", available: true},
		{backend: "wayland", available: true},
		{backend: "x11", available: false},
		{backend: "ansi", available: false},
	} {
		if err := unavailableGUIBackendError(tt.backend, tt.available); err != nil {
			t.Errorf("unavailableGUIBackendError(%q, %v) = %v, want nil", tt.backend, tt.available, err)
		}
	}
}
