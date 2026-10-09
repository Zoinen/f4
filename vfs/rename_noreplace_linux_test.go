//go:build linux

package vfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// renameNoReplace (the linux-only fast path backed by renameat2's
// RENAME_NOREPLACE flag) had no dedicated test: only its portable fallback,
// renameNoReplacePortable, was exercised elsewhere. These tests hit the real
// syscall on the Linux CI runner this package builds and tests on.

func TestRenameNoReplaceLinuxMovesFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := renameNoReplace(oldPath, newPath); err != nil {
		t.Fatalf("renameNoReplace: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("source still exists after rename: %v", err)
	}
	got, err := os.ReadFile(newPath)
	if err != nil || string(got) != "payload" {
		t.Fatalf("destination content = %q, %v", got, err)
	}
}

func TestRenameNoReplaceLinuxRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("already here"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := renameNoReplace(oldPath, newPath)
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("renameNoReplace(existing destination) = %v, want ErrDestinationExists", err)
	}
	// RENAME_NOREPLACE must leave both files exactly as they were.
	if got, rerr := os.ReadFile(oldPath); rerr != nil || string(got) != "source" {
		t.Fatalf("source after refused rename = %q, %v", got, rerr)
	}
	if got, rerr := os.ReadFile(newPath); rerr != nil || string(got) != "already here" {
		t.Fatalf("destination after refused rename = %q, %v", got, rerr)
	}
}

func TestRenameNoReplaceLinuxSameObjectIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same")
	if err := os.WriteFile(path, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplace(path, path); err != nil {
		t.Fatalf("renameNoReplace(same path): %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "same" {
		t.Fatalf("content after same-path rename = %q, %v", got, err)
	}
}

func TestRenameNoReplaceLinuxDirectory(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old-dir")
	newPath := filepath.Join(dir, "new-dir")
	if err := os.Mkdir(oldPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "inside"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := renameNoReplace(oldPath, newPath); err != nil {
		t.Fatalf("renameNoReplace(directory): %v", err)
	}
	if _, err := os.Stat(filepath.Join(newPath, "inside")); err != nil {
		t.Fatalf("directory contents missing after rename: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("directory source still exists: %v", err)
	}
}

func TestRenameNoReplaceLinuxMissingSource(t *testing.T) {
	dir := t.TempDir()
	err := renameNoReplace(filepath.Join(dir, "missing"), filepath.Join(dir, "new"))
	if err == nil {
		t.Fatal("renameNoReplace(missing source) unexpectedly succeeded")
	}
	if errors.Is(err, ErrDestinationExists) {
		t.Fatalf("renameNoReplace(missing source) = %v, want a not-exist error, not ErrDestinationExists", err)
	}
}
