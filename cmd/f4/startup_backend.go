package main

import (
	"strings"
)

// StartupMode says which renderer family f4 starts in when nothing on the
// command line settles the question. It exists so that a user who always
// wants the same answer does not have to retype --gui or --tty on every run
// (issue #601).
type StartupMode int

const (
	// StartupModeAuto keeps the historical behavior: probe the environment
	// and pick a graphical window when one is available.
	StartupModeAuto StartupMode = iota
	// StartupModeTTY always starts in the terminal, even on a desktop where
	// automatic selection would have opened a window.
	StartupModeTTY
	// StartupModeGui always starts in a graphical window, including on the
	// platforms where automatic selection deliberately does not try one
	// (native Windows and Wine, see shouldTryGui).
	StartupModeGui
)

func (m StartupMode) String() string {
	switch m {
	case StartupModeTTY:
		return "tty"
	case StartupModeGui:
		return "gui"
	default:
		return "auto"
	}
}

// ParseStartupMode maps a settings.ini value to a mode. Anything unknown
// means "auto": a hand-edited or newer-version config must not be able to
// stop f4 from starting.
func ParseStartupMode(value string) StartupMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tty", "console", "terminal", "text":
		return StartupModeTTY
	case "gui", "graphics", "graphical", "window":
		return StartupModeGui
	default:
		return StartupModeAuto
	}
}

// startupAutoBackend is the spelling that means "decide at startup". It is
// accepted both in settings.ini and on the command line, where it is the way
// to override a configured backend back to automatic selection for one run.
const startupAutoBackend = "auto"

// startupGuiBackends and startupTTYBackends are the backends the settings
// dialog offers, in display order. They are deliberately not the full set
// vtui accepts: "qt" and the "ext:" family are external-UI integrations that
// are configured by hand, and normalizeStartupGuiBackend keeps letting them
// through.
var (
	startupGuiBackends = []string{"win32", "gogpu", "ebiten", "x11", "wayland"}
	startupTTYBackends = []string{"ansi", "winapi"}
)

// normalizeStartupGuiBackend canonicalizes a configured graphics backend
// name, returning "" for "use automatic selection". An unrecognized name also
// becomes "": a stale config naming a backend this build does not know must
// degrade to detection rather than to a failed start.
func normalizeStartupGuiBackend(value string) string {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if lower == "" || lower == startupAutoBackend {
		return ""
	}
	// External UI backends are passed through untouched; RunGui routes them
	// to RunExternalUIWithMapping, which owns their naming.
	if lower == "qt" || strings.HasPrefix(lower, "ext:") {
		return trimmed
	}
	switch lower {
	case "win32", "winapi", "gdi", "win32gui":
		return "win32"
	case "gogpu", "ebiten", "x11", "wayland":
		return lower
	}
	return ""
}

// normalizeStartupTTYBackend canonicalizes a configured console backend name.
// "win32" is the documented alias of "winapi"; both spellings are understood
// downstream, so f4 stores only the canonical one.
func normalizeStartupTTYBackend(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ansi":
		return "ansi"
	case "winapi", "win32":
		return "winapi"
	}
	return ""
}

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
		if strings.EqualFold(strings.TrimSpace(flagValue), startupAutoBackend) {
			return ""
		}
		return flagValue
	}
	return normalize(configured)
}

// startupModeChoices is the order the settings dialog lists modes in; index 0
// must stay the safe default so an out-of-range selection lands on "auto".
var startupModeChoices = []StartupMode{StartupModeAuto, StartupModeTTY, StartupModeGui}

// startupModeChoiceIndex is the inverse of startupModeChoices.
func startupModeChoiceIndex(mode StartupMode) int {
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
