//go:build linux || darwin || openbsd || netbsd || dragonfly || freebsd || illumos || solaris

package main

import (
	"fmt"
	"strings"

	"github.com/unxed/vtui"
)

func runGuiWithRecovery(backend string, run func() error) (err error) {
	// A display backend is an optional integration and may panic while
	// initializing (for example, a malformed Wayland cursor theme).  Treat
	// that as a backend failure so automatic GUI selection can try another
	// available backend instead of terminating f4.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%s GUI backend panicked during startup: %v", backend, recovered)
		}
	}()
	return run()
}

func RunGui(backend string) error {
	return runGuiWithRecovery(backend, func() error {
		if backend == "qt" || strings.HasPrefix(backend, "ext:") {
			return RunExternalUIWithMapping(backend)
		}
		applyDarwinDockIcon(backend)
		return vtui.RunInGUIWindow(AppConfig.GuiCols, AppConfig.GuiRows, backend, effectiveGuiFont(), float64(AppConfig.GuiFontSize), func() {
			SetupUI()
			openDashEFileIfRequested()
		})
	})
}
