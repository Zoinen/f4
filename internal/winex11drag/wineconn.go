package winex11drag

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/jezek/xgb"
	winescape "github.com/unxed/libwinescape/go"

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

	sock, err := winescape.DialUnix(sockPath)
	if err != nil {
		return nil, fmt.Errorf("winex11drag: dialing %s: %w", sockPath, err)
	}

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

	f, err := winescape.OpenFile(path, winescape.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("winex11drag: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	entries, err := ParseXauthority(f)
	if err != nil {
		return nil, fmt.Errorf("winex11drag: parsing %s: %w", path, err)
	}

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
