//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/unxed/vtui"
)

// ErrSessionClosed is what Drag returns once Close has been called.
var ErrSessionClosed = errors.New("winex11drag: session is closed")

// Session is the XDND source half of a drag out of a Wine process, on a
// connection of its own to the real X server (issue #566, step B).
//
// It owns the connection and one 1x1 InputOnly window that never maps. That
// window, not Wine's top-level one, is the XDND source and the owner of the
// XdndSelection: Wine's own connection would be sent the SelectionRequest and
// knows nothing about the drag. A single goroutine, locked to its OS thread,
// reads the connection and feeds vtui's X11DragSource, so the source never
// sees two events at once; the owner reports the pointer through Motion and
// Release, which are queued to that goroutine.
//
// Wine holds the pointer from the button press on and an X server refuses a
// second grab, so the source runs with BorrowPointer: it neither grabs nor
// ungrabs, and the pointer is whatever Motion and Release say it is.
type Session struct {
	conn      *xgb.Conn
	src       *vtui.X11DragSource
	window    xproto.Window
	root      xproto.Window
	selection xproto.Atom // XdndSelection

	released chan struct{}
	cmds     chan func()
	quit     chan struct{}
	loopDone chan struct{}
	quitOnce sync.Once

	dragging atomic.Bool // a Drag call is in flight
	armed    bool        // loop-only: the source owns the selection, so pointer events may reach it
}

// Open opens a connection to the host's X server through libwinescape and
// starts a Session on it. It fails with ErrNotWineHost outside Wine.
func Open() (*Session, error) {
	conn, err := OpenWineX11Connection()
	if err != nil {
		return nil, err
	}
	s, err := NewSession(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

// NewSession creates the source window on conn and starts the event loop.
// The Session takes ownership of conn: Close closes it. On error conn is left
// open for the caller.
func NewSession(conn *xgb.Conn) (*Session, error) {
	if conn == nil {
		return nil, errors.New("winex11drag: no X connection")
	}
	screen := xproto.Setup(conn).DefaultScreen(conn)
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	if err := xproto.CreateWindowChecked(conn, 0, wid, screen.Root, 0, 0, 1, 1, 0,
		xproto.WindowClassInputOnly, 0, 0, nil).Check(); err != nil {
		return nil, err
	}
	sel, err := xproto.InternAtom(conn, false, uint16(len("XdndSelection")), "XdndSelection").Reply()
	if err != nil {
		return nil, err
	}
	s := &Session{
		conn:      conn,
		window:    wid,
		root:      screen.Root,
		selection: sel.Atom,
		released:  make(chan struct{}, 1),
		cmds:      make(chan func(), 64),
		quit:      make(chan struct{}),
		loopDone:  make(chan struct{}),
	}
	s.src = vtui.NewX11DragSource(conn, wid, screen.Root, func() {
		select {
		case s.released <- struct{}{}:
		default:
		}
	})
	if s.src == nil {
		_ = xproto.DestroyWindowChecked(conn, wid).Check()
		return nil, vtui.ErrDragUnsupported
	}
	s.src.BorrowPointer()
	go s.loop()
	return s, nil
}

// Window is the XDND source window.
func (s *Session) Window() xproto.Window { return s.window }

// Released delivers one value each time the source has finished with the
// pointer, which is when the owner has to treat the button as up.
func (s *Session) Released() <-chan struct{} { return s.released }

// Dragging reports whether a Drag call is in flight.
func (s *Session) Dragging() bool { return s.dragging.Load() }

// Drag runs one gesture and blocks until the target has answered or the
// pointer was released over nothing. The caller must keep reporting the
// pointer with Motion and Release from another goroutine meanwhile.
func (s *Session) Drag(payload vtui.DragPayload, allowed vtui.DropAction) (vtui.DropAction, error) {
	select {
	case <-s.quit:
		return vtui.DropNone, ErrSessionClosed
	default:
	}
	if !s.dragging.CompareAndSwap(false, true) {
		return vtui.DropNone, vtui.ErrDragBusy
	}
	defer func() {
		s.dragging.Store(false)
		s.post(func() { s.armed = false }, false)
	}()
	if skip := ownClientSkip(s.conn, s.root, s.window); skip != nil {
		s.src.SkipTargets(skip)
	}
	return s.src.StartDrag(payload, allowed)
}

// ownClientSkip returns a predicate that is true for every window of the X
// client that owns the top-level window under the pointer, or nil when there
// is none to skip. A drag out of Wine starts with the pointer over the Wine
// program's own window, and every window of that program comes from the one
// connection winex11.drv holds, so they share the resource-id base. The
// source must not offer the gesture to them: the program is blocked in the
// drag, and the one it would hand the XDND position to is the one waiting for
// it (issue #566, step C3l). own is the session's window; its client is never
// skipped.
func ownClientSkip(conn *xgb.Conn, root, own xproto.Window) func(xproto.Window) bool {
	ptr, err := xproto.QueryPointer(conn, root).Reply()
	if err != nil || ptr == nil || ptr.Child == 0 {
		return nil
	}
	return sameClientSkip(xproto.Setup(conn).ResourceIdMask, ptr.Child, own)
}

// sameClientSkip is true for windows created by the same client as sample,
// that is, whose ids differ from it only in the bits mask leaves to the
// client. It is nil when sample is own's client.
func sameClientSkip(mask uint32, sample, own xproto.Window) func(xproto.Window) bool {
	base := uint32(sample) &^ mask
	if base == uint32(own)&^mask {
		return nil
	}
	return func(w xproto.Window) bool { return uint32(w)&^mask == base }
}

// Motion reports the pointer at root coordinates rootX, rootY. Calls made
// before Drag has taken the selection, or after it is over, are dropped, so
// the owner may start reporting as soon as it has started the drag.
func (s *Session) Motion(rootX, rootY int, t xproto.Timestamp) {
	s.post(func() {
		if s.arm() {
			s.src.HandleMotion(rootX, rootY, t)
		}
	}, true)
}

// Release reports that the button went up, which ends the gesture.
func (s *Session) Release(t xproto.Timestamp) {
	s.post(func() {
		if s.arm() {
			s.src.HandleRelease(t)
		}
	}, false)
}

// Close stops the loop and closes the connection, which also takes the
// source window and the selection with it. A Drag in flight is abandoned and
// must not be waited for. Close may be called more than once.
func (s *Session) Close() {
	s.quitOnce.Do(func() {
		close(s.quit)
		s.conn.Close()
	})
	<-s.loopDone
}

// arm reports whether the source can take pointer events: StartDrag has put
// itself in charge and owns the XdndSelection, the last thing it sets up
// before it waits. Loop goroutine only.
func (s *Session) arm() bool {
	if s.armed {
		return true
	}
	if !s.src.DraggingOut() {
		return false
	}
	reply, err := xproto.GetSelectionOwner(s.conn, s.selection).Reply()
	if err != nil || reply == nil || reply.Owner != s.window {
		return false
	}
	s.armed = true
	return true
}

// post queues f for the loop goroutine. A droppable f is skipped when the
// queue is full; any other waits for room unless the session is closed.
func (s *Session) post(f func(), droppable bool) {
	if droppable {
		select {
		case s.cmds <- f:
		default:
		}
		return
	}
	select {
	case s.cmds <- f:
	case <-s.quit:
	}
}

func (s *Session) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.loopDone)

	events := make(chan xgb.Event)
	go s.readEvents(events)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			s.dispatch(ev)
		case f := <-s.cmds:
			f()
		case <-s.quit:
			return
		}
	}
}

// readEvents blocks on the connection so the loop can also wait for the
// owner's commands. It ends when the connection is closed.
func (s *Session) readEvents(out chan<- xgb.Event) {
	defer close(out)
	for {
		ev, err := s.conn.WaitForEvent()
		if ev == nil && err == nil {
			return
		}
		if ev == nil {
			continue
		}
		select {
		case out <- ev:
		case <-s.quit:
			return
		}
	}
}

func (s *Session) dispatch(ev xgb.Event) {
	switch e := ev.(type) {
	case xproto.ClientMessageEvent:
		vtui.DebugLog("WINEX11DRAG: session got a client message")
		s.src.HandleClientMessage(&e)
	case xproto.SelectionRequestEvent:
		vtui.DebugLog("WINEX11DRAG: session got a selection request")
		s.src.HandleSelectionRequest(&e)
	}
}
