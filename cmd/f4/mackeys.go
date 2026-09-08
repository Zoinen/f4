package main

// Mac keyboard mode (issue #357).
//
// macOS spells the text-editing chords differently from every other platform
// f4 runs on, and the difference is not a matter of taste: Command and Option
// are what the system teaches, what every other Mac application answers to,
// and what the muscle memory of a Mac user produces. Far's layout gives the
// same keys other meanings, so a Mac user pressing Cmd+Left to reach the
// start of the line jumps a word instead.
//
// Two separate things have to be true before f4 can do anything about it.
//
// **Command has to be distinguishable from Control.** In the GUI window the
// backend already separates them the way far2l does: both Command keys arrive
// on the left Ctrl channel and the physical Control key on the right one, so
// Cmd+C works as Ctrl+C without any help from here. That split is what lets
// this layer rewrite the Command chords and leave the Control ones alone —
// Cmd+Left becomes Home while Ctrl+Left stays a word jump, and nothing a Far
// user knows is taken away. In a terminal there is no split: Terminal.app and
// iTerm2 do not send Command at all, and what does arrive on the left Ctrl
// channel there is the physical Control key. Rewriting it would break the Far
// layout for no gain, so the Command rules are skipped where the split is
// absent. See macCommandIsDistinct.
//
// **Option is the same modifier everywhere.** It reaches f4 as Alt on every
// backend, so the Option rules need no such gate.
//
// The rules are confined to the areas where a Mac user is editing text — the
// editor and dialog input fields. The panels keep the Far layout, where
// Alt+Left/Right scroll long names and walk the folder history and where
// Ctrl+Left/Right move the split.

import (
	"runtime"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Mac keyboard mode settings, as written to settings.ini.
const (
	// MacKeysAuto turns the mode on for macOS and leaves it off elsewhere.
	MacKeysAuto = "auto"
	// MacKeysOn asks for the Mac layout regardless of the platform, for an
	// Apple keyboard plugged into something else.
	MacKeysOn = "on"
	// MacKeysOff keeps the Far layout on macOS too.
	MacKeysOff = "off"
)

// ParseMacKeysMode normalizes a settings.ini value. Anything unrecognized is
// "auto": a typo must not silently change the keyboard.
func ParseMacKeysMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case MacKeysOn, "1", "yes", "true":
		return MacKeysOn
	case MacKeysOff, "0", "no", "false":
		return MacKeysOff
	default:
		return MacKeysAuto
	}
}

// macKeysEnabled reports whether the user wants the Mac layout at all.
func macKeysEnabled() bool {
	switch ParseMacKeysMode(AppConfig.MacKeyboard) {
	case MacKeysOn:
		return true
	case MacKeysOff:
		return false
	default:
		return runtime.GOOS == "darwin"
	}
}

// macCommandBackends are the rendering backends that hand Command over as the
// left Ctrl channel and the physical Control key as the right one. Only the
// one macOS uses for its window does this today; a terminal, and a GUI backend
// that folds nothing, report a single Ctrl and cannot be asked which key it
// was.
//
// The list is here rather than in vtui because the fold is a property of the
// platform layer, not of the framework's interface, and f4 only needs to know
// whether it may trust the distinction.
var macCommandBackends = map[string]bool{
	"gogpu": true,
	"qt":    true,
}

// macCommandIsDistinct reports whether a chord on the left Ctrl channel is
// Command rather than Control. It is a variable so that a test can answer for
// a machine it is not running on.
var macCommandIsDistinct = func() bool {
	return runtime.GOOS == "darwin" && macCommandBackends[vtui.ActiveBackend()]
}

// macKeyAreas are the areas the rules apply in: the editor, and the dialogs
// whose input fields are edited the same way. Everywhere else — the panels
// above all — the Far layout is what the key bar, the help and the rest of f4
// promise, and it stays.
var macKeyAreas = map[string]bool{
	"Editor": true,
	"Dialog": true,
}

// macCommandRules rewrite the Command chords. The source spelling carries the
// Ctrl prefix because that is the channel Command arrives on; the physical
// Control key spells itself RCtrl and matches nothing here, which is what
// keeps Ctrl+Left a word jump.
//
// The targets are the keys macOS means by them: Command with an arrow is the
// edge of the line or of the document, never a word.
var macCommandRules = map[string]string{
	"CtrlLeft":       "Home",
	"CtrlRight":      "End",
	"CtrlUp":         "CtrlHome",
	"CtrlDown":       "CtrlEnd",
	"CtrlShiftLeft":  "ShiftHome",
	"CtrlShiftRight": "ShiftEnd",
	"CtrlShiftUp":    "CtrlShiftHome",
	"CtrlShiftDown":  "CtrlShiftEnd",
}

// macOptionRules rewrite the Option chords, which is where macOS puts word
// navigation. Option+Backspace, the Mac way to delete the word to the left,
// has no f4 command to map onto and is left alone.
var macOptionRules = map[string]string{
	"AltLeft":       "CtrlLeft",
	"AltRight":      "CtrlRight",
	"AltShiftLeft":  "CtrlShiftLeft",
	"AltShiftRight": "CtrlShiftRight",
}

// macKeyTarget returns the key that replaces source in area, or "" when the
// Mac layout has nothing to say about it.
func macKeyTarget(area, source string) string {
	if !macKeyAreas[area] {
		return ""
	}
	if target, ok := macOptionRules[source]; ok {
		return target
	}
	if !macCommandIsDistinct() {
		return ""
	}
	return macCommandRules[source]
}

// applyMacKeys rewrites a live keystroke into the Far chord that means the
// same thing, and reports whether it did.
//
// It runs immediately after the user's own keymap.ini so that an explicit rule
// still wins: a Mac user who has rebound a key by hand gets what they asked
// for, and the built-in table only sees what keymap.ini left behind.
func applyMacKeys(area string, e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.KeyEventType {
		return false
	}
	if !macKeysEnabled() || keyRemapSuspended() {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU:
		// A modifier on its own is not a chord, and rewriting it would
		// desynchronize the key bar and the terminal forwarder.
		return false
	}

	source := EventToHotkeyString(e)
	target := macKeyTarget(area, source)
	if target == "" {
		return false
	}
	mapped := ParseFarKey(target)
	if mapped == nil || (mapped.VirtualKeyCode == 0 && mapped.Char == 0) {
		return false
	}

	e.VirtualKeyCode = mapped.VirtualKeyCode
	// The scan code described the physical key; after a substitution it
	// belongs to no key at all.
	e.VirtualScanCode = 0
	e.Char = mapped.Char
	e.UnshiftedChar = mapped.Char
	e.ControlKeyState = (e.ControlKeyState &^ keyRemapMods) | (mapped.ControlKeyState & keyRemapMods)
	vtui.DebugLog("MACKEYS: %s -> %s in area %s", source, target, area)
	syncKeyBarModifiers(e)
	return true
}
