package terminal

import (
	"github.com/unxed/f4/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func useClipboardAuthConfig(t *testing.T, root string) {
	t.Helper()

	oldUserConfigDir := config.UserConfigDir
	oldExecutable := config.Executable
	config.UserConfigDir = func() (string, error) { return root, nil }
	config.Executable = func() (string, error) { return filepath.Join(root, "f4"), nil }
	config.ResetConfigDirForTest()
	t.Cleanup(func() {
		config.UserConfigDir = oldUserConfigDir
		config.Executable = oldExecutable
		config.ResetConfigDirForTest()
	})
}

func TestNewF4ClipboardAuthLoadsExistingEntries(t *testing.T) {
	root := t.TempDir()
	useClipboardAuthConfig(t, root)

	path := filepath.Join(root, "f4", "tty_clipboard", "autheds")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(" client-a \n\nclient-b\n"), 0600); err != nil {
		t.Fatal(err)
	}

	auth := NewF4ClipboardAuth()
	if auth.path != path {
		t.Fatalf("auth path = %q, want %q", auth.path, path)
	}
	if !auth.autheds["client-a"] || !auth.autheds["client-b"] {
		t.Fatalf("loaded auth entries = %#v, want both clients", auth.autheds)
	}
	if len(auth.autheds) != 2 {
		t.Fatalf("loaded auth entries = %#v, want blank lines ignored", auth.autheds)
	}
}

func TestNewF4ClipboardAuthCreatesStorageDirectory(t *testing.T) {
	root := t.TempDir()
	useClipboardAuthConfig(t, root)

	auth := NewF4ClipboardAuth()
	if _, err := os.Stat(filepath.Dir(auth.path)); err != nil {
		t.Fatalf("clipboard auth directory was not created: %v", err)
	}
	if len(auth.autheds) != 0 {
		t.Fatalf("new auth cache = %#v, want empty", auth.autheds)
	}
}

func TestF4ClipboardAuthAuthorizeCachedClient(t *testing.T) {
	auth := &F4ClipboardAuth{autheds: map[string]bool{"trusted": true}}

	if got := auth.Authorize("trusted"); got != 1 {
		t.Fatalf("Authorize(cached) = %d, want 1", got)
	}
}
