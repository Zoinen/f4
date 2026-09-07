package ttyx

// The keyboard, issue #662. A TTY carries a small and lossy subset of what the
// user pressed: Ctrl+Shift+Up and Ctrl+Up are the same three bytes unless the
// terminal implements one of the modern keyboard protocols, and most do not.
// With a connection to the X server the real key is available.
//
// Grabs are shared state on the X server, which is what makes this the
// dangerous half of the package. A key grabbed by us is a key the rest of the
// desktop no longer receives, so the grab is taken when the terminal has the
// focus and dropped the moment it does not. The event loop does that; nothing
// here has to be remembered by the caller.

import (
	"errors"
	"math"
	"os"
	"sync"

	"github.com/jezek/xgb/xproto"
	"github.com/unxed/keytrans"
	"github.com/unxed/vtinput"
)

// Modifier is a set of the modifiers a combination needs held down.
type Modifier uint16

const (
	ModShift Modifier = 1 << 0
	ModCtrl  Modifier = 1 << 2
	ModAlt   Modifier = 1 << 3
)

// Combo is one key combination to take from the terminal, named by the keysym
// of the key itself and the modifiers that go with it.
type Combo struct {
	Keysym uint32
	Mods   Modifier
}

// lockMasks are the modifiers a grab has to be repeated for, because a grab
// matches the modifier state exactly and the user may well have Caps Lock or
// Num Lock on.
var lockMasks = []uint16{
	0,
	xproto.ModMaskLock,
	xproto.ModMask2, // Num Lock on every keyboard map in practice.
	xproto.ModMaskLock | xproto.ModMask2,
}

type keyState struct {
	mu      sync.Mutex
	combos  []Combo
	codes   []xproto.Keycode
	held    bool
	events  chan *vtinput.InputEvent
	tr      keytrans.Translator
	dropped uint64
	report  []GrabResult
}

// GrabResult is what the X server said to one request. A grab is the one thing
// here that can be refused for a reason outside f4 — another client holding
// the same combination — and a refusal is silent unless it is asked for.
type GrabResult struct {
	Keysym uint32
	Mods   Modifier
	Code   xproto.Keycode
	Err    error
}

// GrabReport is the outcome of the last set of grabs.
func (s *Session) GrabReport() []GrabResult {
	s.mu.Lock()
	ks := s.keys
	s.mu.Unlock()
	if ks == nil {
		return nil
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return append([]GrabResult(nil), ks.report...)
}

// Keys is the stream of key events taken from the X server. It is nil until
// GrabKeys has been called.
func (s *Session) Keys() <-chan *vtinput.InputEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys == nil {
		return nil
	}
	return s.keys.events
}

// Dropped counts the key events thrown away because nobody was reading them,
// which is the only honest thing to do with a full channel in an event loop
// that must not block.
func (s *Session) Dropped() uint64 {
	s.mu.Lock()
	ks := s.keys
	s.mu.Unlock()
	if ks == nil {
		return 0
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.dropped
}

// grabsHeld reports whether the grabs are currently taken. It exists for the
// test that checks they are given back with the focus.
func (s *Session) grabsHeld() bool {
	s.mu.Lock()
	ks := s.keys
	s.mu.Unlock()
	if ks == nil {
		return false
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.held
}

// GrabKeys asks the X server for the given combinations on the terminal
// window. They are taken while the terminal has the focus and released when
// it loses it; calling it again replaces the set.
//
// The combinations are named by keysym rather than by keycode so that a
// caller does not have to know the layout, and so that the set survives the
// user switching layouts.
func (s *Session) GrabKeys(combos []Combo) error {
	s.mu.Lock()
	if s.conn == nil {
		s.mu.Unlock()
		return ErrNoDisplay
	}
	if s.keys == nil {
		s.keys = &keyState{
			events: make(chan *vtinput.InputEvent, 256),
			tr: keytrans.NewX11Translator(keytrans.OSInfo{
				DisplayString: os.Getenv("DISPLAY"),
				XgbConn:       s.conn,
				WindowID:      uint32(s.win),
			}),
		}
	}
	ks := s.keys
	focused := s.focused
	s.mu.Unlock()

	codes, err := s.keycodesFor(combos)
	if err != nil {
		return err
	}

	s.releaseKeys()
	ks.mu.Lock()
	ks.combos, ks.codes = append([]Combo(nil), combos...), codes
	ks.mu.Unlock()

	if focused {
		s.regrabKeys()
	}
	return nil
}

// UngrabKeys gives every combination back to the desktop.
func (s *Session) UngrabKeys() {
	s.releaseKeys()
	s.mu.Lock()
	ks := s.keys
	s.mu.Unlock()
	if ks == nil {
		return
	}
	ks.mu.Lock()
	ks.combos, ks.codes = nil, nil
	ks.mu.Unlock()
}

// keycodesFor resolves each combination's keysym against the current keyboard
// map. A keysym that is not on the keyboard is skipped rather than refused:
// the caller is asking for a set, and part of a set is better than none.
func (s *Session) keycodesFor(combos []Combo) ([]xproto.Keycode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil, ErrNoDisplay
	}

	setup := xproto.Setup(s.conn)
	first := setup.MinKeycode
	keycodeCount := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1
	if keycodeCount <= 0 || keycodeCount > math.MaxUint8 {
		return nil, ErrNoDisplay
	}
	// #nosec G115 -- the explicit MaxUint8 check above makes this conversion lossless.
	count := byte(keycodeCount)
	mapping, err := xproto.GetKeyboardMapping(s.conn, first, count).Reply()
	if err != nil || mapping == nil || mapping.KeysymsPerKeycode == 0 {
		return nil, ErrNoDisplay
	}

	per := int(mapping.KeysymsPerKeycode)
	out := make([]xproto.Keycode, len(combos))
	for i, c := range combos {
		for k := 0; k < int(count); k++ {
			found := false
			for j := 0; j < per; j++ {
				if uint32(mapping.Keysyms[k*per+j]) == c.Keysym {
					// #nosec G115 -- k is bounded by the validated keycode span beginning at first.
					out[i] = xproto.Keycode(int(first) + k)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	return out, nil
}

func (m Modifier) xmask() uint16 {
	var v uint16
	if m&ModShift != 0 {
		v |= xproto.ModMaskShift
	}
	if m&ModCtrl != 0 {
		v |= xproto.ModMaskControl
	}
	if m&ModAlt != 0 {
		v |= xproto.ModMask1
	}
	return v
}

// regrabKeys takes the grabs. It is called when the terminal gains the focus
// and after the set has changed.
func (s *Session) regrabKeys() {
	s.mu.Lock()
	ks := s.keys
	conn, win := s.conn, s.win
	s.mu.Unlock()
	if ks == nil || conn == nil {
		return
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()
	if ks.held || len(ks.codes) == 0 {
		return
	}
	ks.report = ks.report[:0]
	for i, code := range ks.codes {
		res := GrabResult{Keysym: ks.combos[i].Keysym, Mods: ks.combos[i].Mods, Code: code}
		if code == 0 {
			res.Err = errNoKeycode
			ks.report = append(ks.report, res)
			continue
		}
		base := ks.combos[i].Mods.xmask()
		for _, lock := range lockMasks {
			// Checked, because an unchecked grab that the server
			// refuses is a combination that silently keeps going to
			// the terminal instead — which looks from the outside
			// exactly like the feature not being there.
			if err := xproto.GrabKeyChecked(conn, true, win, base|lock, code,
				xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil && res.Err == nil {
				res.Err = err
			}
		}
		ks.report = append(ks.report, res)
	}
	ks.held = true
}

// errNoKeycode means the keysym asked for is not on this keyboard.
var errNoKeycode = errors.New("this keysym is not on the keyboard")

// releaseKeys gives the grabs back. It is called the moment the terminal
// loses the focus: a grab held by a window nobody is typing into takes those
// keys away from every other application on the desktop.
func (s *Session) releaseKeys() {
	s.mu.Lock()
	ks := s.keys
	conn, win := s.conn, s.win
	s.mu.Unlock()
	if ks == nil || conn == nil {
		return
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()
	if !ks.held {
		return
	}
	for i, code := range ks.codes {
		if code == 0 {
			continue
		}
		base := ks.combos[i].Mods.xmask()
		for _, lock := range lockMasks {
			xproto.UngrabKey(conn, code, win, base|lock)
		}
	}
	ks.held = false
}

// onKey turns an X key event into the event the rest of f4 speaks. The
// translation is keytrans, the same one vtui's X11 backend uses, so a key
// arriving this way is indistinguishable from one arriving in GUI mode.
func (s *Session) onKey(code xproto.Keycode, state uint16, down bool) {
	s.mu.Lock()
	ks := s.keys
	s.mu.Unlock()
	if ks == nil {
		return
	}

	ks.mu.Lock()
	tr := ks.tr
	ks.mu.Unlock()
	if tr == nil {
		return
	}

	wev := tr.TranslateX11(uint8(code), state, down)
	if wev.VirtualKeyCode == 0 && wev.Char == 0 {
		return
	}
	ev := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         wev.KeyDown,
		VirtualKeyCode:  wev.VirtualKeyCode,
		Char:            wev.Char,
		ControlKeyState: modsFromState(state),
		InputSource:     wev.InputSource,
	}
	select {
	case ks.events <- ev:
	default:
		// The reader is behind. Blocking here would stop the event loop,
		// and with it the focus tracking that keeps the grabs honest.
		ks.mu.Lock()
		ks.dropped++
		ks.mu.Unlock()
	}
}

// modsFromState turns the X modifier mask into the state the rest of f4
// speaks. The sides cannot be told apart from the mask alone — X reports that
// a control key is down, not which one — so the left one is reported, which is
// what a terminal reports too.
func modsFromState(state uint16) vtinput.ControlKeyState {
	var m vtinput.ControlKeyState
	if state&xproto.ModMaskShift != 0 {
		m |= vtinput.ShiftPressed
	}
	if state&xproto.ModMaskControl != 0 {
		m |= vtinput.LeftCtrlPressed
	}
	if state&xproto.ModMask1 != 0 {
		m |= vtinput.LeftAltPressed
	}
	if state&xproto.ModMaskLock != 0 {
		m |= vtinput.CapsLockOn
	}
	if state&xproto.ModMask2 != 0 {
		m |= vtinput.NumLockOn
	}
	return m
}
