package archive

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/unxed/f4/internal/config"
)

// resetRatarmountDetection undoes RatarmountAvailable's process-lifetime
// cache so a test can control PATH and observe a fresh detection result.
func resetRatarmountDetection(t *testing.T) {
	t.Helper()
	ratarmountOnce = sync.Once{}
	ratarmountAvailable = false
	t.Cleanup(func() {
		ratarmountOnce = sync.Once{}
		ratarmountAvailable = false
	})
}

func writeFakeRatarmount(t *testing.T, dir string) {
	t.Helper()
	name := ratarmountBinary
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		name += ".exe"
		mode = 0o644
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestRatarmountAvailable_FoundOnPATH(t *testing.T) {
	resetRatarmountDetection(t)
	dir := t.TempDir()
	writeFakeRatarmount(t, dir)
	t.Setenv("PATH", dir)

	if !RatarmountAvailable() {
		t.Error("RatarmountAvailable() = false, want true with a ratarmount binary on PATH")
	}
}

func TestRatarmountAvailable_NotOnPATH(t *testing.T) {
	resetRatarmountDetection(t)
	t.Setenv("PATH", t.TempDir()) // empty directory: nothing to find

	if RatarmountAvailable() {
		t.Error("RatarmountAvailable() = true, want false with no ratarmount binary on PATH")
	}
}

func TestRatarmountAvailable_CachesTheFirstResult(t *testing.T) {
	resetRatarmountDetection(t)
	dir := t.TempDir()
	writeFakeRatarmount(t, dir)
	t.Setenv("PATH", dir)

	if !RatarmountAvailable() {
		t.Fatal("first call should find the fake binary")
	}
	// Removing it after the first call must not change the cached answer.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if !RatarmountAvailable() {
		t.Error("RatarmountAvailable() changed after the first call; it should be cached for the process")
	}
}

// TestMaybeLogRatarmountPreference only exercises the setting's three
// reachable states without panicking or touching config.App.ArchiveTarIndexCache's
// behavior: this first part of f4#251 does not change how archives are
// opened yet (see the doc comment on maybeLogRatarmountPreference).
func TestMaybeLogRatarmountPreference(t *testing.T) {
	previous := config.App.ArchiveUseRatarmountIfAvailable
	t.Cleanup(func() { config.App.ArchiveUseRatarmountIfAvailable = previous })

	config.App.ArchiveUseRatarmountIfAvailable = false
	maybeLogRatarmountPreference() // setting off: must be a silent no-op

	resetRatarmountDetection(t)
	dir := t.TempDir()
	writeFakeRatarmount(t, dir)
	t.Setenv("PATH", dir)
	config.App.ArchiveUseRatarmountIfAvailable = true
	maybeLogRatarmountPreference() // setting on, binary found

	resetRatarmountDetection(t)
	t.Setenv("PATH", t.TempDir())
	maybeLogRatarmountPreference() // setting on, binary missing
}
