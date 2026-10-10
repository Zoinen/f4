//go:build darwin

package gui

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyDarwinDockIconSkipsNonCocoaBackends(t *testing.T) {
	for _, backend := range []string{"x11", "wayland", ""} {
		applyDarwinDockIcon(backend)
	}

	oldIcon := darwinIconICNS
	darwinIconICNS = nil
	t.Cleanup(func() { darwinIconICNS = oldIcon })
	// gogpu, ebiten and the native cocoa backend (f4#1571) all open a real
	// AppKit window and take the stamping path; nulling the embedded icon
	// above sends each straight to its early "nothing to stamp" return
	// instead of touching AppKit, which is all a non-GUI unit test may do.
	for _, backend := range []string{"gogpu", "ebiten", "cocoa"} {
		applyDarwinDockIcon(backend)
	}
}

// EnsureDarwinIcon is what a console start calls (there is no backend to
// gate on); with no embedded icon it must return before touching AppKit.
func TestEnsureDarwinIconWithoutIconIsNoop(t *testing.T) {
	oldIcon := darwinIconICNS
	darwinIconICNS = nil
	t.Cleanup(func() { darwinIconICNS = oldIcon })
	EnsureDarwinIcon()
}

// A file the user cannot write never gets the stamp, so EnsureDarwinIcon must
// not try (and load AppKit) on every start; an SSH session must not try either.
func TestEnsureDarwinIconSkipChecks(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	if inSSHSession() {
		t.Fatal("empty SSH variables reported an SSH session")
	}
	t.Setenv("SSH_TTY", "/dev/ttys001")
	if !inSSHSession() {
		t.Fatal("SSH_TTY did not report an SSH session")
	}
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "10.0.0.1 50000 10.0.0.2 22")
	if !inSSHSession() {
		t.Fatal("SSH_CONNECTION did not report an SSH session")
	}
}

// The x11/wayland gate must hold even with an icon to stamp: an XQuartz
// window is not a Cocoa app, and applyDarwinDockIcon must not load AppKit
// for it. With a bogus (non-empty) icon, reaching the stamping path would
// at least log a decode failure; the gate returns before any of that.
func TestApplyDarwinDockIconGateHoldsWithIcon(t *testing.T) {
	oldIcon := darwinIconICNS
	darwinIconICNS = []byte("not an icns")
	t.Cleanup(func() { darwinIconICNS = oldIcon })
	for _, backend := range []string{"x11", "wayland", "", "qt"} {
		applyDarwinDockIcon(backend)
	}
}

func TestHasCustomIconFlag(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if hasCustomIconFlag(missing) {
		t.Fatal("missing file reported a custom icon")
	}

	path := filepath.Join(t.TempDir(), "f4")
	if err := os.WriteFile(path, []byte("executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if hasCustomIconFlag(path) {
		t.Fatal("file without FinderInfo reported a custom icon")
	}

	info := make([]byte, 32)
	if err := unix.Setxattr(path, "com.apple.FinderInfo", info, 0); err != nil {
		t.Skipf("FinderInfo xattr is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = unix.Removexattr(path, "com.apple.FinderInfo") })

	if hasCustomIconFlag(path) {
		t.Fatal("zero FinderInfo reported a custom icon")
	}

	info[8] = 0x04
	if err := unix.Setxattr(path, "com.apple.FinderInfo", info, 0); err != nil {
		t.Fatal(err)
	}
	if !hasCustomIconFlag(path) {
		t.Fatal("kHasCustomIcon flag was not detected")
	}

	info[8] = 0x02
	if err := unix.Setxattr(path, "com.apple.FinderInfo", info, 0); err != nil {
		t.Fatal(err)
	}
	if hasCustomIconFlag(path) {
		t.Fatal("unrelated FinderInfo flag was detected as a custom icon")
	}
}
