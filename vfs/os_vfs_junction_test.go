package vfs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestResolveReparseCandidates verifies that an arbitrary reparse point
// (junction/symlink) anywhere in the path is resolved to its target, not
// just the few hard-coded well-known names. This is the core of the fix for
// entering "C:\Documents and Settings\<user>\Application Data" which used to
// fail with "Access Denied" because ReadDir only knew "documents and
// settings", "all users" and "default user".
func TestResolveReparseCandidates(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reparse-point resolution is Windows-specific")
	}

	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o700); err != nil {
		t.Fatal(err)
	}

	// "Application Data" is deliberately NOT one of the hard-coded
	// well-known junction names -- it is a per-user profile junction like
	// the one that broke navigation.
	link := filepath.Join(root, "Application Data")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink creation unavailable (need elevated privileges): %v", err)
	}

	candidates := resolveReparseCandidates(link)
	if len(candidates) == 0 {
		t.Fatalf("resolveReparseCandidates(%q) returned no candidates", link)
	}

	resolved := filepath.Clean(candidates[0])
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", real, err)
	}
	want = filepath.Clean(want)
	// EvalSymlinks normalizes path casing and expands 8.3 short names on
	// Windows (e.g. "RUNNER~1" -> "runneradmin"), so compare canonical
	// paths case-insensitively.
	if !strings.EqualFold(resolved, want) {
		t.Errorf("resolveReparseCandidates(%q)[0] = %q, want %q", link, resolved, want)
	}
}

// TestOSVFS_ReadDirFollowsNestedReparse exercises the full ReadDir path with a
// reparse point nested inside another reparse point (outer "link" -> real,
// inner "Application Data" -> real2). ReadDir must transparently resolve the
// chain and list the payload that lives only in the final target.
func TestOSVFS_ReadDirFollowsNestedReparse(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reparse-point traversal is Windows-specific")
	}

	root := t.TempDir()
	real := filepath.Join(root, "real")
	real2 := filepath.Join(root, "real2")
	if err := os.MkdirAll(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(real2, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real2, "payload.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink creation unavailable (need elevated privileges): %v", err)
	}
	// Inner reparse living inside the target of the outer one, named like a
	// real per-user profile junction.
	inner := filepath.Join(real, "Application Data")
	if err := os.Symlink(real2, inner); err != nil {
		t.Skipf("symlink creation unavailable (need elevated privileges): %v", err)
	}

	v := NewOSVFS(link)
	nested := filepath.Join(link, "Application Data")

	var found bool
	err := v.ReadDir(context.Background(), nested, func(items []VFSItem) {
		for _, it := range items {
			if it.Name == "payload.txt" {
				found = true
			}
		}
	})
	if err != nil {
		t.Fatalf("ReadDir(%q) failed: %v", nested, err)
	}
	if !found {
		t.Errorf("ReadDir(%q) did not follow the nested reparse and list payload.txt", nested)
	}
}
