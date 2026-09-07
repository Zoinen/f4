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
	font := resolveGuiFont(runtime.GOOS, AppConfig.GuiUseSystemMonospace, AppConfig.GuiFont)
	if p := windowsFontFile(font); p != "" {
		return p
	}
	return font
}

func defaultGuiFontSize(goos string) int {
	if goos == "darwin" {
		return 17
	}
	return 16
}
