//go:build linux || darwin || openbsd || netbsd || dragonfly || freebsd || illumos || solaris

package main

import (
	"strings"

	"github.com/unxed/vtui"
)

func RunGui(backend string) error {
	if backend == "qt" || strings.HasPrefix(backend, "ext:") {
		return RunExternalUIWithMapping(backend)
	}
	return vtui.RunInGUIWindow(AppConfig.GuiCols, AppConfig.GuiRows, backend, effectiveGuiFont(), float64(AppConfig.GuiFontSize), func() {
		SetupUI()
	})
}
