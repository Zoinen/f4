package main

// Deciding whether to ask the terminal what it can draw.
//
// On Windows the protocol is normally chosen from environment variables, and
// that works until the one case it cannot see: Windows Terminal set as the
// default terminal application, with f4 started from a shortcut. The
// environment then belongs to whoever launched f4 (Explorer), and Windows
// Terminal attaches afterwards through handoff without writing WT_SESSION into
// it (docs/WINCON_805_HANDOVER.md F3, upstream microsoft/terminal#13006). So a
// terminal that renders sixel perfectly well is taken for one that cannot, the
// overlay path is chosen instead, and the overlay has nothing to draw on
// because the console window behind a pty is 0x0 (F2). That is the "picture
// flashes and vanishes" report, and this is the half that fixes it.
//
// The terminal itself always knows the answer, and DA1 asks it directly: a 4
// among the reported parameters means sixel. Windows Terminal answers with it
// on every launch path, and the classic conhost of 10.0.22000 answers
// ESC[?1;0c without it (F13, F14), so the question separates the two hosts on
// the evidence rather than on a guess about how f4 was started.
//
// The decision to ask is kept here, apart from the syscalls, because it has
// four conditions and every one of them is a way to get this wrong.

// graphicsProbeInputs is what the decision depends on.
type graphicsProbeInputs struct {
	// protocolIsNone is true when no graphics protocol has been chosen yet.
	protocolIsNone bool
	// ttyBackend is the selected renderer: the Win32 console backends draw
	// cells through the console API and never write a VT query.
	ttyBackend string
	// forcedProtocol is VTUI_GRAPHICS, the user's explicit choice.
	forcedProtocol string
	// windows is true on the platform this applies to.
	windows bool
}

// shouldProbeGraphics reports whether to ask the terminal via DA1.
//
// It asks only when nothing else has answered: a protocol already chosen (from
// WT_SESSION, from a Unix terminal's own detection, or by the user) is left
// alone, and a Win32 console backend is not a VT stream to ask down.
func shouldProbeGraphics(in graphicsProbeInputs) bool {
	if !in.windows || !in.protocolIsNone {
		return false
	}
	if in.forcedProtocol != "" {
		return false
	}
	return !isWinAPITTYBackend(in.ttyBackend)
}

// isWinAPITTYBackend reports whether the renderer talks to the console through
// the Win32 API rather than by writing VT sequences. Sending ESC[c there would
// print the query rather than ask it.
func isWinAPITTYBackend(name string) bool {
	return name == "winapi" || name == "win32"
}
