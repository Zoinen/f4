// Package ttyx gives a terminal session the two things a terminal itself
// cannot: the identity of the window it is running inside, and a way to put
// pixels on the screen outside the character grid.
//
// far2l has the same idea under the name TTY|Xi, where a broker process holds
// a connection to the X server and hands the terminal back the key
// combinations and the clipboard that a TTY cannot carry. This package is the
// part both of those need first — the connection, the window and the focus —
// plus the overlay window that lets a picture be shown over a terminal which
// knows no image protocol at all.
//
// There is no cgo here and there will not be. Everything goes through XGB,
// which speaks the X protocol over a socket in plain Go, and which vtui's own
// X11 backend already depends on.
package ttyx

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

var (
	// ErrNoDisplay means there is no X session to talk to at all.
	ErrNoDisplay = errors.New("there is no X display")

	// ErrNoTerminal means the X server is there but nothing in it could be
	// identified as the window we are running in.
	ErrNoTerminal = errors.New("the terminal window could not be identified")
)

// Source records how the terminal window was found, which is the same thing
// as how far it can be trusted. A caller that is about to draw over the
// window should care: getting it wrong means painting over somebody else's
// application.
type Source int

const (
	// SourceNone means it was not found.
	SourceNone Source = iota

	// SourceWindowID means the terminal published its own window id in the
	// environment. xterm, urxvt, konsole and a few others do. This one is
	// not a guess.
	SourceWindowID

	// SourceProcess means a window on the screen names one of our own
	// ancestors in _NET_WM_PID. Also not a guess, and it is the one that
	// covers the terminals which do not set the environment variable.
	SourceProcess

	// SourceActive means we fell back to whichever window had the focus
	// when we looked. It is right most of the time and wrong exactly when
	// the user was doing something else at the moment f4 started.
	SourceActive
)

func (s Source) String() string {
	switch s {
	case SourceWindowID:
		return "WINDOWID"
	case SourceProcess:
		return "_NET_WM_PID"
	case SourceActive:
		return "_NET_ACTIVE_WINDOW"
	}
	return "none"
}

// Trusted reports whether the window was identified rather than guessed.
func (s Source) Trusted() bool {
	return s == SourceWindowID || s == SourceProcess
}

// Rect is a rectangle in screen pixels.
type Rect struct{ X, Y, W, H int }

// Session is a connection to the X server together with the window of the
// terminal emulator f4 is running in.
type Session struct {
	mu     sync.Mutex
	conn   *xgb.Conn
	root   xproto.Window
	depth  byte
	visual xproto.Visualid

	win    xproto.Window
	source Source

	atoms map[string]xproto.Atom

	// State kept up to date by the event loop, so that asking is free and
	// so that something can be told when it changes.
	alive   bool
	focused bool
	geom    Rect
	geomOK  bool
	changed chan struct{}

	overlays []*Overlay

	keys *keyState
}

// Open connects to the X server and works out which window we are in. It
// fails, rather than guessing wildly, when there is no display or when
// nothing on the display can be our terminal.
func Open() (*Session, error) {
	return open(os.Getenv, processAncestors())
}

func open(env func(string) string, ancestors []uint32) (*Session, error) {
	if strings.TrimSpace(env("DISPLAY")) == "" {
		return nil, ErrNoDisplay
	}
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("no connection to the X server: %w", err)
	}
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)
	s := &Session{
		conn:    conn,
		root:    screen.Root,
		depth:   screen.RootDepth,
		visual:  screen.RootVisual,
		atoms:   make(map[string]xproto.Atom),
		alive:   true,
		changed: make(chan struct{}, 1),
	}

	s.win, s.source = s.locate(env, ancestors)
	if s.source == SourceNone {
		conn.Close()
		return nil, ErrNoTerminal
	}

	s.watch()
	return s, nil
}

// Close drops the connection. Everything created from the session goes with
// it, so an overlay does not have to be destroyed separately.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
		s.alive = false
	}
}

// Window is the terminal window, and Source says how it was found.
func (s *Session) Window() xproto.Window { return s.win }

// Source says how the terminal window was found.
func (s *Session) Source() Source { return s.source }

// Geometry reports where the terminal window is on the screen. The size is
// the window's own, so it includes whatever padding the terminal leaves
// around its character grid and excludes the decoration the window manager
// puts around it.
//
// The event loop keeps this up to date, so it is a read of two integers and
// not a round trip to the server; the round trip only happens before the
// first ConfigureNotify has arrived.
func (s *Session) Geometry() (Rect, error) {
	s.mu.Lock()
	if s.geomOK {
		r := s.geom
		s.mu.Unlock()
		return r, nil
	}
	s.mu.Unlock()
	return s.geometryNow()
}

// geometryNow asks the server rather than the cache.
func (s *Session) geometryNow() (Rect, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return Rect{}, ErrNoDisplay
	}

	geo, err := xproto.GetGeometry(s.conn, xproto.Drawable(s.win)).Reply()
	if err != nil || geo == nil {
		return Rect{}, fmt.Errorf("the terminal window has no geometry: %w", err)
	}
	// The geometry is relative to the parent, which under a window manager
	// is the frame and not the root, so it has to be translated.
	pos, err := xproto.TranslateCoordinates(s.conn, s.win, s.root, 0, 0).Reply()
	if err != nil || pos == nil {
		return Rect{}, fmt.Errorf("the terminal window has no position: %w", err)
	}
	return Rect{
		X: int(pos.DstX),
		Y: int(pos.DstY),
		W: int(geo.Width),
		H: int(geo.Height),
	}, nil
}

// Focused reports whether the keyboard focus is on the terminal window or on
// something inside it.
//
// It walks up from the focus rather than reading _NET_ACTIVE_WINDOW because
// the focus is a property of the X server and the other one is a convention
// a window manager may or may not follow — and because the window a terminal
// puts its focus on is often a child of the one it registered with the window
// manager, which the walk handles and a comparison would not.
// The event loop keeps the answer up to date, so this costs nothing to ask
// and can be asked on every frame.
func (s *Session) Focused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive && s.focused
}

// focusedNow asks the server rather than the cache. The caller holds no lock.
func (s *Session) focusedNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return false
	}

	reply, err := xproto.GetInputFocus(s.conn).Reply()
	if err != nil || reply == nil {
		return false
	}
	// None and PointerRoot are not windows.
	if reply.Focus <= 1 {
		return false
	}

	w := reply.Focus
	for i := 0; i < 32; i++ {
		if w == s.win {
			return true
		}
		if w == s.root || w == 0 {
			return false
		}
		tree, err := xproto.QueryTree(s.conn, w).Reply()
		if err != nil || tree == nil {
			return false
		}
		w = tree.Parent
	}
	return false
}

// atom interns a name once and remembers it. The caller holds no lock: the
// map is only touched from the locked methods and from open.
func (s *Session) atom(name string) xproto.Atom {
	if a, ok := s.atoms[name]; ok {
		return a
	}
	a := xproto.Atom(0)
	if len(name) > math.MaxUint16 {
		return a
	}
	// The atom is created if it is not there. Refusing to create one was
	// the first version and it was wrong: CLIPBOARD does not exist on a
	// server nobody has copied anything on yet, and a selection cannot be
	// taken by a name that has no atom. Interning a name that is never used
	// costs one entry in a table; the alternative was a clipboard that
	// worked everywhere except on a fresh session.
	// #nosec G115 -- atom names longer than the uint16 wire limit are rejected above.
	if reply, err := xproto.InternAtom(s.conn, false, uint16(len(name)), name).Reply(); err == nil && reply != nil {
		a = reply.Atom
	}
	s.atoms[name] = a
	return a
}

// property32 reads a property made of 32 bit values, which is what every
// property this package cares about is.
func (s *Session) property32(win xproto.Window, name string) []uint32 {
	a := s.atom(name)
	if a == 0 {
		return nil
	}
	reply, err := xproto.GetProperty(s.conn, false, win, a, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	if err != nil || reply == nil || reply.Format != 32 {
		return nil
	}
	out := make([]uint32, 0, len(reply.Value)/4)
	for i := 0; i+4 <= len(reply.Value); i += 4 {
		out = append(out, xgb.Get32(reply.Value[i:]))
	}
	return out
}

// InnerWindow looks for a child window of the terminal that is the text area
// itself, and returns where it is on the screen.
//
// Some terminals draw their grid into a window of their own, in which case
// the X server knows exactly where it is and nothing has to be worked out
// from the size of the frame around it. Many do not — a GTK application
// usually draws every widget into the one window of the toplevel — so this
// answers false as often as not.
//
// A candidate is only believed when it is already about the size the caller
// expected. Getting this wrong means drawing over the wrong part of the
// screen, and a tolerance of a few pixels is enough for the thing it is meant
// to find while being far too tight for anything else in the tree.
func (s *Session) InnerWindow(want Rect, tolerance int) (Rect, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return Rect{}, false
	}

	near := func(a, b int) bool {
		d := a - b
		if d < 0 {
			d = -d
		}
		return d <= tolerance
	}

	// Two levels are enough for the shapes this occurs in, and a walk of
	// the whole tree would be a walk of the whole desktop.
	parents := []xproto.Window{s.win}
	for depth := 0; depth < 2; depth++ {
		var next []xproto.Window
		for _, parent := range parents {
			tree, err := xproto.QueryTree(s.conn, parent).Reply()
			if err != nil || tree == nil {
				continue
			}
			for _, child := range tree.Children {
				geo, err := xproto.GetGeometry(s.conn, xproto.Drawable(child)).Reply()
				if err != nil || geo == nil {
					continue
				}
				if near(int(geo.Width), want.W) && near(int(geo.Height), want.H) {
					pos, err := xproto.TranslateCoordinates(s.conn, child, s.root, 0, 0).Reply()
					if err != nil || pos == nil {
						continue
					}
					return Rect{
						X: int(pos.DstX), Y: int(pos.DstY),
						W: int(geo.Width), H: int(geo.Height),
					}, true
				}
				next = append(next, child)
			}
		}
		parents = next
	}
	return Rect{}, false
}

func (s *Session) exists(w xproto.Window) bool {
	geo, err := xproto.GetGeometry(s.conn, xproto.Drawable(w)).Reply()
	return err == nil && geo != nil
}

// locate finds the terminal window, cheapest and most trustworthy first.
func (s *Session) locate(env func(string) string, ancestors []uint32) (xproto.Window, Source) {
	if w, ok := windowIDFromEnv(env); ok && s.exists(w) {
		return w, SourceWindowID
	}
	if w, ok := s.byProcess(ancestors); ok {
		return w, SourceProcess
	}
	if w, ok := s.activeWindow(); ok {
		return w, SourceActive
	}
	return 0, SourceNone
}

// windowIDFromEnv reads the window id a terminal published for its children.
func windowIDFromEnv(env func(string) string) (xproto.Window, bool) {
	for _, name := range []string{"WINDOWID", "XTERM_WINDOWID"} {
		v := strings.TrimSpace(env(name))
		if v == "" {
			continue
		}
		// Base zero, because some terminals write it in hexadecimal.
		n, err := strconv.ParseUint(v, 0, 32)
		if err != nil || n == 0 {
			continue
		}
		return xproto.Window(n), true
	}
	return 0, false
}

// byProcess looks for a window that names one of our ancestors. It covers the
// terminals that publish no window id, including the ones where the window
// belongs to a server process shared by every tab: that process is still an
// ancestor of the shell, and therefore of us.
//
// A server process owns one window per tab, so when several match, the one
// that has the focus wins. Nothing better is available: the tabs are
// indistinguishable from outside.
func (s *Session) byProcess(ancestors []uint32) (xproto.Window, bool) {
	if len(ancestors) == 0 {
		return 0, false
	}
	list := s.property32(s.root, "_NET_CLIENT_LIST")
	if len(list) == 0 {
		return 0, false
	}
	active, _ := s.activeWindow()

	var first xproto.Window
	found := false
	for _, id := range list {
		w := xproto.Window(id)
		pids := s.property32(w, "_NET_WM_PID")
		if len(pids) == 0 || !containsPID(ancestors, pids[0]) {
			continue
		}
		if w == active {
			return w, true
		}
		if !found {
			first, found = w, true
		}
	}
	return first, found
}

func (s *Session) activeWindow() (xproto.Window, bool) {
	v := s.property32(s.root, "_NET_ACTIVE_WINDOW")
	if len(v) == 0 || v[0] == 0 {
		return 0, false
	}
	w := xproto.Window(v[0])
	if !s.exists(w) {
		return 0, false
	}
	return w, true
}

func containsPID(list []uint32, pid uint32) bool {
	for _, v := range list {
		if v == pid {
			return true
		}
	}
	return false
}

// processAncestors lists this process and the ones it descends from, which is
// what a terminal's _NET_WM_PID is compared against. It reads /proc, so it
// finds nothing where there is none — in which case the search simply falls
// through to the next way of identifying the window.
func processAncestors() []uint32 {
	return ancestorPIDs(os.Getpid(), parentPID)
}

func ancestorPIDs(pid int, parent func(int) (int, bool)) []uint32 {
	var out []uint32
	seen := make(map[int]bool)
	for i := 0; i < 32 && pid > 0 && !seen[pid]; i++ {
		if uint64(pid) > math.MaxUint32 {
			break
		}
		seen[pid] = true
		// #nosec G115 -- positive pid was checked against MaxUint32 immediately above.
		out = append(out, uint32(pid))
		next, ok := parent(pid)
		if !ok {
			break
		}
		pid = next
	}
	return out
}

func parentPID(pid int) (int, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		rest, ok := strings.CutPrefix(line, "PPid:")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
