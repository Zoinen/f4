package vfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOSVFSContractCoversLocalOperations(t *testing.T) {
	root := t.TempDir()
	filesystem := NewOSVFS(root)
	if filesystem.GetPath() != root || !filesystem.IsAbs(root) || filesystem.IsAtRoot() {
		t.Fatalf("initial path state = %q/%v/%v", filesystem.GetPath(), filesystem.IsAbs(root), filesystem.IsAtRoot())
	}
	if filesystem.Base(filepath.Join(root, "nested", "file.txt")) != "file.txt" || filesystem.Dir(filepath.Join(root, "nested", "file.txt")) != filepath.Join(root, "nested") {
		t.Fatal("path helper methods returned wrong values")
	}
	if filesystem.ParentVFS() != nil || filesystem.Close() != nil {
		t.Fatal("OSVFS root lifecycle contract changed")
	}
	clone := filesystem.Clone()
	if clone.GetPath() != filesystem.GetPath() || clone == filesystem {
		t.Fatal("Clone did not create an equivalent independent VFS")
	}
	caps := filesystem.GetCapabilities()
	if !caps.HasWrite || !caps.HasRandomAccess || !caps.HasAtomicNoReplaceRename || caps.HasSearch {
		t.Fatalf("local capabilities = %#v", caps)
	}
	if result, err := filesystem.Search(context.Background(), root, "*"); err != nil || result != nil {
		t.Fatalf("local search stub = %v, %v", result, err)
	}

	nested := filepath.Join(root, "nested", "dir")
	if err := filesystem.MkDir(context.Background(), nested); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "file.txt")
	writer, err := filesystem.Create(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("hello local vfs")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := filesystem.Open(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Size() != int64(len("hello local vfs")) {
		t.Fatalf("Open size = %d", reader.Size())
	}
	buf := make([]byte, 5)
	if n, err := reader.ReadAt(context.Background(), buf, 6); err != nil || string(buf[:n]) != "local" {
		t.Fatalf("ReadAt = %q, %v", buf[:n], err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	writeAt, err := filesystem.OpenWriteAt(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeAt.WriteAt([]byte("LOCAL"), 6); err != nil {
		t.Fatal(err)
	}
	if err := writeAt.Truncate(15); err != nil {
		t.Fatal(err)
	}
	if err := writeAt.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "hello LOCAL vfs" {
		t.Fatalf("OpenWriteAt result = %q, %v", data, err)
	}

	if runtime.GOOS != "windows" {
		item, err := filesystem.Stat(context.Background(), file)
		if err != nil {
			t.Fatal(err)
		}
		item.UnixMode = 0600
		item.Uid = -1
		item.Gid = -1
		if err := filesystem.SetAttributes(context.Background(), file, item); err != nil {
			t.Fatal(err)
		}
		if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("SetAttributes mode = %v, %v", info.Mode().Perm(), err)
		}
	}

	rename := filepath.Join(nested, "renamed.txt")
	if err := filesystem.Rename(context.Background(), file, rename); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Remove(context.Background(), filepath.Join(root, "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rename); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Remove left renamed file: %v", err)
	}
}

func TestOSVFSLinksAndDirectoryChunkDelivery(t *testing.T) {
	root := t.TempDir()
	filesystem := NewOSVFS(root)
	for i := 0; i < 1001; i++ {
		name := filepath.Join(root, fmt.Sprintf("file-%04d", i))
		if err := os.WriteFile(name, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "file-0000"), 0700); err != nil { // #nosec G302 -- executable detection is the behavior under test.
		t.Fatal(err)
	}

	chunks := 0
	items := 0
	var executable, hidden bool
	if err := filesystem.ReadDir(context.Background(), root, func(batch []VFSItem) {
		chunks++
		items += len(batch)
		for _, item := range batch {
			executable = executable || item.Name == "file-0000" && item.IsExecutable
			hidden = hidden || item.Name == ".hidden" && item.IsHidden
		}
	}); err != nil {
		t.Fatal(err)
	}
	wantExecutable := runtime.GOOS != "windows"
	if chunks != 2 || items != 1002 || executable != wantExecutable || !hidden {
		t.Fatalf("directory delivery = chunks %d items %d executable %v hidden %v", chunks, items, executable, hidden)
	}

	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("target"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink(context.Background(), target, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if got, err := filesystem.Readlink(context.Background(), link); err != nil || got != target {
		t.Fatalf("Readlink = %q, %v", got, err)
	}
	if item, err := filesystem.Lstat(context.Background(), link); err != nil || !item.IsSymlink {
		t.Fatalf("Lstat symlink = %#v, %v", item, err)
	}
	hard := filepath.Join(root, "hard.txt")
	if err := filesystem.Hardlink(context.Background(), target, hard); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(hard); err != nil || string(data) != "target" {
		t.Fatalf("hardlink contents = %q, %v", data, err)
	}
	junction := filepath.Join(root, "junction.txt")
	if err := filesystem.Junction(context.Background(), target, junction); err != nil {
		t.Skip("junction/symlink unavailable")
	}
}

func TestOSVFSContextCancellationCoversMutations(t *testing.T) {
	root := t.TempDir()
	filesystem := NewOSVFS(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	want := context.Canceled

	if err := filesystem.MkDir(ctx, filepath.Join(root, "mkdir")); !errors.Is(err, want) {
		t.Errorf("MkDir error = %v", err)
	}
	if err := filesystem.Remove(ctx, filepath.Join(root, "remove")); !errors.Is(err, want) {
		t.Errorf("Remove error = %v", err)
	}
	if err := filesystem.Rename(ctx, "old", "new"); !errors.Is(err, want) {
		t.Errorf("Rename error = %v", err)
	}
	if err := filesystem.RenameNoReplace(ctx, "old", "new"); !errors.Is(err, want) {
		t.Errorf("RenameNoReplace error = %v", err)
	}
	if _, err := filesystem.Stat(ctx, root); !errors.Is(err, want) {
		t.Errorf("Stat error = %v", err)
	}
	if _, err := filesystem.Lstat(ctx, root); !errors.Is(err, want) {
		t.Errorf("Lstat error = %v", err)
	}
	if _, err := filesystem.Open(ctx, root); !errors.Is(err, want) {
		t.Errorf("Open error = %v", err)
	}
	if _, err := filesystem.Create(ctx, filepath.Join(root, "create")); !errors.Is(err, want) {
		t.Errorf("Create error = %v", err)
	}
	if _, err := filesystem.OpenWriteAt(ctx, filepath.Join(root, "write")); !errors.Is(err, want) {
		t.Errorf("OpenWriteAt error = %v", err)
	}
	if _, err := filesystem.Readlink(ctx, root); !errors.Is(err, want) {
		t.Errorf("Readlink error = %v", err)
	}
	if err := filesystem.Symlink(ctx, "target", filepath.Join(root, "link")); !errors.Is(err, want) {
		t.Errorf("Symlink error = %v", err)
	}
	if err := filesystem.Hardlink(ctx, "target", filepath.Join(root, "hard")); !errors.Is(err, want) {
		t.Errorf("Hardlink error = %v", err)
	}
	if err := filesystem.Junction(ctx, "target", filepath.Join(root, "junction")); !errors.Is(err, want) {
		t.Errorf("Junction error = %v", err)
	}
}
