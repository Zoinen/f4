//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"time"

	"github.com/jezek/xgb/xproto"

	"github.com/unxed/vtui"
)

// DefaultPollInterval is how often Backend asks its Pointer where the pointer
// is. A drag is a human gesture; the target only needs a position now and then
// to keep its cursor feedback alive.
const DefaultPollInterval = 15 * time.Millisecond

// Pointer reports the pointer the way the Wine process sees it (issue #566,
// step C). The platform supplies it -- GetCursorPos and the state of the
// primary button -- so that this package does not have to.
type Pointer interface {
	// Position returns the pointer in X root coordinates (under Wine these are
	// the Windows screen coordinates of a single-monitor desktop) and whether
	// the primary button is still held.
	Position() (x, y int, down bool)
}

// dragSession is the part of Session that Backend drives; a fake stands in
// for it in tests, which therefore need no X server.
type dragSession interface {
	Drag(payload vtui.DragPayload, allowed vtui.DropAction) (vtui.DropAction, error)
	Motion(rootX, rootY int, t xproto.Timestamp)
	Release(t xproto.Timestamp)
	Close()
}

// Backend is a vtui.DragBackend that drags files out of a Wine process to a
// native X11 application through an XDND Session, where ole32 cannot (see the
// package comment). Wine holds the pointer for the whole gesture, so Backend
// plays the part of the X server's pointer events: it polls Pointer and feeds
// the Session until the button goes up.
//
// Backend is additive and unwired until step C's last part installs it. Drops
// coming in are not its business: AcceptsDrops and any gesture the X side
// cannot start go to Inner, which is the OLE path Wine bridges itself.
type Backend struct {
	// Inner is the backend that handled drags before this one; nil if none.
	Inner vtui.DragBackend
	// Pointer is required.
	Pointer Pointer
	// Interval is the polling period; DefaultPollInterval when zero.
	Interval time.Duration

	open func() (dragSession, error)
}

// NewBackend returns a Backend that opens a Session on the host's X server for
// each gesture. inner may be nil.
func NewBackend(inner vtui.DragBackend, p Pointer) *Backend {
	return &Backend{Inner: inner, Pointer: p, open: func() (dragSession, error) { return Open() }}
}

// AcceptsDrops reports what Inner reports: Wine's OLE-to-XDND bridge is what
// receives drops, so Backend adds nothing.
func (b *Backend) AcceptsDrops() bool { return b.Inner != nil && b.Inner.AcceptsDrops() }

// CanStartDrag reports whether a drag can be attempted: there is a Pointer to
// follow, and either the X side or Inner can take the gesture.
func (b *Backend) CanStartDrag() bool { return b.Pointer != nil }

// StartDrag runs one gesture on a fresh Session and blocks until it is over.
// When no Session can be opened -- not Wine, no X server, no cookie -- the
// gesture goes to Inner unchanged, or fails with the cause when there is none.
func (b *Backend) StartDrag(payload vtui.DragPayload, allowed vtui.DropAction) (vtui.DropAction, error) {
	vtui.DebugLog("WINEX11DRAG: StartDrag called")
	if b.Pointer == nil {
		return vtui.DropNone, vtui.ErrDragUnsupported
	}
	sess, err := b.open()
	if err != nil {
		vtui.DebugLog("WINEX11DRAG: no X session (%v), using the inner backend", err)
		if b.Inner != nil {
			return b.Inner.StartDrag(payload, allowed)
		}
		return vtui.DropNone, err
	}
	defer sess.Close()
	vtui.DebugLog("WINEX11DRAG: X session open, following the pointer")

	stop := make(chan struct{})
	done := make(chan struct{})
	go heartbeat(stop)
	go b.follow(sess, stop, done)
	action, err := sess.Drag(unixPayload(payload), allowed)
	close(stop)
	<-done
	vtui.DebugLog("WINEX11DRAG: drag over: action %s, error %v", action, err)
	return action, err
}

// follow reports the pointer to sess every interval. Once the button is up it
// keeps sending the release until stop is closed: Session drops pointer events
// that arrive before Drag has taken the selection, so a release sent in that
// window would otherwise be lost and Drag would never return.
func (b *Backend) follow(sess dragSession, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	interval := b.Interval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	seen, wasDown := false, false
	for ticks := 0; ; ticks++ {
		if ticks < 4 {
			vtui.DebugLog("WINEX11DRAG: tick %d, asking the pointer", ticks)
		}
		x, y, down := b.Pointer.Position()
		if ticks < 4 {
			vtui.DebugLog("WINEX11DRAG: tick %d, pointer answered", ticks)
		}
		if !seen || down != wasDown {
			vtui.DebugLog("WINEX11DRAG: pointer %d,%d, button down: %v", x, y, down)
			seen, wasDown = true, down
		} else if ticks%20 == 0 {
			vtui.DebugLog("WINEX11DRAG: following, tick %d, pointer %d,%d, button down: %v", ticks, x, y, down)
		}
		if down {
			sess.Motion(x, y, xproto.TimeCurrentTime)
		} else {
			sess.Release(xproto.TimeCurrentTime)
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// heartbeat logs every 250 ms until stop is closed. It touches no Win32 or X
// call, so a gap in these lines means the whole process stood still, and a
// gap in the follow lines alone means follow is stuck in its own calls.
func heartbeat(stop <-chan struct{}) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for n := 1; ; n++ {
		select {
		case <-stop:
			return
		case <-tick.C:
			vtui.DebugLog("WINEX11DRAG: alive, %d ms into the gesture", n*250)
		}
	}
}
