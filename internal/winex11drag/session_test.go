//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/unxed/vtui"
)

func internAtom(t *testing.T, conn *xgb.Conn, name string) xproto.Atom {
	t.Helper()
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply() //nolint:gosec // the names are short literals
	if err != nil {
		t.Fatal(err)
	}
	return reply.Atom
}

func sendClientMessage(conn *xgb.Conn, to xproto.Window, typ xproto.Atom, data [5]uint32) {
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: to,
		Type:   typ,
		Data:   xproto.ClientMessageDataUnionData32New(data[:]),
	}
	xproto.SendEvent(conn, false, to, 0, string(ev.Bytes()))
}

// xdndTarget is a bare XdndAware window on its own connection: it accepts
// every position, asks for the dropped list and reports it.
type xdndTarget struct {
	conn   *xgb.Conn
	window xproto.Window
	got    chan string
	status chan struct{} // one value per XdndPosition answered

	position, statusAtom, drop, finished, selection, uriList, transfer, copyAct xproto.Atom
}

func newXdndTarget(t *testing.T, x, y int16) *xdndTarget {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Skipf("no X server: %v", err)
	}
	t.Cleanup(conn.Close)
	screen := xproto.Setup(conn).DefaultScreen(conn)
	g := &xdndTarget{conn: conn, got: make(chan string, 1), status: make(chan struct{}, 1)}
	g.position = internAtom(t, conn, "XdndPosition")
	g.statusAtom = internAtom(t, conn, "XdndStatus")
	g.drop = internAtom(t, conn, "XdndDrop")
	g.finished = internAtom(t, conn, "XdndFinished")
	g.selection = internAtom(t, conn, "XdndSelection")
	g.uriList = internAtom(t, conn, "text/uri-list")
	g.transfer = internAtom(t, conn, "WINEX11DRAG_TRANSFER")
	g.copyAct = internAtom(t, conn, "XdndActionCopy")
	aware := internAtom(t, conn, "XdndAware")

	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		x, y, 100, 100, 0, xproto.WindowClassInputOutput, screen.RootVisual, 0, nil).Check(); err != nil {
		t.Fatal(err)
	}
	version := make([]byte, 4)
	xgb.Put32(version, 5)
	xproto.ChangeProperty(conn, xproto.PropModeReplace, wid, aware, xproto.AtomAtom, 32, 1, version)
	if err := xproto.MapWindowChecked(conn, wid).Check(); err != nil {
		t.Fatal(err)
	}
	g.window = wid
	return g
}

func (g *xdndTarget) run(done <-chan struct{}) {
	var source xproto.Window
	for {
		select {
		case <-done:
			return
		default:
		}
		ev, _ := g.conn.PollForEvent()
		switch e := ev.(type) {
		case nil:
			time.Sleep(2 * time.Millisecond)
		case xproto.ClientMessageEvent:
			switch e.Type {
			case g.position:
				source = xproto.Window(e.Data.Data32[0])
				sendClientMessage(g.conn, source, g.statusAtom, [5]uint32{uint32(g.window), 1, 0, 0, uint32(g.copyAct)})
				select {
				case g.status <- struct{}{}:
				default:
				}
			case g.drop:
				source = xproto.Window(e.Data.Data32[0])
				xproto.ConvertSelection(g.conn, g.window, g.selection, g.uriList, g.transfer,
					xproto.Timestamp(e.Data.Data32[2]))
			}
		case xproto.SelectionNotifyEvent:
			reply, err := xproto.GetProperty(g.conn, true, g.window, g.transfer, xproto.AtomAny, 0, 1024).Reply()
			if err != nil || reply == nil {
				g.got <- ""
			} else {
				g.got <- string(reply.Value)
			}
			sendClientMessage(g.conn, source, g.finished, [5]uint32{uint32(g.window), 1, uint32(g.copyAct)})
		}
	}
}

func newTestSession(t *testing.T) *Session {
	t.Helper()
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no DISPLAY")
	}
	conn, err := xgb.NewConn()
	if err != nil {
		t.Skipf("no X server: %v", err)
	}
	s, err := NewSession(conn)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// TestSessionDragsToAnotherClient drags a file the way a Wine window does:
// the session does not hold the pointer, its owner reports it. It needs an X
// server (xvfb-run) and skips without one.
func TestSessionDragsToAnotherClient(t *testing.T) {
	target := newXdndTarget(t, 10, 10)
	s := newTestSession(t)

	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		target.run(done)
		close(stopped)
	}()
	defer func() {
		close(done)
		<-stopped
	}()

	paths := []string{"/tmp/winex11drag live drag.txt"}
	type outcome struct {
		action vtui.DropAction
		err    error
	}
	result := make(chan outcome, 1)
	go func() {
		action, err := s.Drag(vtui.DragPayload{Paths: paths}, vtui.DropCopy)
		result <- outcome{action, err}
	}()

	// The owner keeps reporting the pointer over the target until the target
	// has answered a position, then lets go.
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for answered := false; !answered; {
		select {
		case <-deadline:
			t.Fatal("the target never saw the pointer")
		case <-target.status:
			answered = true
		case <-tick.C:
			s.Motion(50, 50, xproto.TimeCurrentTime)
		}
	}
	time.Sleep(100 * time.Millisecond) // the status reaches the session
	s.Release(xproto.TimeCurrentTime)

	select {
	case r := <-result:
		if r.err != nil || r.action != vtui.DropCopy {
			t.Fatalf("Drag = %v, %v; want copy", r.action, r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Drag did not finish")
	}
	select {
	case got := <-target.got:
		if want := vtui.FormatURIList(paths); got != want {
			t.Fatalf("target received %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the target received no data")
	}
	select {
	case <-s.Released():
	default:
		t.Fatal("the owner was not told the button is up")
	}
	if s.Dragging() {
		t.Fatal("still dragging after Drag returned")
	}
}

func TestSessionIdleCallsAndClose(t *testing.T) {
	s := newTestSession(t)
	if s.Window() == 0 {
		t.Fatal("no source window")
	}
	// Pointer reports with no drag in flight are dropped quietly.
	s.Motion(1, 1, xproto.TimeCurrentTime)
	s.Release(xproto.TimeCurrentTime)
	if s.Dragging() {
		t.Fatal("dragging without a drag")
	}
	s.Close()
	s.Close()
	if _, err := s.Drag(vtui.DragPayload{Paths: []string{"/tmp/x"}}, vtui.DropCopy); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Drag after Close = %v, want %v", err, ErrSessionClosed)
	}
}

func TestNewSessionNeedsConnection(t *testing.T) {
	if _, err := NewSession(nil); err == nil {
		t.Fatal("NewSession(nil) succeeded")
	}
}
