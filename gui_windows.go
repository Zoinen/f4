//go:build windows

package main

import (
	"strings"

	"github.com/unxed/vtui"
)

func RunGui(backend string) error {
	if backend == "qt" || strings.HasPrefix(backend, "ext:") {
		return RunExternalUIWithMapping(backend)
	}
	stopIconManager := startWindowsWindowIconManager()
	defer stopIconManager()
	return vtui.RunInGUIWindow(AppConfig.GuiCols, AppConfig.GuiRows, backend, AppConfig.GuiFont, float64(AppConfig.GuiFontSize), func() {
		SetupUI()
	})
}
