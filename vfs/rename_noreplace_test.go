package vfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDestinationExists(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if err := destinationExists(missing); err != nil {
		t.Fatalf("missing destination: %v", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(destinationExists(file), ErrDestinationExists) {
		t.Fatal("existing destination was not rejected")
	}
}

func TestRenameNoReplacePortableFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplacePortable(oldPath, newPath); err != nil {
		t.Fatalf("renameNoReplacePortable(file): %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	got, err := os.ReadFile(newPath)
	if err != nil || string(got) != "content" {
		t.Fatalf("destination = %q, %v", got, err)
	}

	if err := os.WriteFile(oldPath, []byte("new content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplacePortable(oldPath, newPath); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("existing destination error = %v", err)
	}
	if got, err := os.ReadFile(oldPath); err != nil || string(got) != "new content" {
		t.Fatalf("source after refused rename = %q, %v", got, err)
	}
}

func TestRenameNoReplacePortableDirectory(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old-dir")
	newPath := filepath.Join(dir, "new-dir")
	if err := os.Mkdir(oldPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "inside"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplacePortable(oldPath, newPath); err != nil {
		// Unix rejects replacing the reservation directory itself. The fallback
		// must then remove its reservation and leave the source untouched.
		if _, statErr := os.Stat(oldPath); statErr != nil {
			t.Fatalf("directory source after refused rename: %v", statErr)
		}
		if _, statErr := os.Stat(newPath); !os.IsNotExist(statErr) {
			t.Fatalf("directory reservation was not removed: %v", statErr)
		}
		return
	}
	if _, err := os.Stat(filepath.Join(newPath, "inside")); err != nil {
		t.Fatalf("directory contents missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("directory source still exists: %v", err)
	}
}

func TestRenameNoReplacePortableSameObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same")
	if err := os.WriteFile(path, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !sameObject(path, path) {
		t.Fatal("a path was not recognized as the same object")
	}
	if err := renameNoReplacePortable(path, path); err != nil {
		t.Fatalf("same-path rename: %v", err)
	}

	alias := filepath.Join(dir, "alias")
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	if !sameObject(path, alias) {
		t.Fatal("hard-linked paths were not recognized as the same object")
	}
	if err := renameNoReplacePortable(path, alias); err != nil {
		t.Fatalf("same-object rename: %v", err)
	}
	for _, candidate := range []string{path, alias} {
		got, err := os.ReadFile(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || string(got) != "same" {
			t.Fatalf("same-object path %q = %q, %v", candidate, got, err)
		}
	}
	if got, err := os.ReadFile(alias); err != nil || string(got) != "same" {
		t.Fatalf("same-object destination = %q, %v", got, err)
	}
}

func TestRenameNoReplacePortableMissingSource(t *testing.T) {
	err := renameNoReplacePortable(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "new"))
	if err == nil {
		t.Fatal("missing source was accepted")
	}
}

func TestRenameNoReplacePortableSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	oldPath := filepath.Join(dir, "old-link")
	newPath := filepath.Join(dir, "new-link")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, oldPath); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := renameNoReplacePortable(oldPath, newPath); err != nil {
		t.Fatalf("renameNoReplacePortable(symlink): %v", err)
	}
	if got, err := os.ReadFile(newPath); err != nil || string(got) != "target" {
		t.Fatalf("symlink destination = %q, %v", got, err)
	}
}
