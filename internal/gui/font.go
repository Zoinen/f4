package gui

import (
	"github.com/unxed/f4/internal/config"
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

func EffectiveGuiFont() string {
	font := resolveGuiFont(runtime.GOOS, config.App.GuiUseSystemMonospace, config.App.GuiFont)
	if p := windowsFontFile(font); p != "" {
		return p
	}
	return font
}
