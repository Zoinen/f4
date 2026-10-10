//go:build linux || dragonfly || freebsd || netbsd || openbsd || solaris || illumos

package vfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This file covers the platform-agnostic FreeDesktop.org Trash algorithm in
// vfs/trash_xdg.go beyond what trash_freedesktop_test.go's happy-path/real-fs
// scenarios already exercise: the error branches around the trashLstat/
// trashMkdirAll/trashChmod/trashWriteExclusive/trashRenameNoReplace
// substitution points (WINE.md §13 pattern, see trash_xdg.go's own doc
// comment) and the small pure path helpers at the bottom of the file. It
// swaps those package vars the same way trash_windows_posix_test.go already
// does for Wine's posix personality, restoring the originals via t.Cleanup.

func TestPosixAbsClean(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already absolute", "/a/b", "/a/b"},
		{"relative gets rooted", "a/b", "/a/b"},
		{"empty becomes root", "", "/"},
		{"traversal is cleaned", "/a/../b", "/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := posixAbsClean(tt.in); got != tt.want {
				t.Errorf("posixAbsClean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPosixRelUnder(t *testing.T) {
	if _, err := posixRelUnder("/a", "/a"); err == nil {
		t.Error("posixRelUnder(base, base) = nil error, want an error")
	}
	if _, err := posixRelUnder("/a", "/b"); err == nil {
		t.Error("posixRelUnder(/a, /b) = nil error, want an error")
	}
	got, err := posixRelUnder("/a", "/a/b/c")
	if err != nil {
		t.Fatalf("posixRelUnder(/a, /a/b/c) unexpected error: %v", err)
	}
	if got != "b/c" {
		t.Errorf("posixRelUnder(/a, /a/b/c) = %q, want %q", got, "b/c")
	}
	// Root as base has no trailing slash to add; make sure it still resolves.
	got, err = posixRelUnder("/", "/item")
	if err != nil {
		t.Fatalf("posixRelUnder(/, /item) unexpected error: %v", err)
	}
	if got != "item" {
		t.Errorf("posixRelUnder(/, /item) = %q, want %q", got, "item")
	}
}

func TestIsPathWithin(t *testing.T) {
	tests := []struct {
		name string
		path string
		dir  string
		want bool
	}{
		{"identical paths", "/a/b", "/a/b", true},
		{"root dir contains everything", "/anything/at/all", "/", true},
		{"real prefix", "/a/b/c", "/a/b", true},
		{"sibling is not within", "/a/bc", "/a/b", false},
		{"parent is not within child", "/a", "/a/b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPathWithin(tt.path, tt.dir); got != tt.want {
				t.Errorf("isPathWithin(%q, %q) = %v, want %v", tt.path, tt.dir, got, tt.want)
			}
		})
	}
}

func TestPathExists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(present, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if !pathExists(present) {
		t.Error("pathExists(existing file) = false, want true")
	}
	if pathExists(filepath.Join(dir, "missing.txt")) {
		t.Error("pathExists(missing file) = true, want false")
	}
}

// --- var-swap helpers, mirroring trash_windows_posix_test.go's pattern ---

func withTrashMkdirAllFailing(t *testing.T, err error) {
	t.Helper()
	orig := trashMkdirAll
	trashMkdirAll = func(path string, perm os.FileMode) error { return err }
	t.Cleanup(func() { trashMkdirAll = orig })
}

func withTrashChmodStub(t *testing.T, stub func(name string, mode os.FileMode) error) {
	t.Helper()
	orig := trashChmod
	trashChmod = stub
	t.Cleanup(func() { trashChmod = orig })
}

func withTrashLstatOverride(t *testing.T, target string, info os.FileInfo, err error) {
	t.Helper()
	orig := trashLstat
	trashLstat = func(name string) (os.FileInfo, error) {
		if name == target {
			return info, err
		}
		return orig(name)
	}
	t.Cleanup(func() { trashLstat = orig })
}

// fakeStatFileInfo wraps a real os.FileInfo but reports a Sys() value that
// is not a *syscall.Stat_t, the same shape trashStatIdentity rejects.
type fakeStatFileInfo struct{ os.FileInfo }

func (fakeStatFileInfo) Sys() any { return "not a syscall.Stat_t" }

func TestMoveToFreedesktopTrashMissingSource(t *testing.T) {
	dir := t.TempDir()
	err := moveToFreedesktopTrash(context.Background(), filepath.Join(dir, "does-not-exist"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("moveToFreedesktopTrash(missing source) error = %v, want ErrNotExist", err)
	}
}

func TestMoveToFreedesktopTrashRejectsUnknownFilesystemIdentity(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "item.txt")
	if err := os.WriteFile(source, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	real, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	withTrashLstatOverride(t, source, fakeStatFileInfo{real}, nil)

	err = moveToFreedesktopTrash(context.Background(), source)
	if err == nil {
		t.Fatal("moveToFreedesktopTrash with unidentifiable source succeeded, want an error")
	}
}

func TestMoveToFreedesktopTrashCombinesHomeAndVolumeErrors(t *testing.T) {
	source := t.TempDir()
	// Point XDG_DATA_HOME's Trash root inside the item being trashed, so the
	// home trash is rejected as "inside the selected item" (trash_xdg.go's
	// own defensive check), then force the volume fallback to fail too.
	t.Setenv("XDG_DATA_HOME", filepath.Join(source, "xdgdata"))
	withTrashMkdirAllFailing(t, errors.New("no space left on device"))

	err := moveToFreedesktopTrash(context.Background(), source)
	if err == nil {
		t.Fatal("moveToFreedesktopTrash succeeded, want a combined home+volume error")
	}
	if !strings.Contains(err.Error(), "home Recycle Bin is inside the selected item") ||
		!strings.Contains(err.Error(), "no space left on device") {
		t.Errorf("moveToFreedesktopTrash error = %q, want it to mention both the home and volume failures", err.Error())
	}
}

func TestEnsurePrivateTrashDirFailsWhenParentIsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateTrashDir(filepath.Join(blocker, "sub")); err == nil {
		t.Fatal("ensurePrivateTrashDir under a non-directory parent succeeded, want an error")
	}
}

func TestEnsureTrashSubdirsPropagatesFilesError(t *testing.T) {
	withTrashMkdirAllFailing(t, errors.New("boom"))
	trash := freedesktopTrash{
		files: filepath.Join(t.TempDir(), "files"),
		info:  filepath.Join(t.TempDir(), "info"),
	}
	if err := ensureTrashSubdirs(trash); err == nil {
		t.Fatal("ensureTrashSubdirs succeeded despite a failing MkdirAll, want an error")
	}
}

func TestEnsurePrivateTrashDirChmodRepairPaths(t *testing.T) {
	tests := []struct {
		name string
		stub func(name string, mode os.FileMode) error
	}{
		{
			name: "chmod itself fails",
			stub: func(name string, mode os.FileMode) error { return errors.New("chmod denied") },
		},
		{
			name: "chmod reports success but leaves the mode broad",
			stub: func(name string, mode os.FileMode) error { return nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Trash")
			if err := os.Mkdir(path, 0775); err != nil {
				t.Fatal(err)
			}
			withTrashChmodStub(t, tt.stub)
			if err := ensurePrivateTrashDir(path); err == nil {
				t.Fatal("ensurePrivateTrashDir accepted a directory whose broad permissions were never repaired")
			}
		})
	}
}

func TestPrepareVolumeTrashUsesSafeSharedStickyDir(t *testing.T) {
	mountRoot := t.TempDir()
	shared := filepath.Join(mountRoot, ".Trash")
	if err := os.Mkdir(shared, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}

	trash, err := prepareVolumeTrash(mountRoot)
	if err != nil {
		t.Fatalf("prepareVolumeTrash: %v", err)
	}
	want := filepath.Join(shared, strconv.Itoa(os.Getuid()))
	if trash.root != want {
		t.Errorf("prepareVolumeTrash root = %q, want the safe shared sticky dir %q", trash.root, want)
	}
}

func TestPrepareVolumeTrashPropagatesMkdirError(t *testing.T) {
	t.Run("inside the safe shared sticky dir", func(t *testing.T) {
		mountRoot := t.TempDir()
		shared := filepath.Join(mountRoot, ".Trash")
		if err := os.Mkdir(shared, 0777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(shared, 0777|os.ModeSticky); err != nil {
			t.Fatal(err)
		}
		withTrashMkdirAllFailing(t, errors.New("boom"))
		if _, err := prepareVolumeTrash(mountRoot); err == nil {
			t.Fatal("prepareVolumeTrash succeeded despite a failing MkdirAll under the shared dir")
		}
	})
	t.Run("in the private fallback dir", func(t *testing.T) {
		mountRoot := t.TempDir()
		withTrashMkdirAllFailing(t, errors.New("boom"))
		if _, err := prepareVolumeTrash(mountRoot); err == nil {
			t.Fatal("prepareVolumeTrash succeeded despite a failing MkdirAll for .Trash-<uid>")
		}
	})
}

func TestFilesystemMountRootMissingSource(t *testing.T) {
	dir := t.TempDir()
	if _, err := filesystemMountRoot(filepath.Join(dir, "missing"), 0); err == nil {
		t.Fatal("filesystemMountRoot(missing source) succeeded, want an error")
	}
}

func TestFilesystemMountRootStopsAtFilesystemRoot(t *testing.T) {
	// Starting the walk at "/" itself hits the loop's own termination case
	// directly: path.Dir("/") == "/", so parent == current on the very
	// first iteration, before any device comparison is needed.
	got, err := filesystemMountRoot("/", ^uint64(0))
	if err != nil {
		t.Fatalf("filesystemMountRoot(/, ...): %v", err)
	}
	if got != "/" {
		t.Errorf("filesystemMountRoot(/, ...) = %q, want %q", got, "/")
	}
}

func TestMoveIntoFreedesktopTrashRejectsSourceInsideTrashRoot(t *testing.T) {
	root := t.TempDir()
	trash := freedesktopTrash{root: root, files: filepath.Join(root, "files"), info: filepath.Join(root, "info"), absolutePaths: true}
	if err := moveIntoFreedesktopTrash(context.Background(), root, trash); err == nil {
		t.Fatal("moveIntoFreedesktopTrash accepted the trash root itself as a source")
	}
}

func TestMoveIntoFreedesktopTrashRejectsUnrelativizablePath(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	source := filepath.Join(outside, "item.txt")
	if err := os.WriteFile(source, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	trash := freedesktopTrash{
		root:          filepath.Join(base, "Trash"),
		files:         filepath.Join(base, "Trash", "files"),
		info:          filepath.Join(base, "Trash", "info"),
		pathBase:      base,
		absolutePaths: false,
	}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}
	if err := moveIntoFreedesktopTrash(context.Background(), source, trash); err == nil {
		t.Fatal("moveIntoFreedesktopTrash accepted a source outside the volume's mount root")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("rejected move should not have touched the source: %v", err)
	}
}

func TestMoveIntoFreedesktopTrashRejectsDotSource(t *testing.T) {
	root := t.TempDir()
	trash := freedesktopTrash{root: filepath.Join(root, "Trash"), files: filepath.Join(root, "Trash", "files"), info: filepath.Join(root, "Trash", "info"), absolutePaths: true}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}
	if err := moveIntoFreedesktopTrash(context.Background(), ".", trash); err == nil {
		t.Fatal("moveIntoFreedesktopTrash(\".\") succeeded, want an invalid-item-name error")
	}
}

func TestMoveIntoFreedesktopTrashRetriesOnInfoFileCollision(t *testing.T) {
	root := t.TempDir()
	trash := freedesktopTrash{root: filepath.Join(root, "Trash"), files: filepath.Join(root, "Trash", "files"), info: filepath.Join(root, "Trash", "info"), absolutePaths: true}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "item.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	orig := trashWriteExclusive
	calls := 0
	trashWriteExclusive = func(path string, data []byte, mode os.FileMode) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("racing writer: %w", os.ErrExist)
		}
		return orig(path, data, mode)
	}
	t.Cleanup(func() { trashWriteExclusive = orig })

	if err := moveIntoFreedesktopTrash(context.Background(), source, trash); err != nil {
		t.Fatalf("moveIntoFreedesktopTrash: %v", err)
	}
	if calls < 2 {
		t.Fatalf("trashWriteExclusive called %d times, want at least 2 (retry past the collision)", calls)
	}
	// The first (collided) suffix name must have been skipped in favor of ".1".
	if _, err := os.Stat(filepath.Join(trash.files, "item.txt.1")); err != nil {
		t.Errorf("expected the retried name item.txt.1 to exist: %v", err)
	}
}

func TestMoveIntoFreedesktopTrashRetriesOnRenameCollision(t *testing.T) {
	root := t.TempDir()
	trash := freedesktopTrash{root: filepath.Join(root, "Trash"), files: filepath.Join(root, "Trash", "files"), info: filepath.Join(root, "Trash", "info"), absolutePaths: true}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "item.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	orig := trashRenameNoReplace
	calls := 0
	trashRenameNoReplace = func(oldPath, newPath string) error {
		calls++
		if calls == 1 {
			return ErrDestinationExists
		}
		return orig(oldPath, newPath)
	}
	t.Cleanup(func() { trashRenameNoReplace = orig })

	if err := moveIntoFreedesktopTrash(context.Background(), source, trash); err != nil {
		t.Fatalf("moveIntoFreedesktopTrash: %v", err)
	}
	if calls < 2 {
		t.Fatalf("trashRenameNoReplace called %d times, want at least 2 (retry past the rename collision)", calls)
	}
	// The .trashinfo written for the collided first attempt must have been
	// cleaned up, leaving only the successful retry's metadata behind.
	if _, err := os.Stat(filepath.Join(trash.info, "item.txt.trashinfo")); !os.IsNotExist(err) {
		t.Errorf("collided attempt's .trashinfo should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(trash.info, "item.txt.1.trashinfo")); err != nil {
		t.Errorf("expected the retried name's .trashinfo to exist: %v", err)
	}
}
