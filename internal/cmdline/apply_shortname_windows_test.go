//go:build windows

package cmdline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyCommandShortPathRejectsEmbeddedNUL(t *testing.T) {
	const path = "C:\\f4\x00command.exe"
	if got := ApplyCommandShortPath(path); got != path {
		t.Fatalf("ApplyCommandShortPath(%q) = %q, want the original path", path, got)
	}
}

func TestApplyCommandShortPathReturnsOriginalForMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing command.exe")
	if got := ApplyCommandShortPath(path); got != path {
		t.Fatalf("ApplyCommandShortPath(%q) = %q, want the original path", path, got)
	}
}

func TestApplyCommandShortPathHandlesExistingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing command.exe")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	got := ApplyCommandShortPath(path)
	if got == "" {
		t.Fatal("ApplyCommandShortPath returned an empty path")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("ApplyCommandShortPath returned %q, which is not accessible: %v", got, err)
	}
}
