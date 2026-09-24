package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// classicColorStyle is the style built for consoles that cannot show f4's
// default palette: it keeps to the original 16-colour-friendly one.
const classicColorStyle = "Classic"

// firstRunColorStyle answers "which colour style should f4 start with when the
// user has not chosen one", given what the console can show. ok is false when
// the built-in default should stand.
//
// A console shows only 16 colours in two cases: the Win32 console API renderer
// (backend "winapi" or "win32" -- wineconsole, ReactOS, Windows without VT
// support), which maps every colour onto a console attribute, and a terminal
// whose colour profile is 16 (the FreeBSD/Linux virtual console). The default
// style is tuned for true colour and turns into mush there (issue #513), so the
// first start takes Classic instead.
//
// It is a function of its inputs, not of the environment, so that both cases
// can be tested without a console: vtui's own detection returns true colour on
// every Windows and Wine, whatever the backend.
func firstRunColorStyle(configured bool, backend string, profile vtui.ColorProfile) (style string, ok bool) {
	if configured {
		return "", false
	}
	if backend == "winapi" || backend == "win32" || profile == vtui.ColorProfile16 {
		return classicColorStyle, true
	}
	return "", false
}

// consoleFirstRunColorStyle is firstRunColorStyle fed from the running console.
func consoleFirstRunColorStyle() (string, bool) {
	return firstRunColorStyle(config.ColorStyleConfigured(), terminal.SelectedTTYBackend, vtui.DetectColorProfile())
}

// applyFirstRunColorStyle puts the style pick names into the configuration
// when there is one. The choice is only in memory: it reaches settings.ini the
// way any other setting does, on save, and from then on the style counts as the
// user's own.
func applyFirstRunColorStyle(pick func() (string, bool)) {
	if pick == nil {
		return
	}
	style, ok := pick()
	if !ok {
		return
	}
	vtui.DebugLog("COLORS: no ColorStyle configured and the console shows 16 colours; starting with %q instead of %q",
		style, config.App.ColorStyle)
	config.App.ColorStyle = style
}
