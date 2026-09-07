package main

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// TestFileStateKey_LocalPathStaysBare keeps the keys already written to
// file_states.json working: a local file must go on being stored under its own
// absolute path, or every position saved before this change is lost.
func TestFileStateKey_LocalPathStaysBare(t *testing.T) {
	dir := t.TempDir()
	v := vfs.NewOSVFS(dir)
	path := filepath.Join(dir, "notes.txt")

	if got := FileStateKey(v, path); got != path {
		t.Fatalf("local key = %q, want the bare path %q", got, path)
	}
}

// TestFileStateKey_LocalRelativePathIsResolved covers the other half of the
// same promise: a panel may hand over a path relative to where it is standing,
// and the same file must not get two entries because of it.
func TestFileStateKey_LocalRelativePathIsResolved(t *testing.T) {
	dir := t.TempDir()
	v := vfs.NewOSVFS(dir)

	absKey := FileStateKey(v, filepath.Join(dir, "notes.txt"))
	relKey := FileStateKey(v, "notes.txt")
	if absKey != relKey {
		t.Fatalf("the same file has two keys: %q and %q", absKey, relKey)
	}
}

// TestFileStateKey_RemoteDoesNotCollideWithLocal is the reported bug: the same
// path on a remote host and here used to be one entry, so opening one restored
// the position left in the other.
func TestFileStateKey_RemoteDoesNotCollideWithLocal(t *testing.T) {
	local := vfs.NewOSVFS("/")
	remote := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/"), title: "user@host"}

	localKey := FileStateKey(local, "/etc/hosts")
	remoteKey := FileStateKey(remote, "/etc/hosts")
	if localKey == remoteKey {
		t.Fatalf("a remote file and its local namesake share the key %q", localKey)
	}
	// Abs rewrites the path per-platform (drive letter and backslashes on
	// Windows), so derive the expectation instead of hardcoding the unix shape.
	absPath, _ := local.Abs("/etc/hosts")
	if want := "user@host:" + absPath; remoteKey != want {
		t.Fatalf("remote key = %q, want %q", remoteKey, want)
	}
}

// TestFileStateKey_TwoHostsDoNotCollide is the same collision between two
// remote sites, which two hosts built from one image hit on every path.
func TestFileStateKey_TwoHostsDoNotCollide(t *testing.T) {
	first := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/"), title: "user@alpha"}
	second := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/"), title: "user@beta"}

	if a, b := FileStateKey(first, "/srv/app.conf"), FileStateKey(second, "/srv/app.conf"); a == b {
		t.Fatalf("two hosts share the key %q", a)
	}
}

// TestFileStateKey_SameSiteRoundTrips is what makes the feature work at all:
// two sessions to one host must agree, or the position is never found again.
func TestFileStateKey_SameSiteRoundTrips(t *testing.T) {
	saved := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/"), title: "user@host"}
	reopened := &mockTitleVFS{OSVFS: *vfs.NewOSVFS("/"), title: "user@host"}

	fs := &F4FileStateProvider{Limit: 10, Data: make(map[string]*FileState)}
	fs.updateEditorState(FileStateKey(saved, "/srv/app.conf"), 42, 7, 40, 0, false)

	state := fs.GetState(FileStateKey(reopened, "/srv/app.conf"))
	if state == nil {
		t.Fatal("the position saved on one session was not found by the next")
	}
	if state.EditorLine != 42 || state.EditorPos != 7 {
		t.Fatalf("restored %d:%d, want 42:7", state.EditorLine, state.EditorPos)
	}
	if fs.GetState(FileStateKey(vfs.NewOSVFS("/"), "/srv/app.conf")) != nil {
		t.Fatal("the local namesake picked up the remote position")
	}
}

// TestFileStateKey_NilVFS guards the paths that have no file system at hand,
// such as an editor opened on a buffer that was never on disk.
func TestFileStateKey_NilVFS(t *testing.T) {
	if got := FileStateKey(nil, "scratch.txt"); got != "scratch.txt" {
		t.Fatalf("key = %q, want the path unchanged", got)
	}
}
