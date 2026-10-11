package vtui

import (
	"runtime"
	"strconv"
	"strings"
)

// KittyGraphicsTerminal reports whether the environment names a terminal that
// implements the kitty graphics protocol. It is knowledge about terminals, so it
// lives here rather than in each application: an application decides what to do
// with the answer (for instance prefer kitty over sixel where the terminal's
// sixel decoder recolours palette entries in place), vtui says what the
// terminal is. env is os.Getenv or a stand-in.
//
// Kitty, Ghostty and WezTerm are named first so that a stronger marker wins
// over a weaker one inherited into a nested session (WT_SESSION, say).
func KittyGraphicsTerminal(env func(string) string) bool {
	if env == nil {
		return false
	}

	term := strings.ToLower(env("TERM"))
	prog := strings.ToLower(env("TERM_PROGRAM"))

	if env("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty") {
		return true
	}
	if prog == "ghostty" || env("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}
	// WezTerm reached through ConPTY (Windows, WSL) forwards images only on the
	// modern build, which the environment cannot tell from the old one.
	if isWezTermEnv(env) && runtime.GOOS != "windows" && env("WSL_DISTRO_NAME") == "" && env("WSL_INTEROP") == "" {
		return true
	}

	// These implement kitty graphics although the environment detection above
	// reports sixel, or nothing, for them.
	if prog == "contour" || strings.Contains(term, "contour") {
		return true
	}
	if prog == "wayst" || strings.Contains(term, "wayst") {
		return true
	}
	if prog == "rio" || term == "rio" || strings.HasPrefix(term, "rio-") {
		return true
	}
	if prog == "warpterminal" || prog == "warp" {
		return true
	}

	return konsoleKittyGraphics(env)
}

// konsoleKittyGraphics reports Konsole 22.04 or later, which has kitty
// graphics; older versions stay on their sixel path rather than being sent a
// protocol they do not know. Konsole exports a numeric version such as 220400.
func konsoleKittyGraphics(env func(string) string) bool {
	version := env("KONSOLE_VERSION")
	if version == "" {
		return false
	}
	n, err := strconv.Atoi(version)
	return err == nil && n >= 220400
}
