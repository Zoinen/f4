//go:build windows

package app

import (
	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
	"os"
)

// Choosing the ANSI reader is only half of what a nested f4 needs on Windows.
// The other half is asking the console host for the bytes to read.
//
// A program started in f4's built-in terminal gets a pseudoconsole, and a
// pseudoconsole is a console: the input the outer f4 writes into it is parsed
// by the console host into INPUT_RECORDs long before the child sees anything.
// What the child then gets depends on one flag. Without
// ENABLE_VIRTUAL_TERMINAL_INPUT the host answers a read with the *text* of
// those records -- which is nothing at all for a wheel notch, a mouse button,
// or a function key, and nothing for a letter either when the outer f4 sent
// it in a protocol the host does not recognise as text. With the flag the
// host re-encodes the records as the VT stream the reader is written for.
//
// So the nested reader had no bytes to parse: the mouse went to the outer f4
// instead, which did the obvious thing with it and selected text and pasted
// on right-click, and the keyboard arrived in pieces (issue #87). Nothing in
// the reader could have noticed -- an input stream that stays empty looks
// exactly like a user who is not typing.
//
// Mouse input is asked for through the console mode as well as through the
// DECSET sequences the reader writes, because the console host announces its
// client's mouse mode to the terminal by watching those flags
// (microsoft/terminal#9970): with them the outer f4 learns that the program
// it started wants the mouse, and stops keeping it.

// The rest of the console input flags, spelled out beside the three in
// host_input_modes_windows.go for the same reason: it stays obvious which
// bits this file is allowed to touch.
const (
	conInProcessedInput = 0x0001 // ENABLE_PROCESSED_INPUT
	conInLineInput      = 0x0002 // ENABLE_LINE_INPUT
	conInEchoInput      = 0x0004 // ENABLE_ECHO_INPUT
	conInVTInput        = 0x0200 // ENABLE_VIRTUAL_TERMINAL_INPUT
	conInMouseInput     = 0x0010 // ENABLE_MOUSE_INPUT
	conInQuickEdit      = 0x0040 // ENABLE_QUICK_EDIT_MODE
	conInExtendedFlags  = 0x0080 // ENABLE_EXTENDED_FLAGS
)

// nestedConsoleInputMode is the mode the ANSI reader needs, given the one the
// console host handed us. Line and echo processing would hold keys back until
// Enter and print them twice; processed input would eat Ctrl+C before the
// panels see it; quick edit would let the console select text with the mouse
// instead of reporting it, and is why the console host counts it as part of
// the mouse request. Everything else is left as it was found.
func nestedConsoleInputMode(mode uint32) uint32 {
	return (mode | conInVTInput | conInMouseInput | conInExtendedFlags) &^
		(conInQuickEdit | conInProcessedInput | conInLineInput | conInEchoInput)
}

// prepareNestedConsoleInput asks the console host to speak VT to us.
//
// It runs before the reader starts, so the raw-mode switch that follows keeps
// the flag rather than clearing it, and it runs only for a nested f4: the
// console of a top-level one belongs to whoever started it.
func prepareNestedConsoleInput() {
	handle := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Redirected stdin: no console, and nothing to ask.
		vtui.DebugLog("INPUT: nested f4 has no console on stdin: %v", err)
		return
	}
	want := nestedConsoleInputMode(mode)
	if want == mode {
		return
	}
	if err := windows.SetConsoleMode(handle, want); err != nil {
		vtui.DebugLog("INPUT: nested f4 could not enable VT input: %v", err)
		return
	}
	vtui.DebugLog("INPUT: nested console input mode 0x%04X -> 0x%04X", mode, want)
}
