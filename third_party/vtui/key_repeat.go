package vtui

import "github.com/unxed/vtinput"

// keyRepeatState tells an auto-repeated key press from a separate one, by
// the rule far2l uses in console/keyboard.cpp (IsRepeatedKey): a key-down is
// a repeat when the previous key-down carried the same virtual key and no
// key-up arrived in between. Any key-up ends the run.
//
// The rule needs key-up events. Protocols that cannot report releases mark
// their events IsLegacy; far2l's TTY parser follows every such press with a
// synthesized release, so those presses are never repeats there, and they
// are never repeats here. Events the application injects (macros, keys a
// widget re-posts) did not come from a held key either: they report no
// repeat and leave the run of physical presses untouched.
type keyRepeatState struct {
	lastDownVK    uint16
	lastDownKnown bool
	repeated      bool
}

// observe updates the state for one event about to be dispatched.
// Non-key events leave it unchanged.
func (s *keyRepeatState) observe(ev *vtinput.InputEvent, injected bool) {
	if ev == nil || ev.Type != vtinput.KeyEventType {
		return
	}
	if injected {
		s.repeated = false
		return
	}
	if !ev.KeyDown || ev.IsLegacy {
		s.repeated = false
		s.lastDownKnown = false
		return
	}
	s.repeated = s.lastDownKnown && s.lastDownVK == ev.VirtualKeyCode
	s.lastDownVK = ev.VirtualKeyCode
	s.lastDownKnown = true
}

// IsRepeatedKey reports whether the key event being dispatched is an
// auto-repeat of a held key (see keyRepeatState for the rule).
func (fm *frameManager) IsRepeatedKey() bool {
	return fm.keyRepeat.repeated
}
