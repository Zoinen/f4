//go:build windows

package vfs

import (
	"os/exec"
	"testing"
)

// denyDirectoryListing adds an explicit deny ACE for Everyone (S-1-1-0) on the
// "read data/list directory" right (RD) of dir alone. Reading its attributes
// stays allowed, so Stat keeps working while opening it for listing is
// refused -- the state of a folder the current user may not look into.
func denyDirectoryListing(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("icacls", dir, "/deny", "*S-1-1-0:(RD)").CombinedOutput(); err != nil {
		t.Skipf("icacls /deny unavailable on this host: %v: %s", err, out)
	}
	// Registered after t.TempDir, so the ACE is gone before the directory
	// is removed; RemoveAll has to list it.
	t.Cleanup(func() {
		if out, err := exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").CombinedOutput(); err != nil {
			t.Errorf("icacls /remove:d %q: %v: %s", dir, err, out)
		}
	})
}
