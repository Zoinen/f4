package netfox

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestSFTPVFSMetadataCoverageBatch31(t *testing.T) {
	parent := vfs.NewNullVFS(0)
	v := &SFTPVFS{parent: parent, title: "user@example", path: "/home/user"}
	if v.GetTitle() != "user@example" || v.ParentVFS() != parent || v.GetPath() != "/home/user" {
		t.Fatalf("metadata = (%q, %q, %q), want configured values", v.GetTitle(), v.ParentVFS().GetPath(), v.GetPath())
	}
}

func TestSFTPVFSRootDetectionCoverageBatch31(t *testing.T) {
	for _, path := range []string{"", "/"} {
		if !(&SFTPVFS{path: path}).IsAtRoot() {
			t.Errorf("IsAtRoot(%q) = false", path)
		}
	}
}

func TestSFTPVFSNonRootDetectionCoverageBatch31(t *testing.T) {
	if (&SFTPVFS{path: "/home"}).IsAtRoot() {
		t.Fatal("non-root path was reported as root")
	}
}

func TestSFTPVFSPathOperationsCoverageBatch31(t *testing.T) {
	v := &SFTPVFS{}
	if got := v.Join("/home", "user", "..", "tmp"); got != "/home/tmp" {
		t.Fatalf("Join = %q", got)
	}
	if got, err := v.Abs("relative"); err != nil || got != "relative" {
		t.Fatalf("Abs(relative) = (%q, %v)", got, err)
	}
	if got := v.Base("/home/file.txt"); got != "file.txt" || v.Dir("/home/file.txt") != "/home" {
		t.Fatalf("Base/Dir = (%q, %q)", got, v.Dir("/home/file.txt"))
	}
}

func TestSFTPVFSAbsolutePathDetectionCoverageBatch31(t *testing.T) {
	v := &SFTPVFS{}
	if !v.IsAbs("/tmp") || v.IsAbs("tmp") {
		t.Fatal("IsAbs did not distinguish absolute and relative paths")
	}
}

func TestSFTPVFSCapabilitiesCoverageBatch31(t *testing.T) {
	capabilities := (&SFTPVFS{}).GetCapabilities()
	if !capabilities.HasRandomAccess || !capabilities.HasUnixPermissions || !capabilities.HasWrite {
		t.Fatalf("capabilities = %+v, want random access, Unix permissions, and write", capabilities)
	}
}

func TestSFTPVFSConcurrentCallsCoverageBatch31(t *testing.T) {
	if !(&SFTPVFS{}).SupportsConcurrentCalls() {
		t.Fatal("SFTP VFS unexpectedly disallows concurrent calls")
	}
}

func TestSFTPVFSCommandRunnerInfoCoverageBatch31(t *testing.T) {
	info := (&SFTPVFS{}).CommandRunnerInfo()
	if info.Dialect != vfs.CommandDialectPOSIX || info.MaxParallel != 4 {
		t.Fatalf("command runner info = %+v", info)
	}
}

func TestSFTPVFSCommandListANSIWithoutCodepageCoverageBatch31(t *testing.T) {
	v := &SFTPVFS{}
	input := []byte("plain UTF-8")
	got, err := v.EncodeCommandListANSI(input)
	if err != nil || string(got) != string(input) {
		t.Fatalf("EncodeCommandListANSI = (%q, %v)", got, err)
	}
	got[0] = 'P'
	if input[0] != 'p' {
		t.Fatal("EncodeCommandListANSI did not return an independent copy")
	}
}

func TestSFTPVFSSearchStubCoverageBatch31(t *testing.T) {
	ch, err := (&SFTPVFS{}).Search(context.Background(), "/", "needle")
	if err != nil || ch != nil {
		t.Fatalf("Search = (%v, %v), want (nil, nil)", ch, err)
	}
}
