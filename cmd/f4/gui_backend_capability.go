package main

import (
	"fmt"
	"strings"
)

func guiBackendRequiresFFI(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "ebiten", "gogpu", "wayland":
		return true
	default:
		return false
	}
}

func unavailableGUIBackendError(backend string, ffiAvailable bool) error {
	if !guiBackendRequiresFFI(backend) || ffiAvailable {
		return nil
	}
	return fmt.Errorf("GUI backend %q is unavailable in a static build without FFI; use --gui=x11 or --tty=ansi", backend)
}

func checkGUIBackendAvailability(backend string) error {
	return unavailableGUIBackendError(backend, ffiAvailableForGUI())
}
