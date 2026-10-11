package netproxy

// This file closes coverage gaps that netproxy_test.go, coverage_test.go and
// keepalive_linux_test.go leave open: SOCKS5 is exercised by nothing (no
// test ever dials through one), Settings.DialContext's ModeSystem branch is
// only exercised through HTTPClient (never as a raw TCP dial, which is what
// netfox's SSH/FTP connections actually use), and connectDialer's error
// paths (unreachable proxy, a proxy that refuses the tunnel, the
// context-free Dial wrapper) are never triggered.

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- SOCKS5 -----------------------------------------------------------

// fakeSocks5Proxy implements just enough of RFC 1928 (and RFC 1929 for
// username/password auth) to drive golang.org/x/net/proxy's client-side
// SOCKS5 dialer, which is what Settings.DialContext(ModeSOCKS5) uses. On a
// successful CONNECT it dials target and relays bytes both ways, the same
// shape fakeConnectProxy (netproxy_test.go) gives the HTTP CONNECT path.
func fakeSocks5Proxy(t *testing.T, creds *[2]string, replyCode byte, target string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeSocks5(c, creds, replyCode, target)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close() // listener cleanup errors are uninteresting
	})
	return ln
}

func handleFakeSocks5(c net.Conn, creds *[2]string, replyCode byte, target string) {
	defer func() { _ = c.Close() }() // connection cleanup errors are uninteresting

	greeting := make([]byte, 2)
	if _, err := io.ReadFull(c, greeting); err != nil || greeting[0] != 0x05 {
		return
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}

	var chosen byte = 0x00
	if creds != nil {
		found := false
		for _, m := range methods {
			if m == 0x02 {
				found = true
			}
		}
		if !found {
			_, _ = c.Write([]byte{0x05, 0xff})
			return
		}
		chosen = 0x02
	}
	if _, err := c.Write([]byte{0x05, chosen}); err != nil {
		return
	}

	if chosen == 0x02 {
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(c, hdr); err != nil {
			return
		}
		uname := make([]byte, hdr[1])
		if _, err := io.ReadFull(c, uname); err != nil {
			return
		}
		plenBuf := make([]byte, 1)
		if _, err := io.ReadFull(c, plenBuf); err != nil {
			return
		}
		pass := make([]byte, plenBuf[0])
		if _, err := io.ReadFull(c, pass); err != nil {
			return
		}
		if string(uname) != creds[0] || string(pass) != creds[1] {
			_, _ = c.Write([]byte{0x01, 0x01})
			return
		}
		if _, err := c.Write([]byte{0x01, 0x00}); err != nil {
			return
		}
	}

	// The CONNECT request: VER CMD RSV ATYP ADDR PORT.
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	switch hdr[3] {
	case 0x01: // IPv4
		buf := make([]byte, 4+2)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
	case 0x03: // domain name
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return
		}
		buf := make([]byte, int(l[0])+2)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
	case 0x04: // IPv6
		buf := make([]byte, 16+2)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
	default:
		return
	}

	if replyCode != 0x00 {
		_, _ = c.Write([]byte{0x05, replyCode, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	up, err := net.Dial("tcp", target)
	if err != nil {
		return
	}
	defer func() { _ = up.Close() }() // connection cleanup errors are uninteresting
	go func() {
		_, _ = io.Copy(up, c) // tunnel shutdown errors are uninteresting
	}()
	_, _ = io.Copy(c, up) // tunnel shutdown errors are uninteresting
}

// greetingSite is a tiny TCP server that sends one line and closes, used as
// the "site" behind a proxy in these tests.
func greetingSite(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(c, "220 hello\r\n")
			_ = c.Close() // connection cleanup errors are uninteresting
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close() // listener cleanup errors are uninteresting
	})
	return ln
}

func TestDialContextSOCKS5ImplicitFallsBackToDirect(t *testing.T) {
	site := greetingSite(t)
	s := Settings{Mode: ModeSOCKS5} // no host: not Explicit()

	conn, err := s.DialContext(context.Background(), "tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "220 hello") {
		t.Errorf("greeting = %q, err=%v", greeting, err)
	}
}

func TestDialContextSOCKS5TunnelsRawTCP(t *testing.T) {
	site := greetingSite(t)
	ln := fakeSocks5Proxy(t, nil, 0x00, site.Addr().String())
	host, port, _ := net.SplitHostPort(ln.Addr().String())

	s := Settings{Mode: ModeSOCKS5, Host: host, Port: port}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := s.DialContext(ctx, "tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("SOCKS5 tunnel failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "220 hello") {
		t.Errorf("greeting through SOCKS5 tunnel: %q, %v", greeting, err)
	}
}

func TestDialContextSOCKS5WithCredentials(t *testing.T) {
	site := greetingSite(t)
	creds := [2]string{"bob", "s3cret"}
	ln := fakeSocks5Proxy(t, &creds, 0x00, site.Addr().String())
	host, port, _ := net.SplitHostPort(ln.Addr().String())

	good := Settings{Mode: ModeSOCKS5, Host: host, Port: port, User: "bob", Pass: "s3cret"}
	conn, err := good.DialContext(context.Background(), "tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("authenticated SOCKS5 tunnel failed: %v", err)
	}
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close() // connection cleanup errors are uninteresting
	if err != nil || !strings.HasPrefix(greeting, "220 hello") {
		t.Errorf("greeting through authenticated SOCKS5 tunnel: %q, %v", greeting, err)
	}

	bad := good
	bad.Pass = "wrong"
	if _, err := bad.DialContext(context.Background(), "tcp", site.Addr().String()); err == nil {
		t.Error("expected an error for wrong SOCKS5 credentials")
	}
}

func TestDialContextSOCKS5ServerRejects(t *testing.T) {
	site := greetingSite(t)
	// 0x05 is "connection refused" in RFC 1928's reply-code table.
	ln := fakeSocks5Proxy(t, nil, 0x05, site.Addr().String())
	host, port, _ := net.SplitHostPort(ln.Addr().String())

	s := Settings{Mode: ModeSOCKS5, Host: host, Port: port}
	if _, err := s.DialContext(context.Background(), "tcp", site.Addr().String()); err == nil {
		t.Fatal("expected an error when the SOCKS5 server refuses the CONNECT")
	}
}

// --- ModeSystem raw TCP dialing (netfox's actual use of DialContext) --

func TestDialContextSystemModeRoutesRawTCPThroughProxy(t *testing.T) {
	site := greetingSite(t)
	ln := fakeConnectProxy(t, "", site.Addr().String())

	t.Setenv("HTTPS_PROXY", "http://"+ln.Addr().String())
	t.Setenv("https_proxy", "http://"+ln.Addr().String())
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	s := Settings{Mode: ModeSystem}
	conn, err := s.DialContext(context.Background(), "tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("ModeSystem raw TCP dial through HTTPS_PROXY failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "220 hello") {
		t.Errorf("greeting through the system-mode tunnel: %q, %v", greeting, err)
	}
}

func TestDialContextDirectModeBypassesTheEnvironment(t *testing.T) {
	site := greetingSite(t)
	// A poisoned env that would break the dial if ModeDirect consulted it.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	s := Settings{Mode: ModeDirect}
	conn, err := s.DialContext(context.Background(), "tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("ModeDirect dial: %v", err)
	}
	_ = conn.Close() // connection cleanup errors are uninteresting
}

// --- connectDialer error paths and the context-free Dial wrapper -----

func TestConnectDialerDialWrapperTunnelsRawTCP(t *testing.T) {
	site := greetingSite(t)
	ln := fakeConnectProxy(t, "", site.Addr().String())

	d := &connectDialer{addr: ln.Addr().String(), forward: directDialer()}
	conn, err := d.Dial("tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("connectDialer.Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "220 hello") {
		t.Errorf("greeting via connectDialer.Dial: %q, %v", greeting, err)
	}
}

func TestConnectDialerCannotReachProxy(t *testing.T) {
	// Bind and immediately close: nothing is listening on this address
	// afterward, so the dial to the "proxy" itself must fail up front.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	d := &connectDialer{addr: addr, forward: directDialer()}
	_, err = d.DialContext(context.Background(), "tcp", "example.invalid:80")
	if err == nil || !strings.Contains(err.Error(), "cannot reach proxy") {
		t.Fatalf("expected a 'cannot reach proxy' error, got %v", err)
	}
}

func TestConnectDialerUpstreamUnreachableGivesBadGateway(t *testing.T) {
	// fakeConnectProxy dials target itself; an address nothing listens on
	// makes it answer with a 502, which DialContext must surface as an
	// explicit "refused CONNECT" error rather than hanging or panicking.
	unreachable, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target := unreachable.Addr().String()
	_ = unreachable.Close()

	ln := fakeConnectProxy(t, "", target)
	d := &connectDialer{addr: ln.Addr().String(), forward: directDialer()}

	_, err = d.DialContext(context.Background(), "tcp", target)
	if err == nil || !strings.Contains(err.Error(), "refused CONNECT") {
		t.Fatalf("expected a 'refused CONNECT' error for an unreachable target, got %v", err)
	}
}

func TestConnectDialerRejectsNonTCPAddr(t *testing.T) {
	d := &connectDialer{}
	if _, err := d.Dial("udp", "example.invalid:53"); err == nil || !strings.Contains(err.Error(), "cannot carry") {
		t.Fatalf("expected an unsupported-network error from Dial, got %v", err)
	}
}

// proxyFunc for ModeSOCKS5 must build the same URL Settings.URL() does, the
// SOCKS5 counterpart of the ModeHTTP case TestHTTPClientGoesThroughTheProxyWithAuth
// already exercises in netproxy_test.go.
func TestProxyFuncSOCKS5BuildsExplicitURL(t *testing.T) {
	s := Settings{Mode: ModeSOCKS5, Host: "gw", User: "bob", Pass: "s3cret"}
	pFunc := s.proxyFunc()
	if pFunc == nil {
		t.Fatal("proxyFunc returned nil for an explicit SOCKS5 setting")
	}
	u, err := pFunc(&http.Request{})
	if err != nil || u == nil || u.Scheme != "socks5" || u.Host != "gw:1080" {
		t.Fatalf("proxyFunc(SOCKS5) = (%v, %v), want scheme=socks5 host=gw:1080", u, err)
	}
	if pw, _ := u.User.Password(); u.User.Username() != "bob" || pw != "s3cret" {
		t.Errorf("credentials lost in SOCKS5 proxyFunc URL: %v", u.User)
	}

	// A non-explicit SOCKS5 (no host) has nothing to proxy through.
	if got := (Settings{Mode: ModeSOCKS5}).proxyFunc(); got != nil {
		t.Error("proxyFunc should be nil for a non-explicit SOCKS5 setting")
	}
}
