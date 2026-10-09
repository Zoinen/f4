//go:build !windows

package vfs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Code on the UI goroutine opens without elevation: the refusal has to come
// back as it is, without a word to the dispatcher, because the password
// prompt the dispatcher would need is a dialog that very goroutine has to
// show.
func TestOSVFSOpenWithoutElevationLeavesTheDispatcherAlone(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root is refused nothing, so there is nothing for sudo to do")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.txt")
	if err := os.WriteFile(locked, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	old := globalSudoClient
	t.Cleanup(func() { globalSudoClient = old })
	v := NewOSVFS(dir)

	// The dispatcher says "does not exist", which nothing local would say
	// about this file, so that answer coming back means the call went
	// through sudo. The first call checks that it does when allowed to;
	// without it the second check below would prove nothing.
	globalSudoClient = answeringDispatcher(t, "open "+locked+": "+syscall.ENOENT.Error())
	if _, err := v.Open(context.Background(), locked); !os.IsNotExist(err) {
		t.Fatalf("Open = %v, want the dispatcher's answer", err)
	}
	if _, err := v.Open(WithoutElevation(context.Background()), locked); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Open without elevation = %v, want the local refusal", err)
	}
}

func TestElevationAllowedByDefault(t *testing.T) {
	if !ElevationAllowed(context.Background()) {
		t.Error("a plain context rules out elevation")
	}
	if ElevationAllowed(WithoutElevation(context.Background())) {
		t.Error("WithoutElevation did not rule out elevation")
	}
}
