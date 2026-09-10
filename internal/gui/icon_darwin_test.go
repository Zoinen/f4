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
	applyDarwinDockIcon("gogpu")
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
