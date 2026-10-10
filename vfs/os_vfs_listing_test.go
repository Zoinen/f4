package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs/hostfs"
)

func TestOSVFSCheckDirectoryListableWithoutStatOrNavigation(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	v := NewOSVFS(root)
	previousHook := OSVFSSetPathBenchmarkHook
	statEvents := 0
	OSVFSSetPathBenchmarkHook = func(string, ...any) { statEvents++ }
	t.Cleanup(func() { OSVFSSetPathBenchmarkHook = previousHook })
	// Missing/stale rows are deliberately left for ReadDir, just as the
	// existing listing-permission policy does; this is not a Stat replacement.
	for _, path := range []string{child, "child", filepath.Join(root, "missing")} {
		t.Run(path, func(t *testing.T) {
			if err := v.CheckDirectoryListable(path); err != nil {
				t.Fatal(err)
			}
			if got := v.GetPath(); got != root {
				t.Fatalf("listability check navigated to %q", got)
			}
			if statEvents != 0 {
				t.Fatalf("listability check performed %d Stat stages", statEvents)
			}
		})
	}
}

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
	previousHook := OSVFSSetPathBenchmarkHook
	statEvents := 0
	OSVFSSetPathBenchmarkHook = func(string, ...any) { statEvents++ }
	checkErr := v.CheckDirectoryListable(denied)
	OSVFSSetPathBenchmarkHook = previousHook
	var checkNotListable *NotListableError
	if !errors.As(checkErr, &checkNotListable) || !errors.Is(checkErr, os.ErrPermission) {
		t.Fatalf("CheckDirectoryListable(%q) = %v, want NotListableError wrapping permission refusal", denied, checkErr)
	}
	if checkNotListable.Path != denied {
		t.Errorf("refused directory path = %q, want %q", checkNotListable.Path, denied)
	}
	if statEvents != 0 {
		t.Fatalf("CheckDirectoryListable performed %d Stat stages", statEvents)
	}
	if got := v.GetPath(); got != root {
		t.Fatalf("listability refusal navigated to %q", got)
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

// TestOSVFSRefusalMessagesUsePlainPaths is the regression test for the dialog
// that reported "Cannot access folder: open \\?\C:\...: Access is denied".
// The extended-length prefix prepareOSPath hands to the syscall is a Win32
// plumbing detail; the message a user reads must name the path the way the
// panel shows it, or the path cannot be read back, typed or pasted.
func TestOSVFSRefusalMessagesUsePlainPaths(t *testing.T) {
	if globalSudoClient.IsAvailable() {
		t.Skip("an elevation route is configured; SetPath accepts the path and ReadDir elevates")
	}
	root := t.TempDir()
	denied := filepath.Join(root, "denied")
	if err := os.Mkdir(denied, 0o700); err != nil {
		t.Fatal(err)
	}
	denyDirectoryListing(t, denied)

	if f, err := hostfs.Open(prepareOSPath(denied)); err == nil {
		_ = f.Close()
		t.Skipf("precondition not met: the host lets %q be opened for listing", denied)
	} else if !errors.Is(err, os.ErrPermission) {
		t.Skipf("precondition not met: opening %q failed with %v, not a permission error", denied, err)
	}

	v := NewOSVFS(root)
	setErr := v.SetPath(denied)
	if setErr == nil {
		t.Fatalf("SetPath(%q) accepted a directory ReadDir cannot list", denied)
	}
	assertPlainPathMessage(t, "SetPath", setErr, denied)

	readErr := v.ReadDir(context.Background(), denied, func([]VFSItem) {})
	if readErr == nil {
		t.Fatalf("ReadDir(%q) listed a directory the host refuses to open", denied)
	}
	assertPlainPathMessage(t, "ReadDir", readErr, denied)
}

// assertPlainPathMessage fails when a user-facing VFS error leaks the
// extended-length prefix. It does not require the plain path to appear: the
// error may be wrapped in wording of its own, and what is guarded here is the
// spelling of the path, not the sentence around it. Where the prefix would
// have been added, the plain path must be what is left.
func assertPlainPathMessage(t *testing.T, op string, err error, wantPath string) {
	t.Helper()
	if msg := err.Error(); strings.Contains(msg, `\\?\`) {
		t.Errorf("%s(%q) error reports the extended-length prefix: %q", op, wantPath, msg)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("%s(%q) error %v does not unwrap to os.ErrPermission", op, wantPath, err)
	}
	if prepareOSPath(wantPath) != wantPath && !strings.Contains(err.Error(), wantPath) {
		t.Errorf("%s(%q) error %q does not name %q", op, wantPath, err, wantPath)
	}
}
