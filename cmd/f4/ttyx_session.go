package main

// One X session for the whole process, and a running account of what it found.
//
// The overlay and the keyboard both need the same three things: a connection,
// the identity of the terminal's window, and the event loop that watches it.
// Two sessions meant two connections, two window searches that could disagree,
// and two answers to "does this work here" — which is exactly the sort of
// thing that makes a feature work for pictures and not for keys on the same
// machine.
//
// Everything it decides is written to the debug log under one prefix, because
// none of it can be seen from outside and all of it can fail for reasons that
// depend on the window manager, the terminal and how f4 was started:
//
//	VTUI_DEBUG=1 f4 2>/dev/null; grep TTYX ~/.config/f4/logs/debug.log

import (
	"sync"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
)

var (
	ttyxSessionMu    sync.Mutex
	ttyxSessionInst  *ttyx.Session
	ttyxSessionTried bool
)

// sharedTTYXSession connects on the first call and remembers the answer,
// including a negative one.
func sharedTTYXSession() *ttyx.Session {
	ttyxSessionMu.Lock()
	defer ttyxSessionMu.Unlock()
	if ttyxSessionTried {
		return ttyxSessionInst
	}
	ttyxSessionTried = true

	sess, err := ttyx.Open()
	if err != nil {
		vtui.DebugLog("TTYX: no session: %v", err)
		return nil
	}
	if !sess.Source().Trusted() {
		// The window was a guess, and everything built on this draws
		// over that window or takes keys from it.
		vtui.DebugLog("TTYX: the terminal window was only guessed (%v), standing down", sess.Source())
		sess.Close()
		return nil
	}

	geom, gerr := sess.Geometry()
	vtui.DebugLog("TTYX: window %d found through %v, focused=%v, geometry=%+v (%v)",
		sess.Window(), sess.Source(), sess.Focused(), geom, gerr)
	ttyxSessionInst = sess

	// One watcher for the process: a redraw when anything moves, so that the
	// frame drawn while the terminal had no focus does not survive the
	// return of it.
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		for range sess.Changed() {
			frames.Redraw()
		}
	}()
	return ttyxSessionInst
}
