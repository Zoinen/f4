package terminal

import (
	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
	"sync"
)

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

var (
	ttyxSessionMu    sync.Mutex
	ttyxSessionInst  *ttyx.Session
	ttyxSessionTried bool
)

// SharedTTYXSession connects on the first call and remembers the answer,
// including a negative one. The first call looks for the terminal this
// process is running in; a session daemon replaces the answer with the
// attaching client's terminal through AttachTTYXSession.
func SharedTTYXSession() *ttyx.Session {
	ttyxSessionMu.Lock()
	defer ttyxSessionMu.Unlock()
	if ttyxSessionTried {
		return ttyxSessionInst
	}
	ttyxSessionTried = true
	ttyxSessionInst = identifyTTYXSession(ttyx.Open())
	return ttyxSessionInst
}

// AttachTTYXSession finds the terminal window again, for the client that has
// just attached: pid is that client and env reads that client's environment.
//
// It exists because of issue #980. The daemon used to identify the window
// once, from its own ancestry and environment, and keep the answer for its
// whole life. Those describe the terminal the daemon was started from, so
// after the session was attached from a new window the key grabs stayed on
// the old one — which no longer existed — and Ctrl+Shift+P in GNOME Terminal
// arrived as the passive-panel Ctrl+P again.
//
// A client in the same window as before keeps the session it has, so
// anything built on it — the picture overlay above all — is not disturbed by
// a reattach that changed nothing. A different window, or none, replaces it,
// and the old session is closed: nothing is drawn or grabbed any more on a
// window f4 is not shown in.
func AttachTTYXSession(pid int, env func(string) string) {
	sess := identifyTTYXSession(ttyx.OpenFor(pid, env))

	ttyxSessionMu.Lock()
	old := ttyxSessionInst
	if sess != nil && old != nil && old.Alive() &&
		old.Display() == sess.Display() && old.Window() == sess.Window() {
		ttyxSessionMu.Unlock()
		sess.Close()
		vtui.DebugLog("TTYX: client %d is in window %d, the same as before; keeping the session", pid, old.Window())
		return
	}
	ttyxSessionInst, ttyxSessionTried = sess, true
	ttyxSessionMu.Unlock()

	if old != nil {
		vtui.DebugLog("TTYX: client %d is not in window %d; that session is closed", pid, old.Window())
		old.Close()
	}
}

// identifyTTYXSession keeps a session only when its window was identified
// rather than guessed, and starts the one watcher it needs.
func identifyTTYXSession(sess *ttyx.Session, err error) *ttyx.Session {
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
	vtui.DebugLog("TTYX: window %d on %s found through %v, focused=%v, geometry=%+v (%v)",
		sess.Window(), sess.Display(), sess.Source(), sess.Focused(), geom, gerr)

	// One watcher per session: a redraw when anything moves, so that the
	// frame drawn while the terminal had no focus does not survive the
	// return of it. It ends with the session, which a reattach to another
	// window now does.
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		for range sess.Changed() {
			if !sess.Alive() {
				return
			}
			frames.Redraw()
		}
	}()
	return sess
}
