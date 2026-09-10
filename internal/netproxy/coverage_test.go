package netproxy

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSettingsDefaultsAndDescriptions(t *testing.T) {
	if got := (Settings{Mode: ModeDirect, Host: " host "}).Addr(); got != "host:3128" {
		t.Fatalf("direct default address = %q, want host:3128", got)
	}
	if got := (Settings{Mode: ModeSOCKS5, Host: "host"}).Addr(); got != "host:1080" {
		t.Fatalf("SOCKS5 default address = %q, want host:1080", got)
	}
	if (Settings{Mode: ModeHTTP, Host: "  "}).Explicit() {
		t.Fatal("whitespace-only proxy host was explicit")
	}
	if got := (Settings{Mode: ModeGlobal}).Describe(); got != "global" {
		t.Fatalf("global description = %q", got)
	}
	if got := (Settings{Mode: ModeDirect}).Describe(); got != "direct" {
		t.Fatalf("direct description = %q", got)
	}
	if got := (Settings{Mode: ModeSystem}).Describe(); got != "system" {
		t.Fatalf("system description = %q", got)
	}
	if got := (Settings{Mode: ModeHTTP}).Describe(); got != "direct" {
		t.Fatalf("empty HTTP description = %q, want direct", got)
	}
	if got := (Settings{Mode: ModeHTTP, Host: "proxy", User: "bob", Pass: "secret"}).Describe(); got != "http://bob@proxy:3128" {
		t.Fatalf("HTTP description = %q", got)
	}
	if strings.Contains((Settings{Mode: ModeHTTP, Host: "proxy", User: "bob", Pass: "secret"}).Describe(), "secret") {
		t.Fatal("proxy password leaked into description")
	}
}

func TestSystemProxyURLHandlesNilAndEnvironment(t *testing.T) {
	if got, err := systemProxyURL(nil); got != nil || err != nil {
		t.Fatalf("nil request proxy = (%v, %v), want (nil, nil)", got, err)
	}
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("http_proxy", "http://proxy.example:8080")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	req, err := http.NewRequest(http.MethodGet, "http://origin.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := systemProxyURL(req)
	if err != nil || got == nil || got.Host != "proxy.example:8080" {
		t.Fatalf("environment proxy = (%v, %v), want proxy.example:8080", got, err)
	}
}

func TestDecodeSecretRejectsInvalidEncodedValues(t *testing.T) {
	old := keyOverride
	keyOverride = make([]byte, 32)
	t.Cleanup(func() { keyOverride = old })
	for _, value := range []string{"~ENC~%%%", "~ENC~AQ=="} {
		if got := DecodeSecret(value); got != "" {
			t.Errorf("DecodeSecret(%q) = %q, want empty", value, got)
		}
	}
}

func TestConnectDialerRejectsNonTCPNetworks(t *testing.T) {
	_, err := (&connectDialer{}).DialContext(context.Background(), "udp", "example:22")
	if err == nil || !strings.Contains(err.Error(), "cannot carry") {
		t.Fatalf("UDP proxy dial error = %v, want unsupported-network error", err)
	}
}
