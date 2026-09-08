package main

// Coming back from the host console is the moment f4 has to say again what it
// wants from the terminal's input.
//
// While the host console is up, everything the child writes reaches the
// terminal untouched, and f4 adds a protective reset of its own on the way
// out. Both can turn mouse reporting off. A child that used the mouse
// disables tracking when it exits; on Windows the console host does it on the
// child's behalf, because it announces the mouse mode of its client to the
// terminal (ENABLE_MOUSE_INPUT with quick edit off becomes DECSET 1003/1006,
// microsoft/terminal#9970) -- and a pseudoconsole of f4's own sends those
// announcements straight into the passthrough stream, so they end up at the
// real terminal.
//
// Nothing on the way back to the panels used to undo any of that. f4 kept its
// own idea of a working mouse -- the console input handle still had
// ENABLE_MOUSE_INPUT set, the VT reader still expected SGR reports -- while
// the terminal had stopped reporting, so clicks went to the terminal instead
// of the panels: in Windows Terminal that means text selection and the
// context menu (#924, and the Esc/Ctrl+O half of #923). Nothing but a
// restart brought the mouse back, because the request is made once, at
// startup.
//
// So leaveHostConsole ends by asking again. What the request is depends on
// how f4 reads input, not on which terminal is in front of it.

import (
	"runtime"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// hostInputEnableSeq is what vtinput writes at startup for the VT reader (see
// EnableProtocols): cell motion and any-event mouse, URXVT and SGR
// coordinates, focus tracking and bracketed paste. Repeating it verbatim
// leaves the terminal in the state f4 began with, whatever the child did to
// it meanwhile -- and, on the terminals where the protective reset above did
// run, undoes that reset as well.
const hostInputEnableSeq = "\x1b[?1002h\x1b[?1003h\x1b[?1015h\x1b[?1006h\x1b[?1004h\x1b[?2004h"

// nativeConsoleInput reports whether f4 reads input through the Windows
// console API (ReadConsoleInputW) rather than as a VT byte stream. The test
// mirrors the one vtinput makes in EnableProtocols, which sends no ANSI
// protocol sequences at all in that case: there the console input mode *is*
// the request, and repeating it as VT would have ConPTY turn the terminal's
// SGR mouse reports into lossy MOUSE_EVENT records -- the swapped buttons of
// #923.
func nativeConsoleInput() bool {
	if vtinput.InputMode == "ConPTY" {
		return true
	}
	return runtime.GOOS == "windows" && vtinput.InputMode == ""
}

// hostInputRestoreSeq returns the sequences to write to the host terminal on
// the way back to the panels, or "" when the request is not a VT one: a
// native Windows console reader asks through the console mode instead (see
// reannounceConsoleMouseMode), and a FreeBSD syscons has no protocols to ask
// for -- vtui enables none there, and an unknown DEC private mode would be
// printed as text rather than obeyed.
func hostInputRestoreSeq(nativeInput, freeBSDConsole bool) string {
	if nativeInput || freeBSDConsole {
		return ""
	}
	return hostInputEnableSeq
}

// restoreHostInputModes re-asserts the input protocols f4 itself needs after
// the host console gave the terminal back.
func restoreHostInputModes() {
	if seq := hostInputRestoreSeq(nativeConsoleInput(), vtui.IsFreeBSDConsole); seq != "" {
		vtui.WritePassthrough([]byte(seq))
	}
	reannounceConsoleMouseMode()
}
