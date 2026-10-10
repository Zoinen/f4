//go:build windows

package netfox

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestWSLTitle(t *testing.T) {
	if got := wslTitle(""); got != "WSL" {
		t.Errorf(`wslTitle("") = %q, want "WSL"`, got)
	}
	if got := wslTitle("Ubuntu"); got != "WSL:Ubuntu" {
		t.Errorf(`wslTitle("Ubuntu") = %q, want "WSL:Ubuntu"`, got)
	}
}

// TestNewWSLVFSReportsAMissingBinary must not depend on the CI runner
// actually having WSL installed -- see dialWSLSubprocess's own test for why
// wslLookPath is mocked rather than left to find a real wsl.exe.
func TestNewWSLVFSReportsAMissingBinary(t *testing.T) {
	orig := wslLookPath
	wslLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { wslLookPath = orig }()

	v, err := NewWSLVFS(nil, "Ubuntu", 1)
	if err == nil {
		if v != nil {
			_ = v.Close()
		}
		t.Fatal("NewWSLVFS succeeded with no wsl.exe on PATH")
	}
	if v != nil {
		t.Errorf("failed NewWSLVFS returned non-nil file system %T", v)
	}
}

// TestWSLProviderOpenReturnsPlainNilOnFailure mirrors
// TestNetFoxProvidersFailedDialReturnPlainNil (sftp_dial_test.go) for the
// "wsl" site type: the provider's Open must not wrap a nil *FishVFS in a
// non-nil vfs.VFS interface, since the asynchronous panel opener closes
// whatever non-nil result it gets while also reporting err.
func TestWSLProviderOpenReturnsPlainNilOnFailure(t *testing.T) {
	orig := wslLookPath
	wslLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { wslLookPath = orig }()

	manager := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := manager.SaveConfig("unreachable", NetFoxConfig{
		Type:    "wsl",
		Host:    "Ubuntu",
		Timeout: "1",
	}); err != nil {
		t.Fatal(err)
	}

	parent := &netFoxVFSWrapper{NetFoxVFS: manager}
	provider := &wslProvider{}

	if !provider.CanOpen(context.Background(), parent, "unreachable") {
		t.Fatal("wslProvider.CanOpen declined a \"wsl\" site")
	}

	opened, err := provider.Open(context.Background(), parent, "unreachable")
	if err == nil {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatal("opening an unreachable wsl site succeeded")
	}
	if opened != nil {
		t.Errorf("failed wsl open returned non-nil file system %T", opened)
		_ = opened.Close()
	}
}

func TestWSLProviderCanOpenRejectsOtherTypes(t *testing.T) {
	manager := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := manager.SaveConfig("elsewhere", NetFoxConfig{
		Type: "sftp",
		Host: "example.com",
	}); err != nil {
		t.Fatal(err)
	}
	parent := &netFoxVFSWrapper{NetFoxVFS: manager}
	if (&wslProvider{}).CanOpen(context.Background(), parent, "elsewhere") {
		t.Fatal("wslProvider.CanOpen accepted a non-\"wsl\" site")
	}
}

var _ vfs.VFSProvider = (*wslProvider)(nil)
