//go:build linux

// Command xdndtarget is a bare XDND drop target for the Wine/X11
// drag-and-drop acceptance check (issue #566).
//
// It opens one XdndAware window on the X server named by $DISPLAY, accepts
// every position as a copy, asks the source for text/uri-list when the drop
// arrives, prints and optionally stores what it got, and answers
// XdndFinished. The Wine job runs it next to a Wine-hosted f4 on the same
// Xvfb and drags a file out of f4 onto it; the exit code is the verdict:
//
//	0  the drop arrived (and matched -want-path, when given)
//	1  the drop arrived but carried a different list
//	2  no drop within -timeout, or the X server was unusable
//
// "READY" is printed on its own line once the window is mapped, so a driver
// knows when the target can be dragged onto.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/unxed/vtui"
)

const (
	exitOK       = 0
	exitMismatch = 1
	exitFailure  = 2

	maxCoord = 1<<15 - 1
)

type config struct {
	x, y     int16
	w, h     uint16
	timeout  time.Duration
	wantPath string // the one path the drop must carry, empty = any
	out      string // file the received list is written to, empty = none
}

func main() {
	var (
		x       = flag.Int("x", 10, "window left edge")
		y       = flag.Int("y", 10, "window top edge")
		w       = flag.Int("w", 200, "window width")
		h       = flag.Int("h", 200, "window height")
		timeout = flag.Duration("timeout", 30*time.Second, "give up after this long without a drop")
		want    = flag.String("want-path", "", "path the drop must carry (checked as a text/uri-list)")
		out     = flag.String("out", "", "file to store the received text/uri-list in")
	)
	flag.Parse()
	for _, v := range []int{*x, *y, *w, *h} {
		if v < 0 || v > maxCoord {
			fmt.Fprintln(os.Stderr, "xdndtarget: geometry out of range")
			os.Exit(exitFailure)
		}
	}
	cfg := config{
		x: int16(*x), y: int16(*y), //nolint:gosec // range checked above
		w: uint16(*w), h: uint16(*h), //nolint:gosec // range checked above
		timeout: *timeout, wantPath: *want, out: *out,
	}
	os.Exit(run(cfg, os.Stdout, os.Stderr))
}

// say writes a diagnostic line; a closed output must not change the verdict.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// run serves one drop and returns the process exit code.
func run(cfg config, ready, errw io.Writer) int {
	conn, err := xgb.NewConn()
	if err != nil {
		say(errw, "xdndtarget: no X server: %v\n", err)
		return exitFailure
	}
	defer conn.Close()

	t, err := newTarget(conn, cfg)
	if err != nil {
		say(errw, "xdndtarget: %v\n", err)
		return exitFailure
	}
	say(ready, "READY\n")

	got, err := t.waitDrop(cfg.timeout)
	if err != nil {
		say(errw, "xdndtarget: %v\n", err)
		return exitFailure
	}
	say(ready, "DROP %q\n", got)
	if cfg.out != "" {
		if err := os.WriteFile(cfg.out, []byte(got), 0o600); err != nil {
			say(errw, "xdndtarget: %v\n", err)
			return exitFailure
		}
	}
	if cfg.wantPath != "" {
		if want := vtui.FormatURIList([]string{cfg.wantPath}); got != want {
			say(errw, "xdndtarget: got %q, want %q\n", got, want)
			return exitMismatch
		}
	}
	return exitOK
}

type target struct {
	conn   *xgb.Conn
	window xproto.Window

	position, status, drop, finished, selection, uriList, transfer, copyAct xproto.Atom
}

func newTarget(conn *xgb.Conn, cfg config) (*target, error) {
	t := &target{conn: conn}
	names := []struct {
		dst  *xproto.Atom
		name string
	}{
		{&t.position, "XdndPosition"}, {&t.status, "XdndStatus"}, {&t.drop, "XdndDrop"},
		{&t.finished, "XdndFinished"}, {&t.selection, "XdndSelection"},
		{&t.uriList, "text/uri-list"}, {&t.transfer, "F4_XDNDTARGET_TRANSFER"},
		{&t.copyAct, "XdndActionCopy"},
	}
	for _, n := range names {
		a, err := intern(conn, n.name)
		if err != nil {
			return nil, err
		}
		*n.dst = a
	}
	aware, err := intern(conn, "XdndAware")
	if err != nil {
		return nil, err
	}

	screen := xproto.Setup(conn).DefaultScreen(conn)
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		cfg.x, cfg.y, cfg.w, cfg.h, 0, xproto.WindowClassInputOutput, screen.RootVisual, 0, nil).Check(); err != nil {
		return nil, fmt.Errorf("create window: %w", err)
	}
	title := "f4 xdnd target"
	if err := xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, wid, xproto.AtomWmName,
		xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check(); err != nil { //nolint:gosec // short literal
		return nil, fmt.Errorf("set title: %w", err)
	}
	version := make([]byte, 4)
	xgb.Put32(version, 5)
	if err := xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, wid, aware,
		xproto.AtomAtom, 32, 1, version).Check(); err != nil {
		return nil, fmt.Errorf("set XdndAware: %w", err)
	}
	if err := xproto.MapWindowChecked(conn, wid).Check(); err != nil {
		return nil, fmt.Errorf("map window: %w", err)
	}
	t.window = wid
	return t, nil
}

func intern(conn *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply() //nolint:gosec // the names are short literals
	if err != nil {
		return 0, fmt.Errorf("intern %s: %w", name, err)
	}
	return reply.Atom, nil
}

func (t *target) send(to xproto.Window, typ xproto.Atom, data [5]uint32) {
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: to,
		Type:   typ,
		Data:   xproto.ClientMessageDataUnionData32New(data[:]),
	}
	xproto.SendEvent(t.conn, false, to, 0, string(ev.Bytes()))
}

// waitDrop answers the source's positions until a drop arrives and returns
// the text/uri-list it carried.
func (t *target) waitDrop(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var source xproto.Window
	for time.Now().Before(deadline) {
		ev, err := t.conn.PollForEvent()
		if err != nil {
			continue // a protocol error for one request, not a dead connection
		}
		switch e := ev.(type) {
		case nil:
			time.Sleep(2 * time.Millisecond)
		case xproto.ClientMessageEvent:
			switch e.Type {
			case t.position:
				source = xproto.Window(e.Data.Data32[0])
				t.send(source, t.status, [5]uint32{uint32(t.window), 1, 0, 0, uint32(t.copyAct)})
			case t.drop:
				source = xproto.Window(e.Data.Data32[0])
				xproto.ConvertSelection(t.conn, t.window, t.selection, t.uriList, t.transfer,
					xproto.Timestamp(e.Data.Data32[2]))
			}
		case xproto.SelectionNotifyEvent:
			got := ""
			if reply, err := xproto.GetProperty(t.conn, true, t.window, t.transfer, xproto.AtomAny, 0, 1024).Reply(); err == nil && reply != nil {
				got = string(reply.Value)
			}
			ok := uint32(0)
			if got != "" {
				ok = 1
			}
			t.send(source, t.finished, [5]uint32{uint32(t.window), ok, uint32(t.copyAct)})
			// The finish message is queued on the connection; make the server
			// read it before the process exits.
			_, _ = xproto.GetInputFocus(t.conn).Reply()
			return got, nil
		}
	}
	return "", errors.New("no drop before the timeout")
}
