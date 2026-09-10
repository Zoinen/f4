package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/plughost"
	"runtime"
	"strings"
)

// startupGuiBackends and startupTTYBackends are the backends the settings
// dialog offers, in display order. They are deliberately not the full set
// vtui accepts: "qt" and the "ext:" family are external-UI integrations that
// are configured by hand, and config.NormalizeStartupGuiBackend keeps letting them
// through.
var (
	startupGuiBackends = []string{"win32", "gogpu", "ebiten", "x11", "wayland"}
	startupTTYBackends = []string{"ansi", "winapi"}
)

// resolveStartupBackend decides which backend name f4 should use for one
// renderer family.
//
// A value given on the command line always wins, and is passed through
// unchanged: a mistyped --gui=gogpuu should produce a visible error rather
// than silently starting something else. The explicit "auto" spelling is the
// exception, and is how a user overrides a configured default back to
// detection for a single run.
//
// A value that comes from settings.ini is normalized instead, because a
// config file is not a per-run instruction the user is watching: an unusable
// value there must fall back to automatic selection.
func resolveStartupBackend(flagValue string, flagGiven bool, configured string, normalize func(string) string) string {
	if flagGiven {
		if strings.EqualFold(strings.TrimSpace(flagValue), config.StartupAutoBackend) {
			return ""
		}
		return flagValue
	}
	return normalize(configured)
}

// startupModeChoices is the order the settings dialog lists modes in; index 0
// must stay the safe default so an out-of-range selection lands on "auto".
var startupModeChoices = []config.StartupMode{config.StartupModeAuto, config.StartupModeTTY, config.StartupModeGui}

// startupModeChoiceIndex is the inverse of startupModeChoices.
func startupModeChoiceIndex(mode config.StartupMode) int {
	for i, candidate := range startupModeChoices {
		if candidate == mode {
			return i
		}
	}
	return 0
}

// startupBackendChoices returns the combo values for a backend family: the
// empty string (shown as "Auto") followed by the known backend names.
func startupBackendChoices(names []string) []string {
	choices := make([]string, 0, len(names)+1)
	choices = append(choices, "")
	choices = append(choices, names...)
	return choices
}

// startupBackendChoiceIndex locates a stored backend in a choice list. An
// unknown value selects "Auto", matching what normalizeStartup*Backend would
// have done with it.
func startupBackendChoiceIndex(choices []string, current string) int {
	for i, choice := range choices {
		if choice == current {
			return i
		}
	}
	return 0
}

// startupChoiceAt reads a combo selection back, clamping to the safe default.
func startupChoiceAt[T any](choices []T, pos int) T {
	if pos < 0 || pos >= len(choices) {
		var zero T
		if len(choices) > 0 {
			return choices[0]
		}
		return zero
	}
	return choices[pos]
}

// Portable Windows builds default to their embedded Qt frontend. Explicit
// command-line and configured startup modes take precedence.
func portableQtDefault() bool { return runtime.GOOS == "windows" && plughost.HasEmbeddedQtHost() }
