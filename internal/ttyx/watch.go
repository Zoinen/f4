package ttyx

// The event loop. Everything else in this package is built on it: focus and
// geometry stop being round trips taken whenever somebody remembers to ask,
// the overlay follows the terminal window and goes away when the terminal
// does, and the selection and key handling have somewhere for their replies
// to arrive.

import (
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// watch selects the events we care about on the terminal window and starts
// the goroutine that reads them. It runs for the life of the session.
func (s *Session) watch() {
	// Selecting these on another client's window is allowed: only the
	// redirect masks and ButtonPress may be held by one client at a time,
	// and none of those are here.
	xproto.ChangeWindowAttributes(s.conn, s.win, xproto.CwEventMask,
		[]uint32{uint32(xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify)})

	if r, err := s.Geometry(); err == nil {
		s.mu.Lock()
		s.geom, s.geomOK = r, true
		s.mu.Unlock()
	}
	// focusedNow takes the lock itself, so it is asked before we take it:
	// calling it from inside would be a deadlock, and was one.
	focused := s.focusedNow()
	s.mu.Lock()
	s.focused = focused
	s.mu.Unlock()

	go s.pump()
}

// pump is the only goroutine that reads from the connection. It never holds
// the lock while it waits, so a request from another goroutine is never
// blocked behind an event that has not arrived.
func (s *Session) pump() {
	// The connection is taken once: Close sets the field to nil, and a
	// goroutine blocked in WaitForEvent would read that nil rather than the
	// connection it is waiting on. Closing the connection is what ends the
	// wait, and it ends it whatever the field says.
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return
	}

	for {
		ev, xerr := conn.WaitForEvent()
		if ev == nil && xerr == nil {
			// The connection is gone, which is also what Close does.
			s.mu.Lock()
			s.alive = false
			s.mu.Unlock()
			s.notify()
			return
		}
		if ev == nil {
			continue
		}
		s.dispatch(ev)
	}
}

func (s *Session) dispatch(ev xgb.Event) {
	switch e := ev.(type) {
	case xproto.FocusInEvent:
		if !focusEventIsReal(e.Mode, e.Detail) {
			return
		}
		s.setFocused(true)
	case xproto.FocusOutEvent:
		if !focusEventIsReal(e.Mode, e.Detail) {
			return
		}
		s.setFocused(false)

	case xproto.ConfigureNotifyEvent:
		if e.Window == s.win {
			s.refreshGeometry()
		}

	case xproto.UnmapNotifyEvent:
		if e.Window == s.win {
			s.setFocused(false)
		}
	case xproto.DestroyNotifyEvent:
		if e.Window == s.win {
			s.mu.Lock()
			s.alive = false
			s.mu.Unlock()
			// The terminal window is gone, and every overlay was a
			// child of it, so the server has already destroyed them.
			s.notify()
		}

	case xproto.KeyPressEvent:
		s.onKey(e.Detail, e.State, true)
	case xproto.KeyReleaseEvent:
		s.onKey(e.Detail, e.State, false)
	}
}

// focusEventIsReal filters the focus events that are not about the focus.
//
// The X server reports a focus change whenever a grab begins or ends, because
// from the keyboard's point of view the focus really has moved — to the
// grabbing client and back. Taking those at face value is a trap with teeth:
// this package grabs keys, so every grabbed key produced a FocusOut, the
// grabs were handed back on the spot, and the next press of that combination
// went to the terminal as if the feature had never been switched on. It
// looked exactly like a key that was never grabbed at all.
//
// A focus that moved to a child of our window has not left it either.
func focusEventIsReal(mode, detail byte) bool {
	switch mode {
	case xproto.NotifyModeGrab, xproto.NotifyModeUngrab:
		return false
	}
	return detail != xproto.NotifyDetailInferior
}

func (s *Session) setFocused(v bool) {
	s.mu.Lock()
	changed := s.focused != v
	s.focused = v
	s.mu.Unlock()
	if !changed {
		return
	}
	if v {
		s.regrabKeys()
	} else {
		// A grab held by a window nobody is typing into takes those keys
		// away from the whole desktop.
		//
		// The overlays are not touched. They are children of the
		// terminal's window, so the server hides them with it and stacks
		// anything above the terminal above them; a picture that vanished
		// whenever the terminal was not on top would be a picture behaving
		// like nothing else on the screen.
		s.releaseKeys()
	}
	s.notify()
}

// refreshGeometry works out where the window is now and moves the overlays by
// the same amount, so that a picture stays over the terminal while the
// terminal is being dragged.
func (s *Session) refreshGeometry() {
	r, err := s.geometryNow()
	if err != nil {
		return
	}
	s.mu.Lock()
	old, had := s.geom, s.geomOK
	s.geom, s.geomOK = r, true
	overlays := append([]*Overlay(nil), s.overlays...)
	s.mu.Unlock()

	if had && (old.X != r.X || old.Y != r.Y) {
		// The window moved and the server moved its children with it, so
		// nothing has to be reconfigured — only the record of where each
		// overlay is on the screen, which has moved by the same amount.
		// Reconfiguring here would move the picture *inside* the
		// terminal, which is the one thing a window that moved must not
		// do to what is drawn in it.
		dx, dy := r.X-old.X, r.Y-old.Y
		for _, o := range overlays {
			o.followedParent(dx, dy)
		}
	}
	s.notify()
}

// notify wakes whoever is watching Changed. The channel holds one token: a
// reader that is behind does not need to know how far behind it is.
func (s *Session) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Changed carries a token whenever the focus, the geometry or the life of the
// terminal window has changed. It is there so that an application which draws
// on demand knows when to draw.
func (s *Session) Changed() <-chan struct{} { return s.changed }

// Alive reports whether the terminal window and the connection are both still
// there.
func (s *Session) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive && s.conn != nil
}

func (s *Session) unregisterOverlay(o *Overlay) {
	s.mu.Lock()
	kept := s.overlays[:0]
	for _, v := range s.overlays {
		if v != o {
			kept = append(kept, v)
		}
	}
	s.overlays = kept
	s.mu.Unlock()
}
