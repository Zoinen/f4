package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/unxed/vtui"
)

// PreferCompatibleGraphicsProtocol chooses the Kitty transport when the
// terminal is known to implement it. This matters for terminals whose Sixel
// decoder changes indexed palette entries in place, while vtui's true-color
// Sixel encoder intentionally changes the palette for every sixel band.
//
// This is deliberately an application-level choice: the shared Sixel encoder
// remains unchanged for terminals that handle its full-color output correctly.
func PreferCompatibleGraphicsProtocol(scr *vtui.ScreenBuf) {
	preferCompatibleGraphicsProtocol(scr, os.Getenv)
}

func preferCompatibleGraphicsProtocol(scr *vtui.ScreenBuf, env func(string) string) {
	if scr == nil || !kittyGraphicsAvailable(env) {
		return
	}

	// A valid VTUI_GRAPHICS value is an explicit user choice. Do not silently
	// replace it, especially VTUI_GRAPHICS=sixel, which remains useful on
	// Konsole versions where the user has a reason to force that protocol.
	if forced := env("VTUI_GRAPHICS"); forced != "" {
		if _, ok := vtui.ParseGraphicsProtocol(forced); ok {
			return
		}
	}

	if scr.Graphics().Protocol() == vtui.GraphicsSixel {
		scr.Graphics().SetProtocol(vtui.GraphicsKitty)
		vtui.DebugLog("GRAPHICS: Sixel is palette-incompatible here; using kitty graphics")
		return
	}

	// Some Kitty-capable terminals are not identified by vtui yet and start
	// with GraphicsNone. A known terminal marker is sufficient to select the
	// protocol; an explicit VTUI_GRAPHICS value was handled above.
	if scr.Graphics().Protocol() == vtui.GraphicsNone {
		scr.Graphics().SetProtocol(vtui.GraphicsKitty)
		vtui.DebugLog("GRAPHICS: using detected kitty graphics")
	}
}

func kittyGraphicsAvailable(env func(string) string) bool {
	if env == nil {
		return false
	}

	term := strings.ToLower(env("TERM"))
	prog := strings.ToLower(env("TERM_PROGRAM"))

	// Kitty, Ghostty and WezTerm already have explicit markers in vtui. Keep
	// these checks here as well so a stronger marker wins over another marker,
	// such as WT_SESSION inherited by a nested session.
	if env("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty") {
		return true
	}
	if prog == "ghostty" || env("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}
	if isWezTerm(env) && runtime.GOOS != "windows" && env("WSL_DISTRO_NAME") == "" && env("WSL_INTEROP") == "" {
		return true
	}

	// These terminals implement Kitty graphics but vtui's environment
	// detection currently reports Sixel (or no protocol) for them.
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

	return konsoleKittyGraphicsAvailable(env)
}

func isWezTerm(env func(string) string) bool {
	return strings.ToLower(env("TERM_PROGRAM")) == "wezterm" || env("WEZTERM_PANE") != ""
}

func konsoleKittyGraphicsAvailable(env func(string) string) bool {
	if env == nil {
		return false
	}
	version := env("KONSOLE_VERSION")
	if version == "" {
		return false
	}

	// Konsole exports a numeric version such as 220400. Kitty graphics support
	// is present in Konsole 22.04 and later. Keep older versions on their
	// existing sixel path instead of sending a protocol they do not know.
	numericVersion, err := strconv.Atoi(version)
	return err == nil && numericVersion >= 220400
}
