package ttyx

import (
	"os"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// waitFor polls, because the event loop is a goroutine and the X server is on
// the other end of a socket: nothing here is synchronous.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// focus points the keyboard at a window and waits for the session to notice.
func (f *xFixture) focus(t *testing.T, win xproto.Window) {
	t.Helper()
	if err := xproto.SetInputFocusChecked(f.conn, xproto.InputFocusParent,
		win, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatalf("set focus: %v", err)
	}
}

// openOn makes a session that believes the fixture's window is the terminal.
func (f *xFixture) openOn(t *testing.T) *Session {
	t.Helper()
	env := map[string]string{
		"DISPLAY":  os.Getenv("DISPLAY"),
		"WINDOWID": itoa(uint32(f.term)),
	}
	s, err := open(func(k string) string { return env[k] }, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestSessionTracksFocusThroughTheEventLoop(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)

	f.focus(t, f.term)
	waitFor(t, "the focus to arrive", s.Focused)

	// A token has to reach whoever is waiting to redraw.
	select {
	case <-s.Changed():
	case <-time.After(2 * time.Second):
		t.Fatal("a focus change must be announced on Changed")
	}

	f.focus(t, f.root)
	waitFor(t, "the focus to leave", func() bool { return !s.Focused() })
}

func TestSessionGeometryFollowsTheWindow(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)

	if got, err := s.Geometry(); err != nil || got.X != 40 || got.Y != 60 {
		t.Fatalf("before the move: %+v %v", got, err)
	}

	err := xproto.ConfigureWindowChecked(f.conn, f.term,
		xproto.ConfigWindowX|xproto.ConfigWindowY,
		[]uint32{140, 260}).Check()
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	waitFor(t, "the geometry to catch up", func() bool {
		g, err := s.Geometry()
		return err == nil && g.X == 140 && g.Y == 260
	})
}

// An override-redirect window is above everything, so it must not be on the

// The overlay is a child of the terminal's own window. Everything else here
// follows from that, and it is worth asserting directly because it is one
// argument to CreateWindow and nothing else would notice if it changed.
func TestOverlayIsAChildOfTheTerminal(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)

	ov, err := s.NewOverlay()
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	defer ov.Close()

	tree, err := xproto.QueryTree(f.conn, ov.Window()).Reply()
	if err != nil || tree == nil {
		t.Fatalf("query tree: %v", err)
	}
	if tree.Parent != f.term {
		t.Errorf("parent is %d, want the terminal window %d", tree.Parent, f.term)
	}
}

// Being a child means being clipped to the terminal, so a picture cannot
// spill past the edge of the window it belongs to — which is what an
// override-redirect window over the top of everything did.
func TestOverlayIsClippedToTheTerminal(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	ov, err := s.NewOverlay()
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	defer ov.Close()

	// The fixture window is at 40,60 and 400x300. Hang the overlay off its
	// right-hand edge.
	if err := ov.Place(Rect{X: 400, Y: 100, W: 80, H: 40}); err != nil {
		t.Fatalf("place: %v", err)
	}
	pix := make([]byte, 80*40*4)
	for i := range pix {
		if i%4 >= 2 {
			pix[i] = 255
		}
	}
	if err := ov.Draw(pix, 80, 40, 80*4); err != nil {
		t.Fatalf("draw: %v", err)
	}
	f.conn.Sync()

	// Inside the terminal the picture is there; past its edge, at 440, the
	// server has clipped it away.
	if !f.pixelIsBlue(t, 420, 110) {
		t.Error("the part inside the terminal must be drawn")
	}
	if f.pixelIsBlue(t, 445, 110) {
		t.Error("nothing may be drawn past the edge of the terminal window")
	}
}

// pixelIsBlue reads one pixel off the screen.
func (f *xFixture) pixelIsBlue(t *testing.T, x, y int16) bool {
	t.Helper()
	reply, err := xproto.GetImage(f.conn, xproto.ImageFormatZPixmap,
		xproto.Drawable(f.root), x, y, 1, 1, 0xFFFFFFFF).Reply()
	if err != nil || reply == nil || len(reply.Data) < 4 {
		t.Fatalf("read back at %d,%d: %v", x, y, err)
	}
	return reply.Data[0] == 0xFF && reply.Data[1] == 0 && reply.Data[2] == 0
}

// A picture drawn in a window does not disappear because the window is not on
// top: the alt-tab switcher, and anything else the window manager raises, is
// above the terminal and therefore above its children. Taking the overlay
// down by hand used to be necessary and is now the wrong thing.
func TestOverlayStaysWhenTheTerminalLosesFocus(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	ov, err := s.NewOverlay()
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	defer ov.Close()
	if err := ov.Place(Rect{X: 100, Y: 100, W: 40, H: 40}); err != nil {
		t.Fatalf("place: %v", err)
	}
	if !ov.Visible() {
		t.Fatal("the overlay must be up to begin with")
	}

	f.focus(t, f.root)
	waitFor(t, "the focus to leave", func() bool { return !s.Focused() })
	if !ov.Visible() {
		t.Error("the picture must stay where it was drawn")
	}
}

// A picture has to stay over the terminal while the terminal is dragged.
func TestOverlayFollowsTheWindow(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	ov, err := s.NewOverlay()
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	defer ov.Close()
	if err := ov.Place(Rect{X: 50, Y: 70, W: 20, H: 20}); err != nil {
		t.Fatalf("place: %v", err)
	}

	err = xproto.ConfigureWindowChecked(f.conn, f.term,
		xproto.ConfigWindowX|xproto.ConfigWindowY,
		[]uint32{60, 90}).Check()
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	// The window moved by twenty and thirty, so the overlay must too.
	waitFor(t, "the overlay to follow", func() bool {
		r := ov.Rect()
		return r.X == 70 && r.Y == 100
	})
}

// An overlay that was never placed must not appear when the focus arrives.
func TestOverlayStaysDownUntilItIsPlaced(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)

	ov, err := s.NewOverlay()
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	defer ov.Close()

	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)
	time.Sleep(50 * time.Millisecond)
	if ov.Visible() {
		t.Error("nobody asked for this overlay to be on the screen")
	}
}

// A grabbed combination has to arrive as an f4 key event, which is the whole
// point of taking it from the X server instead of from the terminal.
func TestGrabbedKeyArrives(t *testing.T) {
	f := newXFixture(t)
	if err := xtest.Init(f.conn); err != nil {
		t.Skipf("no XTEST on this server: %v", err)
	}
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	// XK_F5, which no terminal has trouble with, but which is a keysym like
	// any other as far as the grab is concerned.
	const xkF5 = 0xFFC2
	if err := s.GrabKeys([]Combo{{Keysym: xkF5}}); err != nil {
		t.Fatalf("grab: %v", err)
	}
	defer s.UngrabKeys()

	code, err := s.keycodesFor([]Combo{{Keysym: xkF5}})
	if err != nil || len(code) == 0 || code[0] == 0 {
		t.Skipf("this keyboard map has no F5: %v", err)
	}

	xtest.FakeInput(f.conn, xproto.KeyPress, byte(code[0]), 0, f.root, 0, 0, 0)
	xtest.FakeInput(f.conn, xproto.KeyRelease, byte(code[0]), 0, f.root, 0, 0, 0)
	f.conn.Sync()

	select {
	case ev := <-s.Keys():
		if ev == nil || !ev.KeyDown {
			t.Fatalf("expected a key down, got %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the grabbed key never arrived")
	}
}

// A grab is shared state on the X server. Holding one while somebody else is
// typing takes that key away from the whole desktop, so it has to go the
// moment the terminal loses the focus.
func TestGrabsAreDroppedWithTheFocus(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	const xkF5 = 0xFFC2
	if err := s.GrabKeys([]Combo{{Keysym: xkF5}}); err != nil {
		t.Fatalf("grab: %v", err)
	}
	defer s.UngrabKeys()
	waitFor(t, "the grab to be taken", func() bool { return s.grabsHeld() })

	f.focus(t, f.root)
	waitFor(t, "the grab to be given back", func() bool { return !s.grabsHeld() })

	f.focus(t, f.term)
	waitFor(t, "the grab to be taken again", func() bool { return s.grabsHeld() })
}

func TestGrabKeysWithoutADisplay(t *testing.T) {
	var s Session
	if err := s.GrabKeys(nil); err != ErrNoDisplay {
		t.Errorf("a session with no connection cannot grab: %v", err)
	}
}

// A grid of thumbnails is one window with the gaps cut out of it, so that the
// captions between the tiles stay the terminal's.
func TestOverlayBoundsCutGaps(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	ov, err := s.NewOverlay()
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	defer ov.Close()
	if !ov.PassesInput() {
		t.Skip("no SHAPE extension on this server")
	}
	if err := ov.Place(Rect{X: 100, Y: 100, W: 40, H: 40}); err != nil {
		t.Fatalf("place: %v", err)
	}

	if !ov.SetBounds([]Rect{{X: 0, Y: 0, W: 10, H: 10}, {X: 20, Y: 20, W: 10, H: 10}}) {
		t.Fatal("the bounding shape was refused")
	}
	// And back to the whole window, which is what a single picture wants.
	if !ov.SetBounds(nil) {
		t.Error("an empty set must restore the whole window")
	}
}

// The X server reports a focus change whenever a grab begins or ends. Taking
// those at face value meant every grabbed key handed the grabs straight back,
// and the next press of that combination went to the terminal instead — which
// looks exactly like a key that was never grabbed.
func TestFocusEventsFromGrabsAreIgnored(t *testing.T) {
	cases := []struct {
		name   string
		mode   byte
		detail byte
		real   bool
	}{
		{"a window really lost the focus", xproto.NotifyModeNormal, xproto.NotifyDetailNonlinear, true},
		{"the pointer moved while the focus stayed", xproto.NotifyModeNormal, xproto.NotifyDetailPointer, true},
		{"a grab began", xproto.NotifyModeGrab, xproto.NotifyDetailNonlinear, false},
		{"a grab ended", xproto.NotifyModeUngrab, xproto.NotifyDetailNonlinear, false},
		{"the focus went to a child of ours", xproto.NotifyModeNormal, xproto.NotifyDetailInferior, false},
	}
	for _, c := range cases {
		if got := focusEventIsReal(c.mode, c.detail); got != c.real {
			t.Errorf("%s: got %v, want %v", c.name, got, c.real)
		}
	}
}

// A grabbed key must not cost the grabs. This drives the whole path: grab,
// synthesise the key, and check the grabs are still held afterwards.
func TestGrabsSurviveTheirOwnKey(t *testing.T) {
	f := newXFixture(t)
	if err := xtest.Init(f.conn); err != nil {
		t.Skipf("no XTEST on this server: %v", err)
	}
	s := f.openOn(t)
	f.focus(t, f.term)
	waitFor(t, "the focus", s.Focused)

	const xkF5 = 0xFFC2
	if err := s.GrabKeys([]Combo{{Keysym: xkF5}}); err != nil {
		t.Fatalf("grab: %v", err)
	}
	defer s.UngrabKeys()
	waitFor(t, "the grab to be taken", s.grabsHeld)

	code, err := s.keycodesFor([]Combo{{Keysym: xkF5}})
	if err != nil || len(code) == 0 || code[0] == 0 {
		t.Skipf("this keyboard map has no F5: %v", err)
	}
	for i := 0; i < 3; i++ {
		xtest.FakeInput(f.conn, xproto.KeyPress, byte(code[0]), 0, f.root, 0, 0, 0)
		xtest.FakeInput(f.conn, xproto.KeyRelease, byte(code[0]), 0, f.root, 0, 0, 0)
	}
	f.conn.Sync()

	// Three presses, three arrivals: a grab that had been handed back would
	// deliver the first and lose the rest.
	for i := 0; i < 3; i++ {
		select {
		case <-s.Keys():
		case <-time.After(3 * time.Second):
			t.Fatalf("press %d never arrived; grabs held: %v", i+1, s.grabsHeld())
		}
	}
	if !s.grabsHeld() {
		t.Error("the grabs must still be held after their own keys")
	}
	if !s.Focused() {
		t.Error("a grab is not a loss of focus")
	}
}

// Where a terminal draws its grid into a window of its own, the X server knows
// exactly where that is and nothing has to be worked out from the frame.
func TestInnerWindowFound(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)

	// A child the size of the text area, inset the way a menu bar insets it.
	child, err := xproto.NewWindowId(f.conn)
	if err != nil {
		t.Fatalf("window id: %v", err)
	}
	err = xproto.CreateWindowChecked(f.conn, 0, child, f.term,
		0, 40, 390, 250, 0, xproto.WindowClassInputOnly,
		0, 0, nil).Check()
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	defer xproto.DestroyWindow(f.conn, child)
	if err := xproto.MapWindowChecked(f.conn, child).Check(); err != nil {
		t.Fatalf("map: %v", err)
	}

	// The fixture window is at 40,60 and 400x300, so the child is at 40,100.
	got, ok := s.InnerWindow(Rect{X: 40, Y: 100, W: 392, H: 252}, 12)
	if !ok {
		t.Fatal("a child of about the right size must be found")
	}
	want := Rect{X: 40, Y: 100, W: 390, H: 250}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// A child that is nothing like the expected size is not the text area, and
// believing it would put the picture over some other part of the screen.
func TestInnerWindowRefusesTheWrongChild(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)

	child, err := xproto.NewWindowId(f.conn)
	if err != nil {
		t.Fatalf("window id: %v", err)
	}
	err = xproto.CreateWindowChecked(f.conn, 0, child, f.term,
		0, 0, 20, 20, 0, xproto.WindowClassInputOnly, 0, 0, nil).Check()
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	defer xproto.DestroyWindow(f.conn, child)
	xproto.MapWindow(f.conn, child)

	if _, ok := s.InnerWindow(Rect{X: 40, Y: 60, W: 400, H: 300}, 12); ok {
		t.Error("a scrollbar is not a text area")
	}
}

// And a terminal with no child window at all keeps whatever was worked out
// from the frame.
func TestInnerWindowWithNoChildren(t *testing.T) {
	f := newXFixture(t)
	s := f.openOn(t)
	if _, ok := s.InnerWindow(Rect{X: 40, Y: 60, W: 400, H: 300}, 12); ok {
		t.Error("there is no child to find")
	}
}
