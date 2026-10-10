package winex11drag

import (
	"errors"
	"net"
	"testing"
	"time"
)

// The CI runner -- and every other place this test suite runs -- is not
// Wine, so hostmode.Posix() is false and OpenWineX11Connection must refuse
// before it ever touches winescape.DialUnix or the filesystem. This is the
// one thing about OpenWineX11Connection this package can test without a
// live Wine + X11 environment; see its doc comment for the rest.
func TestOpenWineX11ConnectionRefusesOutsidePosixMode(t *testing.T) {
	_, err := OpenWineX11Connection()
	if !errors.Is(err, ErrNotWineHost) {
		t.Fatalf("OpenWineX11Connection() error = %v, want %v", err, ErrNotWineHost)
	}
}

func TestWineConnSatisfiesNetConn(t *testing.T) {
	var c wineConn
	var _ net.Conn = c

	if c.LocalAddr() == nil || c.RemoteAddr() == nil {
		t.Fatalf("wineConn address stubs returned nil")
	}
	if err := c.SetDeadline(time.Time{}); err != nil {
		t.Errorf("SetDeadline: %v", err)
	}
	if err := c.SetReadDeadline(time.Time{}); err != nil {
		t.Errorf("SetReadDeadline: %v", err)
	}
	if err := c.SetWriteDeadline(time.Time{}); err != nil {
		t.Errorf("SetWriteDeadline: %v", err)
	}
}

func TestWineConnAddr(t *testing.T) {
	addr := wineConnAddr{}
	if addr.Network() == "" {
		t.Error("wineConnAddr.Network() is empty")
	}
	if addr.String() == "" {
		t.Error("wineConnAddr.String() is empty")
	}
}
