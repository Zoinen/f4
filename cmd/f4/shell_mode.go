package main

import (
	"os"
	"runtime"
	"strings"

	"github.com/unxed/vtui"
	"golang.org/x/term"
)

// ShellMode defines how shell commands and console interactions are executed.
type ShellMode int

const (
	// ShellModeOwn uses the internal terminal emulator (PTY -> parser -> grid -> vtui).
	ShellModeOwn ShellMode = iota
	// ShellModeHost uses host terminal passthrough with internal mirror.
	ShellModeHost
	// ShellModeSimpleInline runs commands via Suspend/exec/Resume directly in host console.
	ShellModeSimpleInline
	// ShellModeSimpleCaptured runs commands with captured output in an f4 dialog.
	ShellModeSimpleCaptured
)

func (m ShellMode) String() string {
	switch m {
	case ShellModeHost:
		return "host"
	case ShellModeSimpleInline:
		return "simple-inline"
	case ShellModeSimpleCaptured:
		return "simple-captured"
	default:
		return "own"
	}
}

// ShellModeConfig carries user configuration for shell mode selection.
type ShellModeConfig struct {
	ConsoleMode      string // "own" | "host"
	ConsoleOverlayUI bool
}

// Environment probe functions, customizable for testing.
var (
	probeGUIBackend = func() string { return vtui.ActiveBackend() }
	probeHostTTY    = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
	probePTYUsable  = func() bool { return isPlatformPTYUsable() }
	probeGOOS       = func() string { return runtime.GOOS }
)

// resolveShellMode calculates the effective shell execution mode based on
// environment capabilities and user preference according to CONSOLE_MODES.md §4.1.
func resolveShellMode(cfg ShellModeConfig) ShellMode {
	if !probePTYUsable() {
		if probeHostTTY() && probeGOOS() == "windows" {
			return ShellModeSimpleInline
		}
		return ShellModeSimpleCaptured
	}
	if consoleViewStyleOf(cfg) == ConsoleViewOwn {
		return ShellModeOwn
	}
	if probeGUIBackend() != "" {
		return ShellModeOwn
	}
	if !probeHostTTY() {
		return ShellModeOwn
	}
	return ShellModeHost
}

// Console view styles for the Ctrl+O screen. This is a single user choice of
// three: ConsoleViewOwn needs a PTY, the other two work with and without one.
const (
	ConsoleViewOwn = "own"
	ConsoleViewFar = "far"
	ConsoleViewMc  = "mc"
)

// consoleViewStyleOf resolves the configured console view. It accepts both the
// current three-way ConsoleMode and the older ConsoleMode+ConsoleOverlayUI pair,
// so configs written by earlier builds keep working untouched.
func consoleViewStyleOf(cfg ShellModeConfig) string {
	switch strings.ToLower(cfg.ConsoleMode) {
	case ConsoleViewFar:
		return ConsoleViewFar
	case ConsoleViewMc:
		return ConsoleViewMc
	case "host":
		if cfg.ConsoleOverlayUI {
			return ConsoleViewFar
		}
		return ConsoleViewMc
	}
	return ConsoleViewOwn
}

// consoleViewStyle returns the console view configured for this instance.
func consoleViewStyle() string {
	return consoleViewStyleOf(ShellModeConfig{
		ConsoleMode:      AppConfig.ConsoleMode,
		ConsoleOverlayUI: AppConfig.ConsoleOverlayUI,
	})
}

// consoleViewStyleFor adapts the configured style to an already resolved shell
// mode. "Own terminal" is meaningless where no PTY could be allocated (Wine,
// pre-ConPTY Windows), so it degrades to the Far style instead of leaving the
// user with a blank screen after Ctrl+O.
func consoleViewStyleFor(mode ShellMode) string {
	style := consoleViewStyle()
	if style == ConsoleViewOwn && (mode == ShellModeSimpleInline || mode == ShellModeSimpleCaptured) {
		return ConsoleViewFar
	}
	return style
}
