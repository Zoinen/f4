package ttyx

// Everything here targets branches that neither the "no display at all" tests
// (openfor_test.go, overlay_lifecycle_test.go, session_state_test.go, ...) nor
// the real-X-server tests (ttyx_test.go, ttyx_events_test.go) reach: a
// Session/Overlay that carries a non-nil *xgb.Conn which was never actually
// connected to a server. Every case here is chosen so that the code returns
// before it ever puts a request on the wire -- xgb.Conn.Close and any request
// send both block forever on such a connection (they select on unbuffered,
// never-started channels), so the one rule that matters while adding a test
// here is: never call anything that would reach past the early-return this
// test means to exercise.

import (
	"errors"
	"math"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// IdentityEnv lists exactly the variables a terminal's window identity is
// carried in, in the order OpenFor's caller is expected to read them.
func TestIdentityEnv(t *testing.T) {
	got := IdentityEnv()
	want := []string{"DISPLAY", "WINDOWID", "XTERM_WINDOWID"}
	if len(got) != len(want) {
		t.Fatalf("IdentityEnv() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IdentityEnv() = %v, want %v", got, want)
		}
	}
}

func TestSessionDisplayAccessor(t *testing.T) {
	s := &Session{display: ":42"}
	if got := s.Display(); got != ":42" {
		t.Fatalf("Display() = %q, want %q", got, ":42")
	}
}

// Open is Open() proper, not the lower-case open() every other test drives
// directly -- it has never been called by name anywhere else in this
// package, so nothing before this test exercised its own body at all.
func TestOpenWithoutDisplayThroughExportedOpen(t *testing.T) {
	t.Setenv("DISPLAY", "")
	if _, err := Open(); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("Open() with no DISPLAY = %v, want %v", err, ErrNoDisplay)
	}
}

// A DISPLAY that names nothing reachable must fail at the connection step,
// not at the "no DISPLAY at all" step or the "no terminal found" step. The
// socket path below cannot exist, so xgb.NewConnDisplay fails immediately
// with ECONNREFUSED-flavoured "no such file or directory", never touching a
// real server and never blocking.
func TestOpenFailsToConnect(t *testing.T) {
	env := map[string]string{"DISPLAY": "/tmp/lunobot-nonexistent-x11-socket-for-tests:0"}
	_, err := open(func(k string) string { return env[k] }, nil)
	if err == nil {
		t.Fatal("a display naming no reachable socket must fail to connect")
	}
	if errors.Is(err, ErrNoDisplay) || errors.Is(err, ErrNoTerminal) {
		t.Fatalf("expected a connection error, got %v", err)
	}
}

func TestKeycodesForWithoutDisplay(t *testing.T) {
	var s Session
	if _, err := s.keycodesFor(nil); err != ErrNoDisplay {
		t.Fatalf("keycodesFor without a display: got %v, want %v", err, ErrNoDisplay)
	}
}

// regrabKeys is only ever reached, in every other test in this package,
// through setFocused(true) on a Session whose keys are nil -- the "keys were
// configured, but there is no connection to grab them on" branch has no
// other test at all.
func TestRegrabKeysWithoutKeyState(t *testing.T) {
	var s Session
	s.regrabKeys()
}

func TestRegrabKeysWithoutDisplay(t *testing.T) {
	s := &Session{keys: &keyState{combos: []Combo{{Keysym: 1}}, codes: []xproto.Keycode{2}}}
	s.regrabKeys()
	if s.keys.held {
		t.Fatal("regrabKeys without a display must not mark the grabs as held")
	}
}

func TestOverlayPassesInputWhenShaped(t *testing.T) {
	var s Session
	o := &Overlay{s: &s, shaped: true}
	if !o.PassesInput() {
		t.Fatal("a shaped overlay must report that input passes through")
	}
}

// SetBounds refuses an overlay the server never confirmed a shape for, and
// it must do so before it touches the connection: only the "no connection at
// all" half of that same guard had a test before this one.
func TestOverlaySetBoundsRequiresShaping(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s}
	if o.SetBounds([]Rect{{W: 1, H: 1}}) {
		t.Fatal("SetBounds must fail when the overlay was never shaped")
	}
}

// Suspend on an overlay that is not mapped has to return without unmapping
// anything, whether or not there happens to be a connection.
func TestOverlaySuspendReturnsWhenNotMapped(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s}
	o.Suspend()
	if o.Mapped {
		t.Fatal("Suspend must leave an unmapped overlay unmapped")
	}
}

// Place with an empty rectangle hides the overlay instead of asking the
// server for a zero sized window. hide() returns before it unmaps anything
// because the overlay was never mapped, so this never reaches the
// connection either.
func TestOverlayPlaceWithEmptyRectHidesWithoutTouchingTheServer(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s, wanted: true}
	if err := o.Place(Rect{}); err != nil {
		t.Fatalf("an empty rectangle must be accepted: %v", err)
	}
	if o.wanted {
		t.Fatal("Place with an empty rectangle must clear wanted")
	}
}

// A rectangle wide enough to overflow the X11 wire format must be rejected
// before Place ever builds a ConfigureWindow request.
func TestOverlayPlaceRejectsOutOfRangeRectangleWithoutTouchingTheServer(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s}
	r := Rect{X: 0, Y: 0, W: math.MaxUint16 + 1, H: 10}
	if err := o.Place(r); err == nil {
		t.Fatal("a rectangle wider than the X11 wire range must be rejected")
	}
}

// Draw validates the picture it was handed before it ever asks the
// connection for its maximum request length, so every one of these must be
// refused without a working connection.
func TestOverlayDrawRejectsBadDimensionsWithoutTouchingTheServer(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s}
	cases := []struct {
		name string
		w, h int
	}{
		{"zero width", 0, 4},
		{"zero height", 4, 0},
		{"width beyond the X11 wire range", math.MaxUint16 + 1, 4},
		{"height beyond the X11 wire range", 4, math.MaxInt16 + 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := o.Draw(nil, c.w, c.h, 4); err == nil {
				t.Fatalf("Draw(%d, %d) must be rejected before it touches the connection", c.w, c.h)
			}
		})
	}
}

func TestOverlayDrawRejectsMismatchedBufferWithoutTouchingTheServer(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s}
	cases := []struct {
		name         string
		pix          []byte
		w, h, stride int
	}{
		{"buffer shorter than one row", make([]byte, 8), 4, 4, 16},
		{"stride narrower than the pixels", make([]byte, 64), 4, 4, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := o.Draw(c.pix, c.w, c.h, c.stride); err == nil {
				t.Fatalf("Draw with %s must be rejected before it touches the connection", c.name)
			}
		})
	}
}
