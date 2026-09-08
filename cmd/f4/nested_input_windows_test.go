//go:build windows

package main

import "testing"

// The one flag the whole nested input path hangs on: without it the console
// host answers reads with the text of its records, and a reader that parses
// escape sequences is handed an empty stream (issue #87).
func TestNestedConsoleInputModeEnablesVTInput(t *testing.T) {
	const conPTYDefault = conInProcessedInput | conInLineInput | conInEchoInput |
		conInQuickEdit | conInExtendedFlags

	got := nestedConsoleInputMode(conPTYDefault)

	if got&conInVTInput == 0 {
		t.Errorf("VT input must be on, got 0x%04X", got)
	}
	if got&conInMouseInput == 0 || got&conInExtendedFlags == 0 {
		t.Errorf("the mouse must be asked for through the console mode too, got 0x%04X", got)
	}
	if got&conInQuickEdit != 0 {
		t.Errorf("quick edit selects text instead of reporting the mouse, got 0x%04X", got)
	}
	if got&(conInProcessedInput|conInLineInput|conInEchoInput) != 0 {
		t.Errorf("keys must arrive raw and unechoed, got 0x%04X", got)
	}
}

// Whatever else the console host set up is its business.
func TestNestedConsoleInputModeLeavesOtherFlagsAlone(t *testing.T) {
	const windowInput = 0x0008 // ENABLE_WINDOW_INPUT
	if nestedConsoleInputMode(windowInput)&windowInput == 0 {
		t.Error("flags this file does not name must survive")
	}
}
