package netfox

import (
	"context"
	"strings"
	"testing"
)

func TestProxyHelpersMapModesAndPadLabels(t *testing.T) {
	if got := padProxyLabel("Host"); got != "Host " {
		t.Fatalf("padProxyLabel = %q, want trailing space", got)
	}
	for i, mode := range proxyModeOrder {
		if got := proxyModeIndex(mode); got != i {
			t.Fatalf("proxyModeIndex(%d) = %d, want %d", mode, got, i)
		}
	}
	if got := proxyModeIndex(-1); got != 0 {
		t.Fatalf("unknown proxy mode index = %d, want 0", got)
	}
	if got := proxyModeItems(); len(got) != len(proxyModeOrder) {
		t.Fatalf("proxyModeItems length = %d, want %d", len(got), len(proxyModeOrder))
	}
}

func TestSFTPURIProviderRejectsMalformedAndHostlessURLs(t *testing.T) {
	provider := &sftpURIProvider{}
	if provider.Scheme() != "sftp" {
		t.Fatalf("scheme = %q, want sftp", provider.Scheme())
	}
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "://", want: "sftp:"},
		{raw: "sftp:///tmp", want: "no host"},
	}
	for _, test := range tests {
		if _, err := provider.OpenURI(context.Background(), nil, test.raw); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("OpenURI(%q) error = %v, want substring %q", test.raw, err, test.want)
		}
	}
}
