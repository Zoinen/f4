//go:build !windows

package vfs

import (
	"os"
	"testing"
)

// denyDirectoryListing removes every permission bit from dir: stat(2) still
// works through the parent, open(2) for reading does not. Root ignores the
// bits, which the caller's precondition check detects.
func denyDirectoryListing(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatalf("chmod %q: %v", dir, err)
	}
	// Registered after t.TempDir, so it runs before the directory is removed.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // #nosec G302 -- restores the test directory so it can be listed and removed.
}
