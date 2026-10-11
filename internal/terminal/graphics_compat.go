package terminal

import (
	"github.com/unxed/vtui"
	"os"
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

// kittyGraphicsAvailable asks vtui, which knows the terminals, whether this one
// speaks the kitty graphics protocol; what to do about it stays here.
func kittyGraphicsAvailable(env func(string) string) bool {
	return vtui.KittyGraphicsTerminal(env)
}
