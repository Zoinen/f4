package winex11drag

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"time"

	"github.com/jezek/xgb"
	winescape "github.com/unxed/libwinescape/go"
	"github.com/unxed/vtui"

	"github.com/unxed/f4/vfs/hostmode"
)

// ErrNotWineHost is what OpenWineX11Connection returns when
// vfs/hostmode.Posix() is false: real Windows (no Wine underneath it) and
// every native, non-Wine build -- every platform this bootstrap is not for
// (issue #566). hostmode.Posix() never changes after its first call in a
// process, so this is a permanent answer once given.
var ErrNotWineHost = errors.New("winex11drag: not running under Wine with libwinescape (hostmode.Posix() is false)")

// OpenWineX11Connection opens an *xgb.Conn to the host's real X server from
// inside a Wine process (issue #566, step 2). Wine's winsock cannot reach an
// AF_UNIX socket on the Linux side, so both the socket and the Xauthority
// file are read through libwinescape rather than net.Dial/os.Open, which
// would only ever see Wine's own, unrelated, Win32 world.
//
// The vendored github.com/jezek/xgb already performs the X11 setup request
// and its authentication handshake once handed a net.Conn and a hex-encoded
// MIT-MAGIC-COOKIE-1 cookie (NewConnNetWithCookieHex): this function's own
// job is only the bootstrap xgb cannot do from inside Wine on its own --
// finding the right socket and the right cookie, both on the host side.
//
// This is the integration point step 2 cannot unit test on its own: every
// call it makes -- HostGetenv, DialUnix, the file read -- is a real syscall
// through libwinescape, so it needs a live Wine process with a real DISPLAY
// and a real X server to exercise end to end. ParseDisplay and
// ParseXauthority, which do all of this function's actual parsing, are
// covered by this package's other tests without either.
//
// It is unreachable outside that environment: hostmode.Posix() is false on
// real Windows and on every native build, per vfs/hostmode's own contract,
// and this function's first line asks exactly that -- see
// TestOpenWineX11ConnectionRefusesOutsidePosixMode.
func OpenWineX11Connection() (*xgb.Conn, error) {
	if !hostmode.Posix() {
		return nil, ErrNotWineHost
	}

	display := winescape.HostGetenv("DISPLAY")
	vtui.DebugLog("WINEX11DRAG: DISPLAY=%q", display)
	spec, err := ParseDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("winex11drag: %w", err)
	}
	sockPath, ok := spec.SocketPath()
	if !ok {
		return nil, fmt.Errorf("winex11drag: DISPLAY=%q names host %q; only a local display is supported", display, spec.Hostname)
	}

	cookie, err := hostXauthCookie(spec)
	if err != nil {
		return nil, err
	}
	vtui.DebugLog("WINEX11DRAG: Xauthority cookie read, dialing %s", sockPath)

	sock, err := winescape.DialUnix(sockPath)
	if err != nil {
		return nil, fmt.Errorf("winex11drag: dialing %s: %w", sockPath, err)
	}

	vtui.DebugLog("WINEX11DRAG: socket connected, X11 setup handshake")
	conn, err := xgb.NewConnNetWithCookieHex(wrapWineConn(sock), hex.EncodeToString(cookie))
	if err != nil {
		_ = sock.Close()
		return nil, fmt.Errorf("winex11drag: X11 setup handshake on %s: %w", sockPath, err)
	}
	return conn, nil
}

// hostXauthCookie reads $XAUTHORITY, or $HOME/.Xauthority when it is unset,
// from the host side via libwinescape, and returns the MIT-MAGIC-COOKIE-1
// cookie for spec's display.
func hostXauthCookie(spec DisplaySpec) ([]byte, error) {
	path := winescape.HostGetenv("XAUTHORITY")
	if path == "" {
		home := winescape.HostGetenv("HOME")
		if home == "" {
			return nil, errors.New("winex11drag: neither XAUTHORITY nor HOME is set on the host")
		}
		path = home + "/.Xauthority"
	}

	vtui.DebugLog("WINEX11DRAG: opening the Xauthority file %q", path)
	f, err := winescape.OpenFile(path, winescape.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("winex11drag: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	vtui.DebugLog("WINEX11DRAG: Xauthority file open, parsing")
	entries, err := ParseXauthority(eofReader{f})
	if err != nil {
		return nil, fmt.Errorf("winex11drag: parsing %s: %w", path, err)
	}

	vtui.DebugLog("WINEX11DRAG: Xauthority parsed, %d entries", len(entries))
	cookie, ok := FindMagicCookie(entries, strconv.Itoa(spec.Display))
	if !ok {
		return nil, fmt.Errorf("winex11drag: no MIT-MAGIC-COOKIE-1 entry for display %d in %s", spec.Display, path)
	}
	return cookie, nil
}

// wineConn adapts a *winescape.File -- the socket winescape.DialUnix hands
// back -- to net.Conn, which is what xgb.NewConnNetWithCookieHex needs. Only
// Read, Write and Close (from the embedded *winescape.File) carry real
// behaviour; the rest are stubs, exactly as issue #566's step 2 allows:
// nothing drives the XDND gesture loop yet (that is step 5, future work), so
// there is nothing here that needs a real deadline or a real address.
type wineConn struct {
	*winescape.File
}

// wrapWineConn is the seam between winescape.DialUnix's result and xgb: kept
// as its own function so OpenWineX11Connection reads as "dial, then wrap",
// and so a future caller of DialUnix elsewhere in this package (e.g. step 4's
// selection window) can reuse the same wrapper.
func wrapWineConn(f *winescape.File) net.Conn { return wineConn{File: f} }

// eofReader turns the (0, nil) that winescape.File.Read returns at end of file
// into io.EOF. winescape reports a raw read(2) count and never io.EOF, and
// io.ReadFull -- hence binary.Read and ParseXauthority -- retries a (0, nil)
// read forever, so without this the first read past the last Xauthority record
// hangs the drag (issue #566, Wine probe: the log stopped after "DISPLAY=").
type eofReader struct{ r io.Reader }

func (e eofReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if n == 0 && err == nil && len(p) > 0 {
		return 0, io.EOF
	}
	return n, err
}

// wineReadSlice is how long one poll(2) of the X socket may block. A raw
// libwinescape syscall bypasses the Go runtime's entersyscall/exitsyscall, so a
// thread parked in read(2) keeps its P: the first stop-the-world (a GC cycle
// started by the allocations of the gesture) then waits for that P until the X
// server happens to send something, and the whole process -- the main loop and
// the heartbeat included -- stands still (issue #566: the Wine freeze snapshot
// shows one thread in unix_stream_read_generic and every other thread idle). The
// wait is therefore sliced, and between slices the thread is back in Go code,
// where the scheduler can stop it.
const wineReadSlice = 10 * time.Millisecond

// waitReadable returns once fd has data, a hang-up or an error to report,
// sleeping in poll(2) at most wineReadSlice at a time and yielding between
// slices. A host where poll is unavailable (winescape.ErrUnavailable) returns
// nil at once: the caller then does the plain blocking read it always did.
func waitReadable(fd int, poll func([]winescape.PollFd, time.Duration) (int, error)) error {
	fds := []winescape.PollFd{{Fd: int32(fd), Events: winescape.POLLIN}} //nolint:gosec // a host file descriptor is far below 2^31
	for {
		n, err := poll(fds, wineReadSlice)
		if errors.Is(err, winescape.ErrUnavailable) {
			return nil
		}
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		runtime.Gosched()
	}
}

// Read is the socket read of an X connection: it waits for data in short poll
// slices (see wineReadSlice), then reads. A (0, nil) from read(2) there means
// the server closed the connection, which xgb must see as io.EOF instead of
// spinning on an empty read.
func (c wineConn) Read(p []byte) (int, error) {
	if len(p) > 0 {
		if err := waitReadable(c.Fd(), winescape.Poll); err != nil {
			return 0, err
		}
	}
	return eofReader{c.File}.Read(p)
}

func (wineConn) LocalAddr() net.Addr              { return wineConnAddr{} }
func (wineConn) RemoteAddr() net.Addr             { return wineConnAddr{} }
func (wineConn) SetDeadline(time.Time) error      { return nil }
func (wineConn) SetReadDeadline(time.Time) error  { return nil }
func (wineConn) SetWriteDeadline(time.Time) error { return nil }

// wineConnAddr is the stub net.Addr wineConn reports at both ends: a real
// address is nobody's business here, and net.Conn has no nil-address form.
type wineConnAddr struct{}

func (wineConnAddr) Network() string { return "unix" }
func (wineConnAddr) String() string  { return "wine-x11-unix-socket" }

// Compile-time check that wineConn actually satisfies net.Conn -- the point
// of wrapping *winescape.File at all.
var _ net.Conn = wineConn{}
