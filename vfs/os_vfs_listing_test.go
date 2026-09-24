package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs/hostfs"
)

// TestOSVFSSetPathRefusesDirectoryItCannotList is the regression test for
// #814: a directory the host will stat but not list must be refused by
// SetPath, leaving the current path alone, instead of being accepted and
// then failing in ReadDir after the panel has already shown it.
func TestOSVFSSetPathRefusesDirectoryItCannotList(t *testing.T) {
	if globalSudoClient.IsAvailable() {
		t.Skip("an elevation route is configured; SetPath accepts the path and ReadDir elevates")
	}
	root := t.TempDir()
	readable := filepath.Join(root, "readable")
	denied := filepath.Join(root, "denied")
	for _, dir := range []string{readable, denied} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	denyDirectoryListing(t, denied)

	// Precondition: the state the issue describes. Stat succeeds and opening
	// the directory for listing is refused. A host that does not produce it
	// (root on Unix, an ACL the host ignores) cannot exercise the refusal.
	if st, err := hostfs.Stat(prepareOSPath(denied)); err != nil || !st.IsDir() {
		t.Skipf("precondition not met: Stat(%q) = %v, %v; want a directory", denied, st, err)
	}
	if f, err := hostfs.Open(prepareOSPath(denied)); err == nil {
		_ = f.Close()
		t.Skipf("precondition not met: the host lets %q be opened for listing", denied)
	} else if !errors.Is(err, os.ErrPermission) {
		t.Skipf("precondition not met: opening %q failed with %v, not a permission error", denied, err)
	}
	// ReadDir is what used to fail after SetPath had accepted the path.
	v := NewOSVFS(root)
	if err := v.ReadDir(context.Background(), denied, func([]VFSItem) {}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("ReadDir(%q) = %v, want a permission error", denied, err)
	}

	err := v.SetPath(denied)
	if err == nil {
		t.Fatalf("SetPath(%q) accepted a directory ReadDir cannot list; path is now %q", denied, v.GetPath())
	}
	var notListable *NotListableError
	if !errors.As(err, &notListable) {
		t.Fatalf("SetPath(%q) = %T %v, want *NotListableError", denied, err, err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("SetPath(%q) error %v does not unwrap to os.ErrPermission", denied, err)
	}
	if got := v.GetPath(); got != root {
		t.Errorf("GetPath() after refused SetPath = %q, want %q", got, root)
	}

	// The check must not refuse what can be listed.
	if err := v.SetPath(readable); err != nil {
		t.Fatalf("SetPath(%q) = %v, want nil", readable, err)
	}
	if got := v.GetPath(); got != readable {
		t.Errorf("GetPath() after SetPath(%q) = %q", readable, got)
	}
}
