package ttyx

import (
	"os"
	"testing"

	"github.com/jezek/xgb/xproto"
)

// OpenFor reads the display from the environment it is given, not from the
// process's own: for a session daemon the two are different terminals.
func TestOpenForNeedsTheClientsDisplay(t *testing.T) {
	if _, err := OpenFor(os.Getpid(), func(string) string { return "" }); err != ErrNoDisplay {
		t.Fatalf("OpenFor with no DISPLAY in the given environment: got %v, want ErrNoDisplay", err)
	}
}

// OpenFor walks the ancestry of the pid it is given. A window owned by that
// pid is found through _NET_WM_PID, which is what a client attaching to a
// daemon from a terminal that publishes no WINDOWID — GNOME Terminal — needs.
func TestOpenForFindsTheWindowOfTheGivenProcess(t *testing.T) {
	f := newXFixture(t)
	f.setWindowProp(t, f.term, "_NET_WM_PID", xproto.AtomCardinal, uint32(os.Getppid()))
	f.setWindowProp(t, f.root, "_NET_CLIENT_LIST", xproto.AtomWindow, uint32(f.term))
	t.Cleanup(func() {
		xproto.DeleteProperty(f.conn, f.root, f.intern(t, "_NET_CLIENT_LIST"))
	})

	env := map[string]string{"DISPLAY": os.Getenv("DISPLAY")}
	s, err := OpenFor(os.Getpid(), func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("OpenFor: %v", err)
	}
	defer s.Close()
	if s.Source() != SourceProcess || s.Window() != f.term {
		t.Errorf("got window %d from %v, want %d from _NET_WM_PID", s.Window(), s.Source(), f.term)
	}
	if s.Display() != env["DISPLAY"] {
		t.Errorf("Display() = %q, want %q", s.Display(), env["DISPLAY"])
	}
}
