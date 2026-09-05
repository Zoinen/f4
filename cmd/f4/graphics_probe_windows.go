//go:build windows

package main

import (
	"os"
	"runtime"

	"github.com/unxed/vtui"
)

// probeGraphicsIfUnknown asks the terminal, through DA1, whether it can draw
// pictures, when nothing else has said so.
//
// Called from ManageSessions after PrepareTerminal (VT output is on, so the
// query is a query and not eight characters printed on the screen) and before
// InstallConsoleOverlay (which must see the answer) and before vtinput's
// reader starts (which would otherwise consume the reply as keystrokes).
//
// The reasoning for asking at all, and the four conditions, are in
// graphics_probe_decision.go. When the terminal says yes, the overlay path
// stops being considered at all: InstallConsoleOverlay returns early once a
// protocol is set.
func probeGraphicsIfUnknown(scr *vtui.ScreenBuf) {
	if scr == nil {
		return
	}
	if !shouldProbeGraphics(graphicsProbeInputs{
		protocolIsNone: scr.Graphics().Protocol() == vtui.GraphicsNone,
		ttyBackend:     SelectedTTYBackend,
		forcedProtocol: os.Getenv("VTUI_GRAPHICS"),
		windows:        runtime.GOOS == "windows",
	}) {
		return
	}

	protocols := vtui.ProbeGraphicsProtocols()
	if len(protocols) == 0 {
		// The classic conhost of 10.0.22000 lands here: it answers
		// ESC[?1;0c, without the 4 (F13). The overlay is the right path for
		// it, and step 1 lets it through because its window really is a
		// visible ConsoleWindowClass.
		vtui.DebugLog("GRAPHICS: the terminal did not declare sixel in DA1; keeping the console overlay path")
		return
	}
	scr.Graphics().SetProtocol(protocols[0])
	vtui.DebugLog("GRAPHICS: the terminal answered DA1 with graphics support; using %v", protocols[0])
}
