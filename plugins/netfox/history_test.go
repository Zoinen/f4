package netfox

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// TestNetFoxVFSHistoryEntryDeclines is a regression test for f4#262: the
// NetFox root connections list must never produce a folder-history entry.
// Before this hook existed, the root VFS's bare GetPath() ("net://") fell
// through to the generic "no parent VFS, trust it as local" rule and was
// recorded as a useless entry point with no navigational value.
func TestNetFoxVFSHistoryEntryDeclines(t *testing.T) {
	v := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if display, ref, ok := v.HistoryEntry(); ok {
		t.Fatalf("NetFox root connections list must not produce a folder-history entry, got display=%q ref=%q", display, ref)
	}
	if v.NavigateHistoryEntry("anything") {
		t.Fatal("NetFox root VFS must never accept a history navigation request")
	}
}

func TestFishHistoryNavigatesWithinQualifiedSavedConnection(t *testing.T) {
	filesystem := newLocalFishVFS(t)
	defer func() { _ = filesystem.Close() }()
	filesystem.SetDevicePath(vfs.DevicePath{Scheme: "net", Device: "saved connection"})
	start := filesystem.GetPath()
	_, reference, ok := filesystem.HistoryEntry()
	if !ok {
		t.Fatal("qualified connection declined its folder history entry")
	}
	if err := filesystem.SetPathOptimistic(filesystem.Join(start, "different")); err != nil {
		t.Fatal(err)
	}
	if !filesystem.NavigateHistoryEntry(reference) || filesystem.GetPath() != start {
		t.Fatalf("history failed to restore public path %q: got %q", start, filesystem.GetPath())
	}
	other := filesystem.Clone().(*FishVFS)
	defer func() { _ = other.Close() }()
	other.SetDevicePath(vfs.DevicePath{Scheme: "net", Device: "other connection"})
	if other.NavigateHistoryEntry(reference) {
		t.Fatal("history accepted a reference belonging to another connection authority")
	}
}

// TestFishVFSHistoryEntryIsNavigable checks that an open FISH+ session
// records a real, navigable folder-history entry (f4#262) rather than a bare
// connection-list placeholder, and that the recorded reference actually
// takes the same live session back to that location.
func TestFishVFSHistoryEntryIsNavigable(t *testing.T) {
	v := newLocalFishVFS(t)
	defer func() { _ = v.Close() }()

	startPath := v.GetPath()
	display, ref, ok := v.HistoryEntry()
	if !ok {
		t.Fatal("an open FISH+ session must produce a folder-history entry")
	}
	if display == "" || strings.Contains(display, "net://") {
		t.Fatalf("history entry display must be a real, navigable location, not a bare entry point, got %q", display)
	}
	if ref == "" {
		t.Fatal("history entry must carry a non-empty opaque reference")
	}

	// Move away, then use the recorded ref to come back: the "session
	// already open" success path (no reconnect, same live instance).
	if err := v.SetPath("/tmp"); err != nil {
		t.Skipf("could not change directory for the round-trip check: %v", err)
	}
	if !v.NavigateHistoryEntry(ref) {
		t.Fatal("navigating back to a live session's own recorded entry must succeed")
	}
	if got := v.GetPath(); got != startPath {
		t.Fatalf("NavigateHistoryEntry moved to %q, want %q", got, startPath)
	}

	// A reference recorded from one session must never be accepted by a
	// different one, and doing so must not attempt to reconnect or crash.
	other := newLocalFishVFSWithTitle(t, "a-different-session")
	defer func() { _ = other.Close() }()
	if other.NavigateHistoryEntry(ref) {
		t.Fatal("a history entry from one session must not be accepted by a different one")
	}
}
