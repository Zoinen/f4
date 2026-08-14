package netfox

import (
	"context"
	"errors"
	"github.com/unxed/f4/internal/netproxy"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestFishReconnectRepointsEveryView is what the mechanical change bought: a
// panel that did not ask for the reconnect must still be able to work, and it
// must work through the session that replaced the one that died rather than
// through the corpse it was holding.
func TestFishReconnectRepointsEveryView(t *testing.T) {
	dial := localShellDialer(t)
	v, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	defer v.Close()

	other, ok := v.Clone().(*FishVFS)
	if !ok {
		t.Fatal("Clone did not hand back a FishVFS")
	}
	defer other.Close()

	dead := v.Client()
	dead.Session().MarkBroken()

	if err := v.Reconnect(context.Background()); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if other.Client() == dead {
		t.Fatal("the view that did not reconnect still holds the session that died")
	}
	if other.Client() != v.Client() {
		t.Fatal("two views of one connection ended up on different sessions")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// The accessor agreeing is not enough: the request path has to reach the
	// live session too, which is the only thing a user would notice.
	if _, err := other.Stat(ctx, "/"); err != nil {
		t.Fatalf("the view that did not reconnect cannot make a request: %v", err)
	}
}

// deadTCPPort hands back a port on the loopback interface that nothing is
// listening on, by taking one and giving it straight back.
func deadTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback listener available: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return strconv.Itoa(port)
}

// TestSSHFishDialerReportsAFailedDial: a dialer that cannot reach the host says
// so and hands back nothing to close. A closer returned alongside an error
// would be leaked by every caller, since none of them expect one.
func TestSSHFishDialerReportsAFailedDial(t *testing.T) {
	dial := sshFishDialer("127.0.0.1", deadTCPPort(t), "nobody", "", 1, netproxy.Settings{})
	stdin, stdout, closer, err := dial(context.Background())
	if err == nil {
		if closer != nil {
			closer.Close()
		}
		t.Fatal("dialling a dead port succeeded")
	}
	if stdin != nil || stdout != nil || closer != nil {
		t.Fatal("a failed dial handed back something to use")
	}
}

// TestSSHFishDialerHonoursACancelledContext: a reconnect the user gave up on
// must not open a connection, and the check has to happen before the dial
// because DialSSH itself cannot be interrupted.
func TestSSHFishDialerHonoursACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dial := sshFishDialer("127.0.0.1", deadTCPPort(t), "nobody", "", 1, netproxy.Settings{})
	_, _, closer, err := dial(ctx)
	if !errors.Is(err, context.Canceled) {
		if closer != nil {
			closer.Close()
		}
		t.Fatalf("dialler answered %v, want context.Canceled", err)
	}
	if closer != nil {
		t.Fatal("a cancelled dial handed back something to close")
	}
}

// TestNewFishVFSReportsAFailedDial keeps the site constructor honest: it now
// opens through a dialer, and a host that cannot be reached must still fail at
// open time rather than at the first request.
func TestNewFishVFSReportsAFailedDial(t *testing.T) {
	v, err := NewFishVFS(nil, "127.0.0.1", deadTCPPort(t), "nobody", "", 1, netproxy.Settings{})
	if err == nil {
		v.Close()
		t.Fatal("opening a site on a dead port succeeded")
	}
	if v != nil {
		t.Fatal("a failed open handed back a file system")
	}
}
