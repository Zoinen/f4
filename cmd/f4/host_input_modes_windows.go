//go:build windows

package main

import (
	"os"

	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

// The console input flags the mouse announcement depends on. Spelled out here
// rather than taken from a library so it stays obvious which three bits this
// file is allowed to touch: mouse input on, quick edit off (it suppresses
// mouse events, which is why the console host counts it as part of the
// request), and extended flags set, without which the quick edit bit is
// ignored.
const (
	conInMouseInput    = 0x0010 // ENABLE_MOUSE_INPUT
	conInQuickEdit     = 0x0040 // ENABLE_QUICK_EDIT_MODE
	conInExtendedFlags = 0x0080 // ENABLE_EXTENDED_FLAGS
)

// reannounceConsoleMouseMode makes the console host tell the terminal once
// more that this application wants mouse reports.
//
// The host sends that request when the flags change, and only then
// (microsoft/terminal#9970), so re-applying the mode f4 already has would be
// silent: the flag has to be dropped for a moment and set again. The request
// also reaches the terminal only with the next write to the console
// (microsoft/terminal#15711, fixed in Windows Terminal 1.19), which is why
// leaveHostConsole calls this before the redraw it ends with, not after it.
//
// Only those flags are touched. Everything else stays exactly as the reader
// set it up -- ENABLE_VIRTUAL_TERMINAL_INPUT above all, which the native
// reader needs off and a nested f4's ANSI reader needs on.
func reannounceConsoleMouseMode() {
	handle := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Redirected stdin: no console, and no mouse to ask for.
		return
	}
	want := (mode | conInMouseInput | conInExtendedFlags) &^ conInQuickEdit
	if err := windows.SetConsoleMode(handle, want&^conInMouseInput); err != nil {
		vtui.DebugLog("HOSTCONSOLE: dropping ENABLE_MOUSE_INPUT failed: %v", err)
		return
	}
	if err := windows.SetConsoleMode(handle, want); err != nil {
		vtui.DebugLog("HOSTCONSOLE: restoring ENABLE_MOUSE_INPUT failed: %v", err)
		return
	}
	vtui.DebugLog("HOSTCONSOLE: mouse mode re-announced, console input mode 0x%04X -> 0x%04X", mode, want)
}
