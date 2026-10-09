package netfox

import (
	"context"
	"strings"
	"testing"
)

func assertSFTPURIErrorBatch33(t *testing.T, raw string) {
	t.Helper()
	if _, err := (&sftpURIProvider{}).OpenURI(context.Background(), nil, raw); err == nil {
		t.Fatalf("OpenURI(%q) returned nil error", raw)
	}
}

func TestSFTPURIProviderSchemeCoverageBatch33(t *testing.T) {
	if got := (&sftpURIProvider{}).Scheme(); got != "sftp" {
		t.Fatalf("Scheme() = %q, want sftp", got)
	}
}

func TestSFTPURIProviderMalformedEscapeCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://%zz")
}

func TestSFTPURIProviderMissingHostCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp:///var/tmp")
}

func TestSFTPURIProviderEmptyInputCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "")
}

func TestSFTPURIProviderDefaultPortCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://127.0.0.1/")
}

func TestSFTPURIProviderExplicitPortCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://127.0.0.1:1/")
}

func TestSFTPURIProviderUsernameCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://alice@127.0.0.1:1/")
}

func TestSFTPURIProviderPasswordCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://alice:secret@127.0.0.1:1/")
}

func TestSFTPURIProviderRootPathCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://127.0.0.1:1/")
}

func TestSFTPURIProviderNestedPathCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://alice:secret@127.0.0.1:1/home/alice")
}

// TestSCPURIProviderIsTheSFTPBackendUnderAnotherScheme (f4#187): scp:// is
// answered by the SFTP provider, and its errors name the scheme the user typed.
func TestSCPURIProviderIsTheSFTPBackendUnderAnotherScheme(t *testing.T) {
	provider := &sftpURIProvider{alias: "scp"}
	if got := provider.Scheme(); got != "scp" {
		t.Fatalf("scheme = %q, want scp", got)
	}
	if _, err := provider.OpenURI(context.Background(), nil, "scp:///tmp"); err == nil || !strings.Contains(err.Error(), "scp: no host") {
		t.Errorf("hostless scp URL error = %v, want \"scp: no host\"", err)
	}
	if _, err := provider.OpenURI(context.Background(), nil, "://"); err == nil || !strings.Contains(err.Error(), "scp:") {
		t.Errorf("malformed scp URL error = %v, want the scp: prefix", err)
	}
}

// TestSCPConnectionTypeIsListed (f4#187): "scp" is a connection type of the
// manager next to sftp, with the SSH port.
func TestSCPConnectionTypeIsListed(t *testing.T) {
	listed := map[string]bool{}
	for _, p := range GetProtocols() {
		listed[p] = true
	}
	if !listed["scp"] || !listed["sftp"] {
		t.Fatalf("protocols = %v, want scp and sftp", GetProtocols())
	}
	ph := &scpProtocolHandler{}
	if ph.Prefix() != "scp" || ph.DefaultPort() != "22" {
		t.Errorf("scp handler = %q port %q", ph.Prefix(), ph.DefaultPort())
	}
	if ui, cleanup := ph.BuildExtraUI(&NetFoxConfig{}, 0, 0, 10, 1); ui != nil {
		cleanup()
		t.Error("scp has no extra UI")
	}
}
