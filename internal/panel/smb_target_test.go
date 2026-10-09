package panel

import (
	"context"
	"runtime"
	"testing"

	"github.com/unxed/f4/vfs"
)

type smbStubProvider struct{}

func (smbStubProvider) Scheme() string { return "smb" }
func (smbStubProvider) OpenURI(context.Context, vfs.VFS, string) (vfs.VFS, error) {
	return nil, nil
}

func TestSMBTargetForTurnsUNCNamesIntoSMBURIs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows opens UNC paths natively")
	}
	local := vfs.NewOSVFS(t.TempDir())
	cases := []struct{ in, want string }{
		{`\\host\share\dir`, "smb://host/share/dir"},
		{"//no-such-host-f4/share", "smb://no-such-host-f4/share"},
		{"/tmp", "/tmp"},
		{"relative/dir", "relative/dir"},
	}
	// Without an SMB provider (the lite build) every name stays as typed.
	for _, c := range cases {
		if got := smbTargetFor(local, c.in); got != c.in {
			t.Fatalf("no provider: smbTargetFor(%q) = %q, want it unchanged", c.in, got)
		}
	}
	if err := vfs.RegisterURIProvider(smbStubProvider{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vfs.UnregisterURIProvider("smb") })
	for _, c := range cases {
		if got := smbTargetFor(local, c.in); got != c.want {
			t.Errorf("smbTargetFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// On a remote file system "//x" is that system's own path, but a UNC name
	// with backslashes is still unambiguous.
	remote := vfs.NewNullVFS(0)
	if got := smbTargetFor(remote, "//host/share"); got != "//host/share" {
		t.Errorf("remote //host/share = %q, want unchanged", got)
	}
	if got := smbTargetFor(remote, `\\host\share`); got != "smb://host/share" {
		t.Errorf(`remote \\host\share = %q`, got)
	}
}
