package netproxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveInheritsTheGlobalSettings(t *testing.T) {
	old := Global()
	t.Cleanup(func() { SetGlobal(old) })

	SetGlobal(Settings{Mode: ModeHTTP, Host: "gw", Port: "8080"})
	if got := Resolve(Settings{}); got.Mode != ModeHTTP || got.Host != "gw" {
		t.Errorf("a zero value must inherit, got %+v", got)
	}
	// An explicit per-connection setting wins over the global one.
	own := Settings{Mode: ModeDirect}
	if got := Resolve(own); got.Mode != ModeDirect {
		t.Errorf("an override must win, got %+v", got)
	}
	// The global settings themselves can never mean "inherit".
	SetGlobal(Settings{Mode: ModeGlobal})
	if Global().Mode != ModeSystem {
		t.Errorf("a global ModeGlobal must fall back to the environment, got %+v", Global())
	}
}

func TestURLCarriesCredentialsAndDefaultPorts(t *testing.T) {
	u := Settings{Mode: ModeHTTP, Host: "gw", User: "bob", Pass: "s3cret"}.URL()
	if u == nil || u.Scheme != "http" || u.Host != "gw:3128" {
		t.Fatalf("http proxy url: %v", u)
	}
	if pw, _ := u.User.Password(); u.User.Username() != "bob" || pw != "s3cret" {
		t.Errorf("credentials lost: %v", u.User)
	}
	if u := (Settings{Mode: ModeSOCKS5, Host: "gw"}).URL(); u == nil || u.Host != "gw:1080" {
		t.Errorf("socks5 default port: %v", u)
	}
	// Without a host there is nothing to proxy through.
	if u := (Settings{Mode: ModeHTTP}).URL(); u != nil {
		t.Errorf("an empty host must not produce a url: %v", u)
	}
	// The password never leaks into the description.
	if d := (Settings{Mode: ModeHTTP, Host: "gw", User: "bob", Pass: "s3cret"}).Describe(); strings.Contains(d, "s3cret") {
		t.Errorf("Describe leaked the password: %s", d)
	}
}

func TestHTTPClientGoesThroughTheProxyWithAuth(t *testing.T) {
	var gotAuth, gotHost string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Proxy-Authorization")
		gotHost = r.Host
		if _, err := w.Write([]byte("through the proxy")); err != nil {
			t.Errorf("write proxy response: %v", err)
		}
	}))
	defer proxySrv.Close()

	host := strings.TrimPrefix(proxySrv.URL, "http://")
	hostOnly, port, _ := net.SplitHostPort(host)
	s := Settings{Mode: ModeHTTP, Host: hostOnly, Port: port, User: "bob", Pass: "s3cret"}

	resp, err := s.HTTPClient(5 * time.Second).Get("http://example.invalid/release.json")
	if err != nil {
		t.Fatalf("request through the proxy failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close() // response body cleanup errors are uninteresting
	}()
	body, _ := io.ReadAll(resp.Body)

	if string(body) != "through the proxy" {
		t.Errorf("body: %q", body)
	}
	if gotHost != "example.invalid" {
		t.Errorf("the proxy should have been asked for the origin host, got %q", gotHost)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("bob:s3cret"))
	if gotAuth != want {
		t.Errorf("Proxy-Authorization: got %q, want %q", gotAuth, want)
	}

	// Without credentials the header must simply be absent.
	gotAuth = ""
	s.User, s.Pass = "", ""
	resp2, err := s.HTTPClient(5 * time.Second).Get("http://example.invalid/")
	if err != nil {
		t.Fatalf("anonymous request through the proxy failed: %v", err)
	}
	_ = resp2.Body.Close() // response body cleanup errors are uninteresting
	if gotAuth != "" {
		t.Errorf("unexpected Proxy-Authorization: %q", gotAuth)
	}
}

func TestDirectModeIgnoresTheEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1/")
	t.Setenv("http_proxy", "http://127.0.0.1:1/")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("direct")); err != nil {
			t.Errorf("write direct response: %v", err)
		}
	}))
	defer srv.Close()

	resp, err := (Settings{Mode: ModeDirect}).HTTPClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("direct mode should have bypassed the env proxy: %v", err)
	}
	defer func() {
		_ = resp.Body.Close() // response body cleanup errors are uninteresting
	}()
	if body, _ := io.ReadAll(resp.Body); string(body) != "direct" {
		t.Errorf("body: %q", body)
	}
}

// fakeConnectProxy answers CONNECT and then pipes the tunnel to target.
func fakeConnectProxy(t *testing.T, wantAuth string, target string) net.Listener {
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
			go func(c net.Conn) {
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					_ = c.Close() // connection cleanup errors are uninteresting
					return
				}
				if req.Method != http.MethodConnect || (wantAuth != "" && req.Header.Get("Proxy-Authorization") != wantAuth) {
					if _, err := io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n"); err != nil {
						_ = c.Close() // connection cleanup errors are uninteresting
						return
					}
					_ = c.Close() // connection cleanup errors are uninteresting
					return
				}
				up, err := net.Dial("tcp", target)
				if err != nil {
					if _, err := io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"); err != nil {
						_ = c.Close() // connection cleanup errors are uninteresting
						return
					}
					_ = c.Close() // connection cleanup errors are uninteresting
					return
				}
				if _, err := io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
					_ = up.Close() // connection cleanup errors are uninteresting
					_ = c.Close()  // connection cleanup errors are uninteresting
					return
				}
				go func() {
					_, _ = io.Copy(up, c) // tunnel shutdown errors are uninteresting
				}()
				_, _ = io.Copy(c, up) // tunnel shutdown errors are uninteresting
				_ = up.Close()        // connection cleanup errors are uninteresting
				_ = c.Close()         // connection cleanup errors are uninteresting
			}(c)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close() // listener cleanup errors are uninteresting
	})
	return ln
}

func TestDialContextTunnelsRawTCPThroughCONNECT(t *testing.T) {
	// The "site" behind the proxy: it greets whoever connects, the way an
	// SSH or FTP server would.
	site, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = site.Close() // listener cleanup errors are uninteresting
	}()
	go func() {
		for {
			c, err := site.Accept()
			if err != nil {
				return
			}
			if _, err := io.WriteString(c, "220 hello\r\n"); err != nil {
				_ = c.Close() // connection cleanup errors are uninteresting
				continue
			}
			_ = c.Close() // connection cleanup errors are uninteresting
		}
	}()

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("bob:s3cret"))
	ln := fakeConnectProxy(t, want, site.Addr().String())
	host, port, _ := net.SplitHostPort(ln.Addr().String())

	s := Settings{Mode: ModeHTTP, Host: host, Port: port, User: "bob", Pass: "s3cret"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := s.DialContext(ctx, "tcp", site.Addr().String())
	if err != nil {
		t.Fatalf("CONNECT tunnel failed: %v", err)
	}
	defer func() {
		_ = conn.Close() // connection cleanup errors are uninteresting
	}()
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "220 hello") {
		t.Errorf("greeting through the tunnel: %q, %v", greeting, err)
	}

	// Wrong credentials must surface as a plain 407 rather than a hang.
	bad := s
	bad.Pass = "nope"
	if _, err := bad.DialContext(ctx, "tcp", site.Addr().String()); err == nil || !strings.Contains(err.Error(), "407") {
		t.Errorf("expected a 407 error, got %v", err)
	}
}

func TestSecretRoundTrip(t *testing.T) {
	keyOverride = make([]byte, 32)
	t.Cleanup(func() { keyOverride = nil })

	enc := EncodeSecret("s3cret")
	if enc == "s3cret" || !strings.HasPrefix(enc, secretPrefix) {
		t.Fatalf("password was not obfuscated: %q", enc)
	}
	if got := DecodeSecret(enc); got != "s3cret" {
		t.Errorf("round trip: %q", got)
	}
	// A hand-written plain password stays usable, an empty one stays empty.
	if got := DecodeSecret("plain"); got != "plain" {
		t.Errorf("plain value: %q", got)
	}
	if EncodeSecret("") != "" || DecodeSecret("") != "" {
		t.Error("empty passwords must stay empty")
	}
}
func TestSystemModeAttachesAppCredentialsIfMissingInEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:3128")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	t.Setenv("http_proxy", "http://127.0.0.1:3128")
	t.Setenv("https_proxy", "http://127.0.0.1:3128")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")

	s := Settings{Mode: ModeSystem, User: "bob", Pass: "s3cret"}
	req, _ := http.NewRequest("GET", "https://api.github.com/", nil)

	pFunc := s.proxyFunc()
	if pFunc == nil {
		t.Fatal("proxyFunc returned nil for ModeSystem")
	}

	u, err := pFunc(req)
	if err != nil || u == nil {
		t.Fatalf("expected proxy URL, got err=%v, u=%v", err, u)
	}

	if u.User == nil {
		t.Fatal("proxy URL missing User credentials")
	}
	if u.User.Username() != "bob" {
		t.Errorf("got username %q, want %q", u.User.Username(), "bob")
	}
	if pass, _ := u.User.Password(); pass != "s3cret" {
		t.Errorf("got password %q, want %q", pass, "s3cret")
	}
}

func TestSystemModePreservesEnvCredentialsIfPresent(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://envuser:envpass@127.0.0.1:3128")
	t.Setenv("HTTPS_PROXY", "http://envuser:envpass@127.0.0.1:3128")
	t.Setenv("http_proxy", "http://envuser:envpass@127.0.0.1:3128")
	t.Setenv("https_proxy", "http://envuser:envpass@127.0.0.1:3128")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")

	s := Settings{Mode: ModeSystem, User: "appuser", Pass: "apppass"}
	req, _ := http.NewRequest("GET", "https://api.github.com/", nil)

	u, err := s.proxyFunc()(req)
	if err != nil || u == nil {
		t.Fatalf("expected proxy URL, got err=%v, u=%v", err, u)
	}

	if u.User.Username() != "envuser" {
		t.Errorf("got username %q, want envuser", u.User.Username())
	}
}

func TestSystemModeAllProxyFallback(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:3128")

	s := Settings{Mode: ModeSystem, User: "bob", Pass: "s3cret"}
	req, _ := http.NewRequest("GET", "https://api.github.com/", nil)

	u, err := s.proxyFunc()(req)
	if err != nil || u == nil {
		t.Fatalf("expected proxy URL from ALL_PROXY, got err=%v, u=%v", err, u)
	}

	if u.Host != "127.0.0.1:3128" {
		t.Errorf("got host %q, want 127.0.0.1:3128", u.Host)
	}
	if u.User == nil || u.User.Username() != "bob" {
		t.Errorf("credentials not attached to ALL_PROXY fallback: %v", u.User)
	}
}
