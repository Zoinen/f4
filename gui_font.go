package main

import (
	"runtime"
	"strings"
)

// resolveGuiFont keeps Linux on its existing configured/backend default while
// selecting the platform's established monospace family on Windows and macOS.
// The explicit OS argument keeps the policy testable on every build host.
func resolveGuiFont(goos string, useSystem bool, configured string) string {
	configured = strings.TrimSpace(configured)
	if !useSystem {
		return configured
	}
	switch goos {
	case "windows":
		return "Consolas"
	case "darwin":
		return "Monaco"
	default:
		return configured
	}
}

func effectiveGuiFont() string {
	return resolveGuiFont(runtime.GOOS, AppConfig.GuiUseSystemMonospace, AppConfig.GuiFont)
}

func defaultGuiFontSize(goos string) int {
	if goos == "darwin" {
		return 17
	}
	return 16
}
