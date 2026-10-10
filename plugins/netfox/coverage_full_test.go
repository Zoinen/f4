//go:build !lite

package netfox

import (
	"context"
	"strings"
	"testing"
)

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
