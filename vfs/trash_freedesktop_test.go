//go:build linux || dragonfly || freebsd || netbsd || openbsd || solaris || illumos

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreedesktopTrashWritesInfoAndKeepsCollisions(t *testing.T) {
	root := t.TempDir()
	trash := freedesktopTrash{
		root:          filepath.Join(root, "Trash"),
		files:         filepath.Join(root, "Trash", "files"),
		info:          filepath.Join(root, "Trash", "info"),
		absolutePaths: true,
	}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}

	srcA := filepath.Join(root, "a", "same name.txt")
	srcB := filepath.Join(root, "b", "same name.txt")
	for _, path := range []string{srcA, srcB} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
		if err := moveIntoFreedesktopTrash(context.Background(), path, trash); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("source still exists after trash move: %s", path)
		}
	}

	for _, name := range []string{"same name.txt", "same name.txt.1"} {
		if _, err := os.Stat(filepath.Join(trash.files, name)); err != nil {
			t.Errorf("missing trashed payload %q: %v", name, err)
		}
		body, err := os.ReadFile(filepath.Join(trash.info, name+".trashinfo"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.HasPrefix(text, "[Trash Info]\nPath=/") || strings.Contains(text, "%2F") || !strings.Contains(text, "%20") || !strings.Contains(text, "\nDeletionDate=") {
			t.Errorf("invalid .trashinfo for %q:\n%s", name, text)
		}
	}
}

func TestFreedesktopTrashCancellationIsNonDestructive(t *testing.T) {
	root := t.TempDir()
	trash := freedesktopTrash{root: filepath.Join(root, "Trash"), files: filepath.Join(root, "Trash", "files"), info: filepath.Join(root, "Trash", "info"), absolutePaths: true}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := moveIntoFreedesktopTrash(ctx, source, trash); err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("canceled trash move changed source: %v", err)
	}
	entries, err := os.ReadDir(trash.info)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled trash move left metadata: %v", entries)
	}
}

func TestFreedesktopTrashRejectsItsOwnAncestor(t *testing.T) {
	source := t.TempDir()
	trash := freedesktopTrash{
		root:          filepath.Join(source, "data", "Trash"),
		files:         filepath.Join(source, "data", "Trash", "files"),
		info:          filepath.Join(source, "data", "Trash", "info"),
		absolutePaths: true,
	}
	if err := ensurePrivateTrashDir(trash.root); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrashSubdirs(trash); err != nil {
		t.Fatal(err)
	}
	if err := moveIntoFreedesktopTrash(context.Background(), source, trash); err == nil {
		t.Fatal("trash ancestor was moved into itself")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("rejecting trash ancestor changed source: %v", err)
	}
}

func TestFreedesktopTrashRepairsBroadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Trash")
	if err := os.Mkdir(path, 0775); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateTrashDir(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != privateTrashDirMode {
		t.Fatalf("repaired trash directory mode = %#o, want %#o", got, privateTrashDirMode)
	}
}

func TestFreedesktopTrashRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	path := filepath.Join(root, "Trash")
	if err := os.Mkdir(target, privateTrashDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateTrashDir(path); err == nil {
		t.Fatal("symlink trash directory was accepted")
	}
}

func TestFreedesktopVolumeTrashFallsBackFromUnsafeSharedDir(t *testing.T) {
	mountRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(mountRoot, ".Trash"), 0755); err != nil {
		t.Fatal(err)
	}
	trash, err := prepareVolumeTrash(mountRoot)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := filepath.Join(mountRoot, ".Trash-")
	if !strings.HasPrefix(trash.root, wantPrefix) {
		t.Fatalf("unsafe shared .Trash selected as root %q; want private %q prefix", trash.root, wantPrefix)
	}
}

func TestOSVFSFreedesktopTrashEndToEnd(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	trashRoot := filepath.Join(dataHome, "Trash")
	for _, path := range []string{trashRoot, filepath.Join(trashRoot, "files"), filepath.Join(trashRoot, "info")} {
		if err := os.MkdirAll(path, 0775); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0775); err != nil {
			t.Fatal(err)
		}
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(work, "recoverable.txt")
	if err := os.WriteFile(source, []byte("recover me"), 0600); err != nil {
		t.Fatal(err)
	}

	filesystem := NewOSVFS(work)
	if err := filesystem.MoveToTrash(context.Background(), "recoverable.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after MoveToTrash: %v", err)
	}
	payload := filepath.Join(dataHome, "Trash", "files", "recoverable.txt")
	if got, err := os.ReadFile(payload); err != nil || string(got) != "recover me" {
		t.Fatalf("trashed payload = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dataHome, "Trash", "info", "recoverable.txt.trashinfo")); err != nil {
		t.Fatalf("trash metadata missing: %v", err)
	}
	for _, path := range []string{trashRoot, filepath.Join(trashRoot, "files"), filepath.Join(trashRoot, "info")} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != privateTrashDirMode {
			t.Errorf("%s mode = %#o, want %#o", path, got, privateTrashDirMode)
		}
	}
}
