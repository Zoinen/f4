package terminal

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
	"golang.org/x/term"
	"os"
	"runtime"
	"strings"
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
	ProbeGUIBackend = func() string { return vtui.ActiveBackend() }
	ProbeHostTTY    = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
	ProbePTYUsable  = func() bool { return isPlatformPTYUsable() }
	probeGOOS       = func() string { return runtime.GOOS }
)

// ResolveShellMode calculates the effective shell execution mode based on
// environment capabilities and user preference according to CONSOLE_MODES.md §4.1.
func ResolveShellMode(cfg ShellModeConfig) ShellMode {
	if !ProbePTYUsable() {
		if ProbeHostTTY() && probeGOOS() == "windows" {
			return ShellModeSimpleInline
		}
		return ShellModeSimpleCaptured
	}
	if ConsoleViewStyleOf(cfg) == ConsoleViewOwn {
		return ShellModeOwn
	}
	if ProbeGUIBackend() != "" {
		return ShellModeOwn
	}
	if !ProbeHostTTY() {
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

// ConsoleViewStyleOf resolves the configured console view. It accepts both the
// current three-way ConsoleMode and the older ConsoleMode+ConsoleOverlayUI pair,
// so configs written by earlier builds keep working untouched.
func ConsoleViewStyleOf(cfg ShellModeConfig) string {
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
	return ConsoleViewStyleOf(ShellModeConfig{
		ConsoleMode:      config.App.ConsoleMode,
		ConsoleOverlayUI: config.App.ConsoleOverlayUI,
	})
}

// ConsoleViewStyleFor adapts the configured style to an already resolved shell
// mode. "Own terminal" is meaningless where no PTY could be allocated (Wine,
// pre-ConPTY Windows), so it degrades to the Far style instead of leaving the
// user with a blank screen after Ctrl+O.
func ConsoleViewStyleFor(mode ShellMode) string {
	style := consoleViewStyle()
	if style == ConsoleViewOwn && (mode == ShellModeSimpleInline || mode == ShellModeSimpleCaptured) {
		return ConsoleViewFar
	}
	return style
}
