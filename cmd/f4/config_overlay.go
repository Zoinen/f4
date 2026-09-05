package main

// One switch, two overlays, and a name that only ever described one of them.
//
// `[Images] X11Overlay` was written when the window over the terminal existed
// only on X. It now also turns off the window over the Windows console, which
// no reader of a configuration file would guess, and which had to be explained
// by hand every time somebody was asked to try turning the overlay off.
//
// So the name is `[Images] Overlay`. The old one keeps working, because a
// setting that stops being honoured is worse than one that is badly named:
// somebody who switched the overlay off in 2026 and never thought about it
// again must not have it come back on under them after an update.

// overlayEnabled resolves the setting. get is `ini.GetString` — taken as a
// function so this can be tested without a file on disk.
//
// The precedence is the obvious one: the new name wins where it is present,
// the old name answers where it is not, and the default is on. That falls out
// of using the old name's value as the new name's default rather than being
// spelled out with a lookup for presence, which the ini reader does not offer.
func overlayEnabled(get func(section, key, def string) string) bool {
	legacy := get("Images", "X11Overlay", "1")
	return get("Images", "Overlay", legacy) == "1"
}
