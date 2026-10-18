//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"os"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// liveDndTarget is a bare XdndAware window on its own connection: it accepts
// every position, asks for the dropped data and reports it back.
type liveDndTarget struct {
	conn   *xgb.Conn
	d      *x11Dnd
	window xproto.Window
	got    chan string
}

func newLiveDndTarget(t *testing.T, x, y int16) *liveDndTarget {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Skipf("no X server: %v", err)
	}
	t.Cleanup(conn.Close)
	screen := xproto.Setup(conn).DefaultScreen(conn)
	d := &x11Dnd{conn: conn}
	if !d.internAtoms() {
		t.Fatal("the server does not know the XDND atoms")
	}
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		x, y, 100, 100, 0, xproto.WindowClassInputOutput, screen.RootVisual, 0, nil).Check(); err != nil {
		t.Fatal(err)
	}
	version := make([]byte, 4)
	xgb.Put32(version, xdndVersion)
	xproto.ChangeProperty(conn, xproto.PropModeReplace, wid, d.a.aware, xproto.AtomAtom, 32, 1, version)
	if err := xproto.MapWindowChecked(conn, wid).Check(); err != nil {
		t.Fatal(err)
	}
	return &liveDndTarget{conn: conn, d: d, window: wid, got: make(chan string, 1)}
}

// run answers the source until the data has arrived and been acknowledged,
// or the test ends.
func (g *liveDndTarget) run(done <-chan struct{}) {
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
			case g.d.a.position:
				source = xproto.Window(e.Data.Data32[0])
				g.d.send(source, g.d.a.status, [5]uint32{uint32(g.window), 1, 0, 0, uint32(g.d.a.actCopy)})
			case g.d.a.drop:
				source = xproto.Window(e.Data.Data32[0])
				xproto.ConvertSelection(g.conn, g.window, g.d.a.selection, g.d.a.uriList,
					g.d.a.transfer, xproto.Timestamp(e.Data.Data32[2]))
			}
		case xproto.SelectionNotifyEvent:
			reply, err := xproto.GetProperty(g.conn, true, g.window, g.d.a.transfer,
				xproto.AtomAny, 0, 1024).Reply()
			if err != nil || reply == nil {
				g.got <- ""
			} else {
				g.got <- string(reply.Value)
			}
			g.d.send(source, g.d.a.finished, [5]uint32{uint32(g.window), 1, uint32(g.d.a.actCopy)})
		}
	}
}

// TestX11DragSourceBorrowedPointerLive drags a file from a source that does
// not hold the pointer to a second client on the same server, the way a Wine
// window drags out. It needs an X server (xvfb-run) and skips without one.
func TestX11DragSourceBorrowedPointerLive(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no DISPLAY")
	}
	target := newLiveDndTarget(t, 10, 10)

	conn, err := xgb.NewConn()
	if err != nil {
		t.Skipf("no X server: %v", err)
	}
	t.Cleanup(conn.Close)
	screen := xproto.Setup(conn).DefaultScreen(conn)
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := xproto.CreateWindowChecked(conn, 0, wid, screen.Root, 0, 0, 1, 1, 0,
		xproto.WindowClassInputOnly, 0, 0, nil).Check(); err != nil {
		t.Fatal(err)
	}

	released := make(chan struct{}, 1)
	src := NewX11DragSource(conn, wid, screen.Root, func() { released <- struct{}{} })
	if src == nil {
		t.Fatal("no drag source")
	}
	src.BorrowPointer()

	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		target.run(done)
		close(stopped)
	}()
	defer func() {
		close(done)
		<-stopped
	}()

	paths := []string{"/tmp/vtui live drag.txt"}
	type outcome struct {
		action DropAction
		err    error
	}
	result := make(chan outcome, 1)
	go func() {
		action, err := src.StartDrag(DragPayload{Paths: paths}, DropCopy)
		result <- outcome{action, err}
	}()

	// One goroutine owns the pointer and the source's events, as in a real
	// owner: it moves over the target, waits for its status, releases.
	moved, dropped := false, false
	deadline := time.After(10 * time.Second)
	for finished := false; !finished; {
		select {
		case <-deadline:
			t.Fatalf("drag did not finish (moved=%v dropped=%v)", moved, dropped)
		case r := <-result:
			if r.err != nil || r.action != DropCopy {
				t.Fatalf("StartDrag = %v, %v; want copy", r.action, r.err)
			}
			finished = true
			continue
		default:
		}
		switch e := pollSourceEvent(conn).(type) {
		case nil:
			time.Sleep(2 * time.Millisecond)
		case xproto.ClientMessageEvent:
			src.HandleClientMessage(&e)
		case xproto.SelectionRequestEvent:
			src.HandleSelectionRequest(&e)
		}
		switch {
		case !moved && src.DraggingOut() && ownsSelection(conn, src.d.a.selection, wid):
			src.HandleMotion(50, 50, xproto.TimeCurrentTime)
			moved = true
		case moved && !dropped && src.d.srcStatus != DropNone:
			src.HandleRelease(xproto.TimeCurrentTime)
			dropped = true
		}
	}

	if want := FormatURIList(paths); <-target.got != want {
		t.Fatalf("target received a different list than %q", want)
	}
	select {
	case <-released:
	default:
		t.Fatal("the owner was not told the button is up")
	}
}

func pollSourceEvent(conn *xgb.Conn) xgb.Event {
	ev, _ := conn.PollForEvent()
	return ev
}

// ownsSelection reports whether window holds selection on the server, which
// is the last thing StartDrag sets up before it waits.
func ownsSelection(conn *xgb.Conn, selection xproto.Atom, window xproto.Window) bool {
	reply, err := xproto.GetSelectionOwner(conn, selection).Reply()
	return err == nil && reply != nil && reply.Owner == window
}

// TestFindTargetSkipsWindowsTheOwnerNamesLive checks that a window the owner
// excludes is passed over although it announces XdndAware, and that without
// the exclusion it is found. It needs an X server (xvfb-run) and skips without
// one.
func TestFindTargetSkipsWindowsTheOwnerNamesLive(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no DISPLAY")
	}
	newLiveDndTarget(t, 10, 10)

	conn, err := xgb.NewConn()
	if err != nil {
		t.Skipf("no X server: %v", err)
	}
	t.Cleanup(conn.Close)
	screen := xproto.Setup(conn).DefaultScreen(conn)
	d := &x11Dnd{conn: conn, srcRoot: screen.Root}
	if !d.internAtoms() {
		t.Fatal("the server does not know the XDND atoms")
	}

	// A window manager may put a frame between the root and the target, so
	// compare with the window found while nothing is skipped.
	want, version := d.findTarget(50, 50)
	if want == 0 || version == 0 {
		t.Skip("the target window is not reachable at 50,50 on this server")
	}

	var asked []xproto.Window
	d.srcSkip = func(w xproto.Window) bool {
		asked = append(asked, w)
		return w == want
	}
	if w, _ := d.findTarget(50, 50); w == want {
		t.Fatalf("the skipped window %d was still chosen", want)
	}
	if len(asked) == 0 || asked[len(asked)-1] == 0 {
		t.Fatalf("skip was not asked about the windows under the pointer: %v", asked)
	}
}
