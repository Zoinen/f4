//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// X11DragSource is the source half of XDND for a program that owns an X
// connection and a window but no X11Host, for example a Windows program
// running under Wine that wants to drag files out to the Linux desktop. The
// owner feeds it the events it reads from its own connection and calls
// StartDrag from the UI goroutine; nothing here touches the host's state.
//
// A window that is not announced as XdndAware never receives the target side
// of the protocol, so this type only answers the messages a source gets.
type X11DragSource struct {
	d *x11Dnd
}

// NewX11DragSource interns the XDND atoms on conn and returns a source that
// owns the selection and the pointer grab on window and looks for targets
// below root. released, when not nil, is called once the pointer has been
// handed back, for an owner that has to be told the button is up. It returns
// nil when conn or window are missing or the server does not know the atoms,
// which leaves the caller with no drag out rather than half of one.
func NewX11DragSource(conn *xgb.Conn, window, root xproto.Window, released func()) *X11DragSource {
	if conn == nil || window == 0 {
		return nil
	}
	d := &x11Dnd{conn: conn, srcWindow: window, srcRoot: root, srcReleased: released}
	if !d.internAtoms() {
		DebugLog("XDND: could not intern the required atoms, drag out disabled")
		return nil
	}
	return &X11DragSource{d: d}
}

// BorrowPointer tells the source that another client already holds the
// pointer for the gesture, which is what a Wine window does from the button
// press on: an X server refuses a second grab, so StartDrag would fail. The
// source then neither grabs nor ungrabs, and the owner reports the pointer
// through HandleMotion and HandleRelease alone. Call it before StartDrag.
func (s *X11DragSource) BorrowPointer() {
	if s != nil {
		s.d.srcBorrowed = true
	}
}

// SkipTargets makes the source pass over every window skip reports true for,
// as if it were not XdndAware, and look at the windows below it instead. An
// owner uses it to keep a drag from landing on its own window: a Wine program
// that drags out is blocked waiting for the gesture while the host's X server
// hands the drag to the very window it started from. Call it before StartDrag;
// skip runs on the goroutine that feeds HandleMotion.
func (s *X11DragSource) SkipTargets(skip func(xproto.Window) bool) {
	if s != nil {
		s.d.srcSkip = skip
	}
}

// StartDrag runs the whole gesture and blocks until the pointer is released
// and the target has answered, so it must not run on the goroutine that feeds
// the events below.
func (s *X11DragSource) StartDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	if s == nil || s.d == nil {
		return DropNone, ErrDragUnsupported
	}
	return s.d.startDrag(payload, allowed)
}

// DraggingOut reports whether a drag of ours holds the pointer, which is when
// the owner must stop feeding pointer events to its UI.
func (s *X11DragSource) DraggingOut() bool {
	return s != nil && s.d.draggingOut()
}

// HandleMotion follows the pointer during a drag; rootX and rootY are the
// root coordinates of a MotionNotify.
func (s *X11DragSource) HandleMotion(rootX, rootY int, t xproto.Timestamp) {
	if s.DraggingOut() {
		s.d.srcMotion(rootX, rootY, t)
	}
}

// HandleRelease ends a drag at a ButtonRelease.
func (s *X11DragSource) HandleRelease(t xproto.Timestamp) {
	if s.DraggingOut() {
		s.d.srcRelease(t)
	}
}

// HandleClientMessage consumes XdndStatus and XdndFinished and returns true
// when the message was one of them.
func (s *X11DragSource) HandleClientMessage(e *xproto.ClientMessageEvent) bool {
	if s == nil || e == nil || e.Format != 32 {
		return false
	}
	switch e.Type {
	case s.d.a.status:
		s.d.onSourceStatus(e)
	case s.d.a.finished:
		s.d.onSourceFinished(e)
	default:
		return false
	}
	return true
}

// HandleSelectionRequest serves the dragged data to the target that accepted
// the drop and returns true when the request was for our selection.
func (s *X11DragSource) HandleSelectionRequest(e *xproto.SelectionRequestEvent) bool {
	return s != nil && s.d.handleSelectionRequest(e)
}
